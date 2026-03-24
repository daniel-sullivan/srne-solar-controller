# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go application for interfacing with SRNE ASF-series hybrid inverters via MODBUS protocol over Solarman V5 wifi adaptors.

**Planned interfaces** (in priority order):
1. CLI — dump/modify registers, verify communication
2. REST API
3. MQTT event publishing
4. TypeScript/React frontend
5. Home Assistant integration

**Multi-inverter:** Systems can run in parallel — multiple read endpoints, single master for writes.

**Test hardware:** SRNE ASP48100U200-H (product type 4, protocol V1.07, SW V8.18)

## Environment Setup

- **mise** manages the Go toolchain version
- Run `mise install` to set up the environment
- Prefix commands with `mise exec --` or use `mise shell` to activate the environment

## Commands

- **Build**: `mise exec -- go build ./...`
- **Run tests**: `mise exec -- go test ./...`
- **Run a single test**: `mise exec -- go test ./... -run TestName`

## Architecture

- `modbus/` — MODBUS RTU framing, CRC16, `Client` interface, `Session` (per-connection register cache)
- `solarman/` — Solarman V5 TCP transport implementing `modbus.Client`
- `register/` — Register definitions with `ScaleFunc` for context-aware value scaling
- `cmd/` — Cobra CLI commands (read, write, dump, info, scan)

### Key Design Decisions

- **`modbus.Client` interface** — all CLI/API code uses this interface, not the solarman package directly. Future RS485 transport just needs to implement it.
- **`modbus.Session`** — wraps a Client with per-connection register cache. Scale functions receive a `modbus.Lookup` to resolve dependent registers on demand (e.g., system voltage for 12V-base scaling). Cache is invalidated on any write.
- **`register.ScaleFunc`** — `func(raw float64, lookup modbus.Lookup) float64`. Reusable instances: `Mul1`, `Mul01`, `Mul001`, `Mul10`, `Voltage12V`. The `Voltage12V` scaler reads register 0xE003 via the session cache to apply systemVoltage/12 multiplier.
- **Bulk reads with smart batching** — register groups are read in contiguous spans up to 32 registers, with gaps >4 registers causing a span split.
- **Capability probing** — registers marked `Optional: true` are probed individually at runtime rather than gated by protocol version. This is necessary because ASP and ASF series use independent protocol version counters (ASP V1.07 supports registers from ASF V1.7+). Optional registers that fail to read are silently omitted from output. The frontend should eventually use the same probing to determine available controls.

## Protocol Stack

Communication flows: **CLI/API → Solarman V5 (TCP:8899) → MODBUS RTU → Inverter**

### Solarman V5 Protocol
- Wraps MODBUS RTU frames in a TCP envelope. See `docs/solarman_v5_protocol.md`.
- **Dongle serial number is required** — without it, the dongle ACKs but doesn't forward to the inverter. Auto-detected on first request: send with serial=0, read the ACK to learn the serial from response bytes 7-10, then resend.
- Other services (ha-solarman, cloud) may share the dongle connection. Their responses interleave — filter by slave ID and CRC.

### SRNE MODBUS
- Function codes: 0x03 (read), 0x06 (write single), 0x10 (write multiple), 0x78 (factory reset), 0x79 (clear history)
- 9600 baud, 8N1, slave default 1, max 32 registers per read
- CRC16 polynomial 0xA001, low byte first
- Voltage settings stored in 12V-base (multiply by systemVoltage/12)
- See `docs/srne_modbus_registers.md` for full register map

### Key Register Ranges
- `0x000A-0x0049` — Product info (read-only)
- `0x0100-0x0111` — Battery/PV realtime data (read-only)
- `0x0200-0x0237` — Inverter/grid/load data (read-only)
- `0xDF00-0xDF0D` — Device control (write-only: power, reset, sleep)
- `0xE001-0xE025` — Battery settings (read/write)
- `0xE026-0xE04D` — Timed charge/discharge with per-section SOC/voltage/power cutoffs
- `0xE200-0xE21B` — Inverter settings (read/write)
- `0xF000-0xF04B` — Statistics/historical data (read-only)
- `0xF800-0xF9FF` — Fault history (read-only)

## Reference

- SRNE MODBUS protocol PDFs: https://github.com/shakthisachintha/SRNE-Hybrid-Inverter-Monitor/tree/master/Resources
- Prior art (ha-solarman): https://github.com/davidrapan/ha-solarman — Solarman V5 protocol implementation, SRNE inverter profile
- V1.96 protocol with changelog: https://github.com/krimsonkla/srne_ble_modbus — full protocol as markdown in `/resources/`, includes version history showing when each register was added
- V2.08 protocol + ESPHome YAML: https://github.com/phinix-org/SRNE-inverters-by-modbus-rs485 — latest known protocol PDF, complete ESPHome register definitions
- V1.7 CSV register list: HotNoob/PythonProtocolGateway
