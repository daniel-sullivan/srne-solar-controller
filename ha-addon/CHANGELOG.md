# Changelog

## 1.1.9

- Add a **Timed Charge (Grid)** switch (`switch.timed_charge_enable`, register `0xE02C`) so an external controller (e.g. an off-peak optimiser) can trigger grid battery charging without touching the charger-priority setting. Turning it ON pins timed-charge section 1 to a full-day window (`00:00`–`23:59`) then enables `0xE02C`; turning it OFF clears `0xE02C` only. Charge rate is still governed by **Mains Charge Current** — this switch only gates whether the inverter's timed-charge feature is active. Note: `timed_charge_enable` and `charge_from_mains` are independent registers, so leaving one enabled while disabling the other may not stop grid charging.
- Add a **Timed Charge Window** diagnostic sensor showing the configured section-1 start/end times.

## 1.1.8

- Fix **Charger Priority** writes in parallel systems: the setting is now written only to the master unit instead of all units. Writing to non-master inverters in a parallel group is ignored by the hardware and could cause conflicts; the master propagates the mode to subordinate units automatically.

## 1.1.7

- The **Charge from Mains** switch is now backed by the AC charge current limit (`0xE205`) instead of the charger-priority register (`0xE20F`). Turning it OFF writes `0 A`, which deterministically stops grid charging regardless of priority/output mode; turning it ON restores the last non-zero limit (or 60 A if none has been seen). This fixes the switch reverting from ON→OFF by itself on some systems: the old behaviour toggled a *priority* enum whose value is easily rewritten by the inverter's own logic or an external surplus controller (e.g. EVCC), so the next poll would read a value that no longer matched "on". The switch and the `Mains Charge Current` number entity now stay consistent — setting the number to 0 shows the switch OFF, and vice-versa.
- Relabel the **Charger Priority** select options from the opaque `CSO`/`CUB`/`SNU`/`OSO` codes to plain language matching the V2.04 protocol table: `PV Priority` / `AC Priority` / `Hybrid` / `PV Only` (register values 0/1/2/3 unchanged). HA will re-discover the entity with the new option names.

## 1.1.6

- Mains Charge Current is now exposed as a writable Home Assistant `number` entity (`number.mains_charge_current`, register `0xE205`, 0–200 A range). Previously this AC charge-current limit could only be set from the web dashboard; it can now be driven from HA automations (e.g. raising/lowering the limit to track PV surplus) and is published with live state via MQTT discovery.
- Fix a data race in the fault-history endpoints: `/faults` and `/api/faults` read fault records directly from the inverter session, concurrently with the polling loop, which could corrupt the shared session and issue overlapping MODBUS requests to the dongle. Fault reads are now serialized through the hub run loop like polls and settings writes.
- Fix temperature limit settings (`charge_min_temp`, `charge_max_temp`, `discharge_min_temp`, `discharge_max_temp`) not accepting negative values — they were encoded as unsigned, so e.g. `-10` would write an incorrect register value.
- Fix parallel-system temperature aggregation: heatsink and battery temperatures now seed from the first unit rather than 0, so the reported maximum is correct when any unit reads a temperature below the previous seed value.
- Add `driver = "mock"` support in `[[inverter]]` config blocks to run a live simulator without real hardware; a sample `srne-mock.toml` is included.
- Fix unbounded memory growth in long-running deployments: the Solarman client now compacts its receive buffer after each transaction.

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
