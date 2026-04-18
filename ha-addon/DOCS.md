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

## Network

This add-on uses **host networking** to communicate with Solarman dongles on your local
network. The dongles must be reachable from your Home Assistant host on port 8899.

## Web Dashboard

The dashboard is accessible via the Home Assistant sidebar (ingress). It shows real-time
battery, PV, load, and grid data with automatic change highlighting.
