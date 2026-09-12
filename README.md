# SRNE Solar Controller

[![CI](https://github.com/daniel-sullivan/srne-solar-controller/actions/workflows/ci.yml/badge.svg)](https://github.com/daniel-sullivan/srne-solar-controller/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/daniel-sullivan/srne-solar-controller/graph/badge.svg)](https://codecov.io/gh/daniel-sullivan/srne-solar-controller)
[![Go Report Card](https://goreportcard.com/badge/github.com/daniel-sullivan/srne-solar-controller)](https://goreportcard.com/report/github.com/daniel-sullivan/srne-solar-controller)

Monitor and control SRNE ASP/ASF-series hybrid inverters via MODBUS over Solarman V5 wifi dongles.

Integrates with Home Assistant through MQTT auto-discovery, providing sensor and control entities. Also includes a live web dashboard and CLI for direct register access. Supports multi-inverter parallel systems with aggregated data and synchronized settings writes.

## Home Assistant Add-on

### Prerequisites

- A Solarman V5 wifi dongle (LSW-3, LSE-3, etc.) connected to your inverter
- The [Mosquitto broker](https://github.com/home-assistant/addons/tree/master/mosquitto) add-on (or an external MQTT broker)

### Installation

Add this repository URL to your Home Assistant add-on store:

```
https://github.com/daniel-sullivan/srne-solar-controller
```

Install the add-on, then configure your inverter connection(s) in the add-on settings:

| Option | Description |
|---|---|
| `inverters[].host` | IP address of the Solarman dongle |
| `inverters[].port` | TCP port (default 8899) |
| `inverters[].slave_id` | MODBUS slave ID (default 1) |
| `inverters[].serial` | Dongle serial number (0 = auto-detect) |
| `poll_interval` | Seconds between polling cycles (default 10) |

MQTT is auto-configured from the Mosquitto add-on. Optional overrides (`mqtt_broker`, `mqtt_topic_prefix`, `mqtt_username`, `mqtt_password`) are available for external brokers.

Start the add-on. Sensor and control entities appear automatically under the MQTT integration.

### Auto-Discovered Entities

**Sensors (~65 entities):** Battery SOC, voltage, current, power, temperature. PV1/PV2 voltage, current, power. Load power, apparent power, power factor. Grid voltage, current, frequency per phase (L1/L2). Inverter state, bus voltage, heatsink temperatures. Daily and lifetime energy statistics.

**Controls:**

| Entity | Type | Description |
|---|---|---|
| Charge from Mains | Switch | Toggle grid charging. Remembers previous charger priority and restores it on OFF. |
| Charger Priority | Select | CSO (PV Preferred) / CUB (Mains Preferred) / SNU (Hybrid) / OSO (PV Only) |
| Output Priority | Select | SOL (Solar First) / UTI (Utility First) / SBU (Solar, Battery, Utility) |
| Max Charge SOC | Number | Stop charging at this SOC (0-100%) |
| Discharge Cutoff SOC | Number | Hard minimum battery SOC (0-100%) |
| SOC Switch to Mains | Number | Switch to grid power below this SOC (0-100%) |
| SOC Switch to Battery | Number | Switch back to battery above this SOC (0-100%) |
| Battery Conditioning | Switch | Manually start or stop the staged two-inverter balance cycle when the JBD monitor is configured. |

### Web Dashboard

The add-on includes a web dashboard accessible via the HA sidebar (ingress). It provides real-time monitoring with live-updating panels and a full settings editor for all inverter parameters.

The **Bank** page shows each reporting pack's 16 cell voltages, pack voltage, lowest/highest cell, and within-pack cell delta. Its summary compares the lowest and highest reporting pack voltages and cells. Stale readings are marked unavailable and excluded from the live deltas. The tenth connected pack has a separate unavailable card because its BMS does not report over RS485.

### Manual battery conditioning

The Conditioning page provides a manual balance cycle for the installed bank of ten 48 V/100 Ah 16-cell packs and two parallel SRNE inverters. It requires the JBD UP16S RS485 monitor and a persistent restoration journal. Nine packs currently report on RS485 (addresses 0–7 and 9); the tenth remains connected but its RS485 telemetry is unavailable. Its local BMS protection still operates, but the controller cannot observe its cells or certify its balance.

The cycle temporarily suspends inverter CAN BMS communication, caps **each** inverter at **10 A** (nominal **20 A total**), and holds 54.4 V, 54.8 V, then 55.2 V. It advances after at least 30 minutes at each measured voltage when every monitored pack has at most 30 mV cell spread. At 55.2 V it completes only after all monitored cells are at least 3.4 V, every pack has at most 20 mV spread, and combined charge current remains at or below 10 A for 30 minutes. A monitored cell reaching 3.55 V, a BMS alarm, stale telemetry/settings, or the 8-hour limit stops the cycle. On stop or fault, the service restores each inverter’s saved voltage, equalization, CAN, and charge-current settings; an unfinished restoration remains in the journal for retry on restart.

For standalone use, configure both sections before starting the service:

```toml
[bms]
serial_device = "/dev/ttyUSB0"

[conditioning]
state_file = "/data/conditioning-state.json"
```

The Home Assistant add-on always keeps `/data/conditioning-state.json` available for restoration. Starting a new cycle also requires `bms_serial_device`. The cycle never starts automatically.

## Supported Hardware

SRNE ASP and ASF series hybrid inverters connected via Solarman V5 wifi dongles. Communication is over TCP port 8899 using the Solarman V5 protocol wrapping MODBUS RTU.

Tested with ASP48100U200-H units in split-phase 120/240V parallel configuration.

## Standalone Usage

The controller can also run outside Home Assistant as a standalone service or CLI tool.

### Prerequisites

- [mise](https://mise.jdx.dev/) (manages the Go toolchain)

```sh
mise install
```

### Service Mode

Create a `srne.toml` config file:

```toml
[server]
poll_interval = "10s"
web_port = 8080
settings_refresh = "5m"

[[inverter]]
host = "10.100.3.92"
port = 8899
slave_id = 1

[[inverter]]
host = "10.100.5.55"
port = 8899
slave_id = 2

[mqtt]
broker = "tcp://localhost:1883"
client_id = "srne"
topic_prefix = "homeassistant"
```

```sh
mise run serve
```

The web dashboard is available at `http://localhost:8080`. The MQTT section is optional when running standalone.

### CLI

```sh
# Display inverter product info
mise exec -- go run . info --host 10.100.3.92

# Dump all register groups
mise exec -- go run . dump --host 10.100.3.92 all

# Dump a specific group: battery, inverter, settings, stats, faults, timed
mise exec -- go run . dump --host 10.100.3.92 battery

# Read specific registers
mise exec -- go run . read --host 10.100.3.92 0x0100 10

# Write a register
mise exec -- go run . write --host 10.100.3.92 0xE001 0x0036

# Probe for undocumented registers in a range
mise exec -- go run . probe --host 10.100.3.92 0xE100 0xE150

# Passively scan dongle traffic
mise exec -- go run . scan --host 10.100.3.92
```

Global flags: `--host` (required), `--port` (default 8899), `--serial` (auto-detected if omitted), `--slave` (default 1), `--debug`.

### REST API

| Endpoint | Method | Description |
|---|---|---|
| `/api/snapshot` | GET | Current system snapshot (JSON) |
| `/api/snapshot/stream` | GET | SSE stream of snapshots |
| `/api/settings` | GET | Current inverter settings |
| `/api/settings/write` | POST | Write settings (`{"changes": [{"field": "...", "value": "..."}]}`) |
| `/api/faults` | GET | Fault history records |
| `/api/entities` | GET | All entity metadata (sensors + controls) with current state |
| `/api/controls/{key}` | POST | Write a control value (`{"value": "..."}`) |
| `/api/bms` | GET | Read-only JBD pack telemetry and settings |
| `/api/conditioning` | GET | Manual conditioning and restoration status |
| `/api/conditioning/start` | POST | Explicitly start a conditioning cycle after safety checks |
| `/api/conditioning/stop` | POST | Stop and restore saved inverter settings |

## Architecture

```
CLI/API -> modbus.Session (cache + retry) -> modbus.Client -> Solarman V5 (TCP:8899) -> MODBUS RTU -> Inverter
```

| Package | Role |
|---|---|
| `modbus/` | MODBUS RTU framing, CRC16, `Client` interface, `Session` (register cache + exponential backoff retry) |
| `interfaces/solarman/` | Solarman V5 TCP transport with serial auto-detection, interleaved response filtering, and automatic reconnection |
| `interfaces/mock/` | Mock inverter and live simulator for testing |
| `register/` | Register definitions, context-aware scaling (`ScaleFunc`), fault code lookup |
| `inverter/` | Multi-inverter system management, typed snapshots, aggregation, settings encoding |
| `bms/interpack/` | Read-only JBD UP16S RS485 telemetry and installed settings |
| `conditioning/` | Deterministic staged-CV decisions and safety gates |
| `serve/` | Polling hub, MQTT publisher with HA control entities, web dashboard (HTMX/SSE), REST API |
| `cmd/` | Cobra CLI: `read`, `write`, `dump`, `info`, `scan`, `probe`, `serve` |

## Building

```sh
mise exec -- go build ./...
mise exec -- go test ./...
```

## References

- [SRNE ASP 8-10kW User Manual](https://www.srnesolar.com/userfiles/files/2025/11/28/ASP%20_8-10kW_U_All-in-one%20solar%20charge%20inverter_V1.3[20250514].pdf)
- [SRNE MODBUS Protocol PDFs](https://github.com/shakthisachintha/SRNE-Hybrid-Inverter-Monitor/tree/master/Resources)
- [ha-solarman](https://github.com/davidrapan/ha-solarman) -- Solarman V5 protocol implementation, SRNE inverter profile
- [V2.08 Protocol + ESPHome YAML](https://github.com/phinix-org/SRNE-inverters-by-modbus-rs485) -- Latest known protocol PDF

## License

See [LICENSE](LICENSE) for details.
