# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go application for interfacing with SRNE ASP/ASF-series hybrid inverters via MODBUS protocol over Solarman V5 wifi adaptors.

**Interfaces:**
1. CLI — dump/modify registers, verify communication (done)
2. Web dashboard — HTMX + SSE + Beer CSS (Material Design 3), live updates (done)
3. REST API — JSON endpoints for snapshot, settings, faults (done)
4. MQTT — Home Assistant auto-discovery, per-unit and system-level sensors (done)
5. Home Assistant integration — via MQTT discovery (done)

**Multi-inverter:** Systems can run in parallel — multiple read endpoints, single master for writes. The `inverter.System` type manages connections, aggregates snapshots, and writes settings to all units.

**Test hardware:** Two SRNE ASP48100U200-H in parallel (split-phase 120/240V)
- Inverter 1: dongle 10.100.3.92, slave ID 1, serial 3574468395
- Inverter 2: dongle 10.100.5.55, slave ID 2
- Product type 4, protocol V1.07, SW V8.18, LiFePO4 with BMS (battery type 6)

## Environment Setup

- **mise** manages the Go toolchain version
- Run `mise install` to set up the environment
- Prefix commands with `mise exec --` or use `mise shell` to activate the environment

## Commands

- **Build**: `mise exec -- go build ./...`
- **Run tests**: `mise exec -- go test ./...`
- **Run a single test**: `mise exec -- go test ./... -run TestName`
- **Dump all**: `mise exec -- go run . dump --host 10.100.3.92 all`
- **Probe registers**: `mise exec -- go run . probe --host 10.100.3.92 0xE100 0xE150`
- **Start service**: `mise run serve` (uses `srne.toml`)

## Architecture

- `modbus/` — MODBUS RTU framing, CRC16, `Client` interface, `Session` (register cache + retry)
- `interfaces/solarman/` — Solarman V5 TCP transport implementing `modbus.Client`
- `interfaces/mock/` — Mock inverter (`Inverter`) and live simulator (`Sim`) with deterministic poke methods (`SetSOC`, `SetPV`, `SetLoad`, `SetGridVoltage`, `SetParallelMode`)
- `register/` — Register definitions, `ScaleFunc` for context-aware scaling, fault code lookup
- `inverter/` — Multi-inverter system: `System` (connect/snapshot/write), `Snapshot` (typed data structs with JSON tags), aggregation logic, settings read/write with encoding
- `serve/` — Long-running service: polling hub, MQTT publisher, web dashboard (HTMX/SSE/Beer CSS), REST API, settings editor
- `cmd/` — Cobra CLI commands: read, write, dump, info, scan, probe, serve
- `docs/` — Protocol documentation, register maps, undocumented register research

### Key Design Decisions

- **`modbus.Client` interface** — all CLI/API code uses this, not solarman directly. Future RS485 transport just needs to implement the 5 methods (Connect, Close, ReadRegisters, WriteSingleRegister, WriteMultipleRegisters).
- **`modbus.Session`** — wraps a Client with per-connection register cache and automatic retry with exponential backoff (`cenkalti/backoff/v5`) on I/O timeouts. Only timeout errors are retried; MODBUS errors (illegal address, etc.) fail immediately. Cache is invalidated on any write.
- **`modbus.Lookup`** — `func(addr uint16) (uint16, error)`. Passed to `ScaleFunc` so scalers can resolve dependent registers on demand (e.g., `Voltage12V` reads 0xE003 for system voltage). Results are cached per session.
- **`register.ScaleFunc`** — `func(raw float64, lookup modbus.Lookup) float64`. Reusable instances: `Mul1`, `Mul01`, `Mul001`, `Mul10`, `Voltage12V`.
- **Capability probing** — registers marked `Optional: true` are probed individually at runtime rather than gated by protocol version. ASP and ASF series use independent protocol version counters (ASP V1.07 supports ASF V1.7+ registers). Optional registers that fail are silently omitted. The `probe` command can scan arbitrary address ranges to discover undocumented registers.
- **Bulk reads with smart batching** — register groups are read in contiguous spans up to 32 registers. Gaps >4 registers cause a span split. Optional registers always get their own span to isolate failures. Failed optional spans are skipped; failed required spans return errors.
- **Fault history** — 16 fault records at 0xF800-0xF8FF (16 registers each) with fault code, timestamp, and system snapshot. Fault codes are named from the ASP user manual.
- **Interleaved response filtering** — Solarman V5 `sendAndReceive` filters responses by matching function code to skip interleaved responses from other services (ha-solarman, cloud) sharing the dongle connection.
- **Serialized MODBUS access** — all reads and writes go through the `Hub` run loop to prevent concurrent access to the dongle, which can't handle overlapping requests.
- **Typed settings writes** — `inverter.WriteSetting(field, value)` accepts human-readable field names and values (e.g., `"boost_charge_voltage"`, `"53.6"`). The server handles encoding: scale factors, 12V-base voltage math, time packing (HH:MM → packed uint16), boolean conversion.

### Inverter Package (`inverter/`)

- **`System`** — manages `[]modbus.Client`, wraps each in a `Session`. `Init()` reads product info and detects parallel mode. `Snapshot()` reads all units concurrently and aggregates. `WriteSetting()` encodes and writes to all units. `ReadSettings()` returns typed settings structs. `ReadFaults()` returns fault history.
- **`Snapshot`** — aggregated system state with typed fields: `BatteryData`, `PVData`, `LoadData`, `GridData` (per-phase `PhaseData`), `InverterData`, `StatsData`. All JSON-tagged.
- **Aggregation rules** — battery SOC/voltage: average. Power/current/energy: sum. Temperatures: max. AC voltage/frequency: average. Fault bits: OR. Load ratio: sum. Machine state: worst.
- **Stale fallback** — failed units return previous snapshot data. After `MaxStaleSnapshots` (3) consecutive failures, `UnitSnapshot.Stale = true`.

### Serve Package (`serve/`)

- **`Hub`** — polling loop with fan-out to subscribers. Serializes all MODBUS access (polls, settings reads, writes) through a single run loop. Buffered subscriber channels (cap 2) with non-blocking send.
- **`WebServer`** — stdlib `net/http` with `go:embed` templates. Routes: `/` (dashboard), `/settings` (editable), `/faults` (history), `/api/snapshot`, `/api/settings`, `/api/settings/write`, `/api/faults`, `/api/snapshot/stream` (SSE).
- **SSE** — per-connection previous-snapshot diffing. Changed values get `value-changed` CSS class server-side (green pulse animation, 2.5s). No client-side JS for change detection.
- **Settings editor** — staged commit flow: edit values inline, dirty fields highlighted, floating Apply bar appears, confirmation dialog lists all changes (old → new), batch POST writes sequentially through hub, spinner during write, result dialog.
- **MQTT** — `paho.mqtt.golang`. HA auto-discovery with one JSON state topic per device, individual sensors use `value_template`. LWT for availability. Sensor registry covers ~20 entities (SOC, voltage, current, power, temps, energy stats).
- **Config** — TOML (`BurntSushi/toml`). `[server]` (poll interval, web port, settings refresh), `[[inverter]]` (host, port, serial, slave ID), optional `[mqtt]` (broker, client ID, topic prefix).
- **Startup** — web server starts immediately (shows loading spinner), inverter connections happen after, `SetSystem()` wires up when ready.

### Split-Phase Register Layout

L2/L3 registers are **interleaved by type**, not contiguous blocks:
```
0x022A Grid Voltage L2    0x022B Grid Voltage L3
0x022C Inv Voltage L2     0x022D Inv Voltage L3
0x022E Inv Current L2     0x022F Inv Current L3
0x0230 Load Current L2    0x0231 Load Current L3
0x0232 Load Power L2      0x0233 Load Power L3
0x0234 Load App Power L2  0x0235 Load App Power L3
0x0236 Load Ratio L2      0x0237 Load Ratio L3
0x0238 Grid Current L2    0x0239 Grid Current L3
```
This was verified against live hardware and matches the ha-solarman SRNE profile + V1.7 protocol PDF.

## Protocol Stack

Communication flows: **CLI/API → modbus.Session (cache+retry) → modbus.Client → Solarman V5 (TCP:8899) → MODBUS RTU → Inverter**

### Solarman V5 Protocol
- Wraps MODBUS RTU frames in a TCP envelope. See `docs/solarman_v5_protocol.md`.
- **Dongle serial number is required** — without it, the dongle ACKs but doesn't forward to the inverter. Auto-detected on first request: send with serial=0, read the ACK to learn the serial from response bytes 7-10, then resend. Serial is scoped to the TCP connection.
- Other services (ha-solarman, cloud) may share the dongle connection. Their responses interleave — filter by function code, slave ID, and CRC.

### SRNE MODBUS
- Function codes: 0x03 (read), 0x06 (write single), 0x10 (write multiple), 0x78 (factory reset), 0x79 (clear history)
- 9600 baud, 8N1, slave default 1, max 32 registers per read
- CRC16 polynomial 0xA001, low byte first
- Voltage settings stored in 12V-base (multiply by systemVoltage/12)
- See `docs/srne_modbus_registers.md` for full register map and undocumented register research

### Key Register Ranges
- `0x000A-0x0048` — Product info (read-only)
- `0x0100-0x011D` — Battery/PV realtime data, BMS data (read-only)
- `0x0200-0x0239` — Inverter/grid/load data, L1/L2/L3 interleaved (read-only)
- `0xDF00-0xDF0D` — Device control (write-only: power, reset, sleep)
- `0xE001-0xE039` — Battery settings incl. temp limits, SOC thresholds (read/write)
- `0xE026-0xE04C` — Timed charge/discharge with per-section SOC/voltage/power cutoffs
- `0xE100-0xE149` — Undocumented settings block (possibly grid protection / split-phase tuning)
- `0xE200-0xE221` — Inverter settings (read/write)
- `0xE400-0xE431` — Grid-connection parameters (mostly dormant in off-grid mode)
- `0xF000-0xF055` — Statistics and historical data (read-only)
- `0xF800-0xF8FF` — Fault history with 16 records (read-only)

### Protocol Version Caveat
The ASP series uses an independent protocol version counter from the ASF series. ASP V1.07 ≠ ASF V1.07. The ASP firmware (SW V8.18) supports registers documented up to ASF V1.7+ despite reporting V1.07. Do not use the protocol version number to gate register availability — use runtime probing instead.

## CI

- GitHub Actions: build, test (with race detector + coverage), vet, lint
- `dorny/test-reporter@v3` — per-test pass/fail Check annotations (reporter: `golang-json`)
- `vladopajic/go-test-coverage@v2` — coverage report + badge on `badges` branch

## Reference

- SRNE ASP 8-10kW User Manual V1.3: https://www.srnesolar.com/userfiles/files/2025/11/28/ASP%20_8-10kW_U_All-in-one%20solar%20charge%20inverter_V1.3[20250514].pdf
- SRNE MODBUS protocol PDFs: https://github.com/shakthisachintha/SRNE-Hybrid-Inverter-Monitor/tree/master/Resources
- Prior art (ha-solarman): https://github.com/davidrapan/ha-solarman — Solarman V5 protocol implementation, SRNE inverter profile
- V1.96 protocol with changelog: https://github.com/krimsonkla/srne_ble_modbus — includes version history showing when each register was added
- V2.08 protocol + ESPHome YAML: https://github.com/phinix-org/SRNE-inverters-by-modbus-rs485 — latest known protocol PDF
- V1.7 CSV register list: HotNoob/PythonProtocolGateway
