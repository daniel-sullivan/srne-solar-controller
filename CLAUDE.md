# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go application for interfacing with SRNE ASP/ASF-series hybrid inverters via MODBUS protocol over Solarman V5 wifi adaptors.

**Planned interfaces** (in priority order):
1. CLI — dump/modify registers, verify communication (done)
2. REST API
3. MQTT event publishing
4. TypeScript/React frontend
5. Home Assistant integration

**Multi-inverter:** Systems can run in parallel — multiple read endpoints, single master for writes.

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

## Architecture

- `modbus/` — MODBUS RTU framing, CRC16, `Client` interface, `Session` (register cache + retry)
- `solarman/` — Solarman V5 TCP transport implementing `modbus.Client`
- `register/` — Register definitions, `ScaleFunc` for context-aware scaling, fault code lookup
- `cmd/` — Cobra CLI commands: read, write, dump, info, scan, probe
- `docs/` — Protocol documentation, register maps, undocumented register research

### Key Design Decisions

- **`modbus.Client` interface** — all CLI/API code uses this, not solarman directly. Future RS485 transport just needs to implement the 5 methods (Connect, Close, ReadRegisters, WriteSingleRegister, WriteMultipleRegisters).
- **`modbus.Session`** — wraps a Client with per-connection register cache and automatic retry with exponential backoff (`cenkalti/backoff/v5`) on I/O timeouts. Only timeout errors are retried; MODBUS errors (illegal address, etc.) fail immediately. Cache is invalidated on any write.
- **`modbus.Lookup`** — `func(addr uint16) (uint16, error)`. Passed to `ScaleFunc` so scalers can resolve dependent registers on demand (e.g., `Voltage12V` reads 0xE003 for system voltage). Results are cached per session.
- **`register.ScaleFunc`** — `func(raw float64, lookup modbus.Lookup) float64`. Reusable instances: `Mul1`, `Mul01`, `Mul001`, `Mul10`, `Voltage12V`.
- **Capability probing** — registers marked `Optional: true` are probed individually at runtime rather than gated by protocol version. ASP and ASF series use independent protocol version counters (ASP V1.07 supports ASF V1.7+ registers). Optional registers that fail are silently omitted. The `probe` command can scan arbitrary address ranges to discover undocumented registers.
- **Bulk reads with smart batching** — register groups are read in contiguous spans up to 32 registers. Gaps >4 registers cause a span split. Optional registers always get their own span to isolate failures. Failed optional spans are skipped; failed required spans return errors.
- **Fault history** — 16 fault records at 0xF800-0xF8FF (16 registers each) with fault code, timestamp, and system snapshot. Fault codes are named from the ASP user manual.

## Protocol Stack

Communication flows: **CLI/API → modbus.Session (cache+retry) → modbus.Client → Solarman V5 (TCP:8899) → MODBUS RTU → Inverter**

### Solarman V5 Protocol
- Wraps MODBUS RTU frames in a TCP envelope. See `docs/solarman_v5_protocol.md`.
- **Dongle serial number is required** — without it, the dongle ACKs but doesn't forward to the inverter. Auto-detected on first request: send with serial=0, read the ACK to learn the serial from response bytes 7-10, then resend. Serial is scoped to the TCP connection.
- Other services (ha-solarman, cloud) may share the dongle connection. Their responses interleave — filter by slave ID and CRC.

### SRNE MODBUS
- Function codes: 0x03 (read), 0x06 (write single), 0x10 (write multiple), 0x78 (factory reset), 0x79 (clear history)
- 9600 baud, 8N1, slave default 1, max 32 registers per read
- CRC16 polynomial 0xA001, low byte first
- Voltage settings stored in 12V-base (multiply by systemVoltage/12)
- See `docs/srne_modbus_registers.md` for full register map and undocumented register research

### Key Register Ranges
- `0x000A-0x0048` — Product info (read-only)
- `0x0100-0x011D` — Battery/PV realtime data, BMS data (read-only)
- `0x0200-0x0239` — Inverter/grid/load data, L1/L2/L3 (read-only)
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

## Reference

- SRNE ASP 8-10kW User Manual V1.3: https://www.srnesolar.com/userfiles/files/2025/11/28/ASP%20_8-10kW_U_All-in-one%20solar%20charge%20inverter_V1.3[20250514].pdf
- SRNE MODBUS protocol PDFs: https://github.com/shakthisachintha/SRNE-Hybrid-Inverter-Monitor/tree/master/Resources
- Prior art (ha-solarman): https://github.com/davidrapan/ha-solarman — Solarman V5 protocol implementation, SRNE inverter profile
- V1.96 protocol with changelog: https://github.com/krimsonkla/srne_ble_modbus — includes version history showing when each register was added
- V2.08 protocol + ESPHome YAML: https://github.com/phinix-org/SRNE-inverters-by-modbus-rs485 — latest known protocol PDF
- V1.7 CSV register list: HotNoob/PythonProtocolGateway
