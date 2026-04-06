# Changelog

## 1.1.2

- Separate per-phase load data from grid data in snapshot structs
  - Per-phase load sensors now under `load.l1`/`load.l2`/`load.l3` (was `grid.l1`/`grid.l2`/`grid.l3`)
  - MQTT discovery paths updated: e.g. `load.l1.power` replaces `grid.l1.load_power`
  - **Breaking:** HA entities using old JSON paths will need re-discovery
- Update GitHub Actions to Node.js 24 compatible versions
- Fix Dockerfile `InvalidDefaultArgInFrom` lint warning

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
