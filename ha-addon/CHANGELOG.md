# Changelog

## 1.1.1

- Auto-reconnect dropped dongle TCP connections instead of staying permanently disconnected
- Skip settings refresh when all units are stale to reduce log noise during outages

## 1.0.2

- Fix s6-overlay startup (add `init: false`)
- Add service startup and timeout configuration
- Add `.dockerignore` for faster builds

## 1.0.0

- Initial release
- Web dashboard with HTMX + SSE live updates
- MQTT auto-discovery with ~65 sensors and 7 writable controls
- Multi-inverter parallel system support
- CLI for direct register access
