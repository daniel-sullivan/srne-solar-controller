# Changelog

## Unreleased

- Mains Charge Current is now exposed as a writable Home Assistant `number` entity (`number.mains_charge_current`, register `0xE205`, 0–200 A range). Previously this AC charge-current limit could only be set from the web dashboard; it can now be driven from HA automations (e.g. raising/lowering the limit to track PV surplus) and is published with live state via MQTT discovery.
- Fix a data race in the fault-history endpoints: `/faults` and `/api/faults` read fault records directly from the inverter session, concurrently with the polling loop, which could corrupt the shared session and issue overlapping MODBUS requests to the dongle. Fault reads are now serialized through the hub run loop like polls and settings writes.

## 1.1.5

- Load power factor is now computed from real / apparent power (the 0x021A register reads 0 on ASP hardware); aggregate PF is sum(P)/sum(S) rather than an average across units
- Per-inverter MPPT labels: `mppt1_label` / `mppt2_label` in each `[[inverter]]` block override the dashboard's default "MPPT 1" / "MPPT 2", and rewrite the per-unit MQTT entity display names (e.g. "Roof East Voltage" instead of "PV1 Voltage"). Entity unique_ids are unchanged so existing HA entities survive the rename.

## 1.1.4

- Dashboard theme refresh: Home Assistant Material You palette (HA blue primary, tonal dark surfaces)
- Navigation moved from left drawer to fixed top bar, stays visible on mobile and is not occluded by the HA sidebar
- Merged Inverter card into Units card: bus voltage, DC-DC/DC-AC temp and machine state shown per unit
- Light / dark theme support: follows OS `prefers-color-scheme` by default, top-bar toggle persists in `localStorage`

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
