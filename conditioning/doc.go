// Package conditioning implements an isolated, deterministic, side-effect-free
// decision engine for staged Constant-Voltage (CV) battery bank conditioning on
// parallel SRNE hybrid inverters and JBD UP16S BMS battery packs.
//
// # Agreed Bank Topology and Operating Contract
//
// The physical bank contains 10 connected 16S 100Ah LiFePO4 packs. Nine packs report
// telemetry and verified settings through a shared JBD UP16S RS485 communication bus.
// The tenth pack (physical pack 8) has non-functioning RS485 communication, although
// its internal BMS cell sensing, balancing, and hardware charge cutoff remain functional.
//
// An operator exception has been explicitly accepted requiring exactly the 9 expected
// bus addresses: 0, 1, 2, 3, 4, 5, 6, 7, and 9. The engine strictly requires 9 distinct
// physical BMS identifiers with fresh telemetry and settings.
//
// # Invariants and Safety Guarantees
//
//   - Unmonitored Pack Visibility: The engine explicitly exposes that 1 connected pack
//     is unmonitored (TotalConnectedPacks = 10, ExpectedMonitoredPacks = 9, UnmonitoredPacks = 1).
//     The engine never asserts or implies that the unmonitored pack is safe or balanced.
//   - Terminal Voltage Independence: Bank-terminal voltage is never presumed to bound
//     individual cell voltages. Individual cell voltages across all monitored packs are
//     evaluated individually on every cycle.
//   - Explicit Address Contract: Monitored pack count is never silently inferred from
//     observed responders. The engine strictly verifies the explicit 9-address set {0..7, 9}.
//   - Inverter Telemetry Gating: Inverter telemetry validity (both-unit connectivity, fresh
//     non-zero timestamp, valid numbers) is verified on every active update at all stages.
//     Stale or invalid inverter telemetry immediately halts the session.
//   - Redacted Identifiers: Physical BMS identifiers are redacted from user-facing status,
//     reasons, and logs to avoid leaking hardware serial numbers.
//   - Idempotent Activation: Calling Start while StateRunning returns the current status
//     without mutating session state or resetting dwell timers.
//   - No Live Mutations: This package is a pure decision engine. It issues no charge writes,
//     initiates no network or serial I/O, and runs no goroutines or background timers.
//
// # Fail-Closed Safety Gates
//
// The engine immediately transitions to StateFaulted, cuts target voltage to 0V, and halts
// on any of the following conditions:
//   - Invalid, missing, or stale inverter telemetry (valid=false, age > DefaultMaxInverterAge, NaN/Inf).
//   - Missing, extra, or duplicate pack addresses (strictly requires {0, 1, 2, 3, 4, 5, 6, 7, 9}).
//   - Missing, empty, or duplicate physical BMS identifiers across packs.
//   - Stale telemetry (age exceeds DefaultMaxTelemetryAge) or unconfirmed address 0 responder.
//   - Stale or unavailable BMS settings (balance or protection block age exceeds DefaultMaxSettingsAge).
//   - Divergence from installed safety settings:
//   - Balance start: 3400 mV
//   - Balance delta: 30 mV
//   - Cell OVP alarm: 3600 mV
//   - Cell OVP protect: 3650 mV
//   - Pack OVP alarm: 57.60 V (5760 cV)
//   - Pack OVP protect: 58.40 V (5840 cV)
//   - Any non-zero fault mask or warning mask on any pack (including unverified bitfields).
//   - Any monitored cell voltage >= 3550 mV.
//
// # Staged CV Schedule and Stage Dwell Qualification
//
// Conditioning progresses through three staged Constant-Voltage absorption levels:
//   - Stage 1: 54.4V target voltage.
//     Advance condition: measured bank voltage must reach 54.40V within ±0.10V and dwell
//     continuously for at least 30 minutes, with all 9 monitored packs having max-min cell spread <= 30 mV.
//   - Stage 2: 54.8V target voltage.
//     Advance condition: measured bank voltage must reach 54.80V within ±0.10V and dwell
//     continuously for at least 30 minutes, with all 9 monitored packs having max-min cell spread <= 30 mV.
//   - Stage 3: 55.2V target voltage.
//     Completion condition: measured bank voltage within ±0.10V of 55.20V, all monitored cells >= 3400 mV,
//     each pack spread <= 20 mV, and total charge current <= 10A continuously for 30 minutes.
//
// If measured bank voltage falls away from the stage target by more than 0.10V (e.g. cloud or load event),
// the dwell/tail timer immediately resets to zero (fall-away reset). This prevents pre-balanced packs from
// declaring a false success without actually undergoing Constant-Voltage absorption.
//
// If total charge current rises above 10A, any cell drops below 3400 mV, or any pack spread exceeds 20 mV
// during Stage 3, the tail timer immediately resets to zero (tail reset).
//
// An overall 8-hour timeout bounds the conditioning session. If spreads do not settle or completion
// is not achieved within 8 hours from Start, the engine transitions to StateTimedOut.
//
// The total bank charging cap is 20A, representing a 10A current limit on each of the two
// parallel SRNE inverters (configured externally).
package conditioning
