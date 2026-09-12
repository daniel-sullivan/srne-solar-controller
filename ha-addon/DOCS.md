# SRNE Solar Controller

Monitor and control SRNE ASP/ASF-series hybrid inverters via Solarman V5 wifi dongles.

## Features

- Real-time web dashboard with live updates (HTMX + SSE)
- MQTT integration with Home Assistant auto-discovery
- Multi-inverter support (parallel split-phase systems)
- Settings editor with staged commit flow
- Fault history viewer
- REST API

## Configuration

### Inverters

Add one entry per Solarman wifi dongle:

- **host**: IP address of the Solarman dongle on your LAN
- **port**: Solarman V5 port (default: 8899)
- **slave_id**: MODBUS slave ID (default: 1, usually 1-2 for parallel setups)
- **serial**: Dongle serial number (0 = auto-detect)
- **mppt1_label** / **mppt2_label**: Optional friendly names for the two MPPT inputs
  on this inverter (e.g. "Roof East", "Roof West"). Used as the display label on the
  dashboard and as the per-unit MQTT entity name (e.g. "Roof East Voltage"). Leave
  blank to keep the default "MPPT 1" / "MPPT 2".

### MQTT

MQTT is enabled by default. If you have the Mosquitto add-on installed, the broker
credentials are auto-discovered — no configuration needed.

To use an external broker, set `mqtt_broker` to its URL (e.g., `tcp://192.168.1.100:1883`)
and provide `mqtt_username`/`mqtt_password` if required.

### Poll Interval

How often (in seconds) to read data from the inverters. Default: 10 seconds.

### BMS (JBD UP16S Inter-pack Monitor)

Optional serial monitor for the JBD UP16S inter-pack RS485 bus:

- **bms_serial_device**: Serial port path for the inter-pack RS485 bus (e.g. `/dev/ttyUSB0`).
  When set, the service reads inter-pack telemetry and BMS balance/protection settings via read-only queries, exposing them through `GET /api/bms`. Leave blank if no BMS serial adapter is connected.

### Manual battery conditioning

When the JBD monitor is configured, the Conditioning page offers a manual cycle for this installation’s two parallel SRNE inverters and ten connected 48 V/100 Ah packs. Each inverter is capped at **10 A** (nominal **20 A** combined). The controller holds 54.4 V, 54.8 V, then 55.2 V, using the nine reporting packs’ cell voltages and alarms to decide when to advance or stop. One connected pack does not report over RS485; its local BMS cutoff still operates, but its cell balance cannot be observed by this service.

The add-on keeps the exact prior inverter settings in `/data/conditioning-state.json` before changing any settings. Manual stop, completion, a safety fault, or an 8-hour timeout triggers restoration. If a unit is unavailable, restoration remains pending and the journal is retained for retry. Do not remove that file while restoration is pending. The cycle does not start on add-on launch.

## Network

This add-on uses **host networking** to communicate with Solarman dongles on your local
network. The dongles must be reachable from your Home Assistant host on port 8899.

## Web Dashboard

The dashboard is accessible via the Home Assistant sidebar (ingress). It shows real-time
battery, PV, load, and grid data with automatic change highlighting.

The **Bank** tab shows all 16 cells per reporting pack, each pack's cell spread,
and live pack-to-pack and bank-wide cell voltage deltas. It keeps missing or stale
packs visibly unavailable. The connected pack without RS485 data is shown as an
unmonitored card with no invented cell readings. Configure `bms_serial_device`
to populate this view.
