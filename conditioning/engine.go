package conditioning

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
)

// Engine is an isolated, deterministic, side-effect-free conditioning decision engine.
// It maintains no background goroutines, channels, or timers; all state transitions
// are driven strictly by callers supplying an explicit time.Time and Input.
type Engine struct {
	state          State
	stage          Stage
	reason         string
	err            error
	startedAt      time.Time
	stageStartedAt time.Time
	dwellStartedAt time.Time
	tailStartedAt  time.Time
	lastUpdated    time.Time
	lastStatus     Status
}

// NewEngine creates a new conditioning Engine operating under the agreed fixed bank contract.
func NewEngine() *Engine {
	e := &Engine{
		state: StateIdle,
		stage: StageNone,
	}
	e.lastStatus = e.buildStatus(time.Time{}, Input{}, 0, 0, 0, 0, "engine initialized, awaiting Start()")
	return e
}

// InputFromStore creates an Input struct by taking a snapshot from the BMS store
// and attaching the provided inverter telemetry.
func InputFromStore(store *interpack.Store, now time.Time, inv InverterTelemetry, telemetryMaxAge, settingsMaxAge time.Duration) Input {
	if store == nil {
		return Input{Inverter: inv}
	}
	return Input{
		Packs:    store.Snapshot(now, telemetryMaxAge, settingsMaxAge),
		Inverter: inv,
	}
}

// Start begins a conditioning session at explicit time now, evaluating input immediately.
// If already in StateRunning, it acts idempotently and returns the current running status
// without mutating session state or resetting dwell timers.
// If any safety gate fails, it immediately transitions to StateFaulted.
func (e *Engine) Start(now time.Time, input Input) Status {
	if e.state == StateRunning {
		return e.lastStatus
	}

	e.state = StateRunning
	e.stage = Stage1
	e.err = nil
	e.startedAt = now
	e.stageStartedAt = now
	e.dwellStartedAt = time.Time{}
	e.tailStartedAt = time.Time{}
	e.lastUpdated = now
	e.reason = "stage 1 active (target 54.4V)"

	return e.evaluate(now, input)
}

// Update evaluates the next telemetry tick at explicit time now and returns current status.
// If already terminal (Completed, Faulted, TimedOut, Stopped), it returns the terminal status.
func (e *Engine) Update(now time.Time, input Input) Status {
	if e.state.IsTerminal() {
		return e.lastStatus
	}
	if e.state == StateIdle {
		e.lastStatus = e.buildStatus(now, input, 0, 0, 0, 0, "engine is idle; call Start() to begin")
		return e.lastStatus
	}

	// Guard against non-monotonic clock updates.
	if !e.lastUpdated.IsZero() && now.Before(e.lastUpdated) {
		return e.failClosed(now, input, packMetrics{}, fmt.Errorf("%w: received %v before last update %v", ErrNonMonotonicTime, now, e.lastUpdated))
	}
	e.lastUpdated = now

	// Check overall session timeout (8h).
	elapsed := now.Sub(e.startedAt)
	if elapsed >= OverallTimeout {
		e.state = StateTimedOut
		e.err = ErrTimeout
		e.reason = fmt.Sprintf("overall conditioning timeout exceeded (%v >= %v)", elapsed, OverallTimeout)
		e.lastStatus = e.buildStatus(now, input, 0, 0, 0, 0, e.reason)
		return e.lastStatus
	}

	return e.evaluate(now, input)
}

// Stop manually halts the conditioning engine at explicit time now.
func (e *Engine) Stop(now time.Time) Status {
	if e.state.IsTerminal() {
		return e.lastStatus
	}
	e.state = StateStopped
	e.err = ErrManualStop
	e.reason = "manual stop requested"
	e.lastStatus = e.buildStatus(now, Input{}, 0, 0, 0, 0, e.reason)
	return e.lastStatus
}

// Abort records an external safety failure while preserving its actual cause.
// The orchestration layer uses this when inverter register drift or transport
// failure occurs outside the telemetry decision gates.
func (e *Engine) Abort(now time.Time, err error) Status {
	if e.state.IsTerminal() {
		return e.lastStatus
	}
	if err == nil {
		err = errors.New("conditioning aborted")
	}
	e.state = StateFaulted
	e.err = err
	e.reason = err.Error()
	e.lastStatus = e.buildStatus(now, Input{}, 0, 0, 0, 0, e.reason)
	return e.lastStatus
}

// Status returns the most recent observable engine status.
func (e *Engine) Status() Status {
	return e.lastStatus
}

// Reset restores the engine back to StateIdle.
func (e *Engine) Reset() {
	e.state = StateIdle
	e.stage = StageNone
	e.err = nil
	e.reason = "engine reset, awaiting Start()"
	e.startedAt = time.Time{}
	e.stageStartedAt = time.Time{}
	e.dwellStartedAt = time.Time{}
	e.tailStartedAt = time.Time{}
	e.lastUpdated = time.Time{}
	e.lastStatus = e.buildStatus(time.Time{}, Input{}, 0, 0, 0, 0, e.reason)
}

type packMetrics struct {
	minCell   uint16
	maxCell   uint16
	maxSpread uint16
}

// evaluate executes the safety gates and stage advancement logic for an active engine.
func (e *Engine) evaluate(now time.Time, input Input) Status {
	metrics, err := e.validateInput(now, input)
	if err != nil {
		return e.failClosed(now, input, metrics, err)
	}

	for {
		switch e.stage {
		case Stage1:
			nearTarget := math.Abs(input.Inverter.BankVoltageVolts-Stage1VoltageVolts) <= StageVoltageToleranceVolts
			if nearTarget {
				if e.dwellStartedAt.IsZero() {
					e.dwellStartedAt = now
				}
			} else {
				e.dwellStartedAt = time.Time{} // fall-away reset
			}

			dwellDuration := time.Duration(0)
			if !e.dwellStartedAt.IsZero() {
				dwellDuration = now.Sub(e.dwellStartedAt)
			}

			switch {
			case nearTarget && dwellDuration >= MinStageDuration && metrics.maxSpread <= MaxStageAdvanceSpreadMillivolts:
				e.stage = Stage2
				e.stageStartedAt = now
				e.dwellStartedAt = time.Time{}
				e.reason = fmt.Sprintf("advanced to stage 2 (54.8V): stage 1 dwell finished after %v with max spread %dmV", dwellDuration, metrics.maxSpread)
				continue
			case !nearTarget:
				e.reason = fmt.Sprintf("stage 1 holding (54.4V): measured bank voltage %.2fV not within ±%.2fV of 54.40V",
					input.Inverter.BankVoltageVolts, StageVoltageToleranceVolts)
			case dwellDuration < MinStageDuration:
				e.reason = fmt.Sprintf("stage 1 holding (54.4V): dwell %v < %v (bank voltage %.2fV, max spread %dmV)",
					dwellDuration, MinStageDuration, input.Inverter.BankVoltageVolts, metrics.maxSpread)
			default:
				e.reason = fmt.Sprintf("stage 1 holding (54.4V): max spread %dmV > %dmV (dwell %v, bank voltage %.2fV)",
					metrics.maxSpread, MaxStageAdvanceSpreadMillivolts, dwellDuration, input.Inverter.BankVoltageVolts)
			}

		case Stage2:
			nearTarget := math.Abs(input.Inverter.BankVoltageVolts-Stage2VoltageVolts) <= StageVoltageToleranceVolts
			if nearTarget {
				if e.dwellStartedAt.IsZero() {
					e.dwellStartedAt = now
				}
			} else {
				e.dwellStartedAt = time.Time{} // fall-away reset
			}

			dwellDuration := time.Duration(0)
			if !e.dwellStartedAt.IsZero() {
				dwellDuration = now.Sub(e.dwellStartedAt)
			}

			switch {
			case nearTarget && dwellDuration >= MinStageDuration && metrics.maxSpread <= MaxStageAdvanceSpreadMillivolts:
				e.stage = Stage3
				e.stageStartedAt = now
				e.dwellStartedAt = time.Time{}
				e.tailStartedAt = time.Time{}
				e.reason = fmt.Sprintf("advanced to stage 3 (55.2V): stage 2 dwell finished after %v with max spread %dmV", dwellDuration, metrics.maxSpread)
				continue
			case !nearTarget:
				e.reason = fmt.Sprintf("stage 2 holding (54.8V): measured bank voltage %.2fV not within ±%.2fV of 54.80V",
					input.Inverter.BankVoltageVolts, StageVoltageToleranceVolts)
			case dwellDuration < MinStageDuration:
				e.reason = fmt.Sprintf("stage 2 holding (54.8V): dwell %v < %v (bank voltage %.2fV, max spread %dmV)",
					dwellDuration, MinStageDuration, input.Inverter.BankVoltageVolts, metrics.maxSpread)
			default:
				e.reason = fmt.Sprintf("stage 2 holding (54.8V): max spread %dmV > %dmV (dwell %v, bank voltage %.2fV)",
					metrics.maxSpread, MaxStageAdvanceSpreadMillivolts, dwellDuration, input.Inverter.BankVoltageVolts)
			}

		case Stage3:
			nearTarget := math.Abs(input.Inverter.BankVoltageVolts-Stage3VoltageVolts) <= StageVoltageToleranceVolts
			cellsAboveMin := metrics.minCell >= MinCompletionCellMillivolts
			spreadWithinLimit := metrics.maxSpread <= MaxCompletionSpreadMillivolts
			currentWithinLimit := input.Inverter.TotalChargeCurrentAmps >= 0 && input.Inverter.TotalChargeCurrentAmps <= MaxCompletionChargeCurrentAmps

			if nearTarget && cellsAboveMin && spreadWithinLimit && currentWithinLimit {
				if e.tailStartedAt.IsZero() {
					e.tailStartedAt = now
				}
				tailDuration := now.Sub(e.tailStartedAt)
				if tailDuration >= TailQualificationDuration {
					e.state = StateCompleted
					e.reason = fmt.Sprintf("conditioning completed: all cells >= %dmV, pack spreads <= %dmV, bank voltage %.2fV, and charge current <= %.1fA continuously for %v",
						MinCompletionCellMillivolts, MaxCompletionSpreadMillivolts, input.Inverter.BankVoltageVolts, MaxCompletionChargeCurrentAmps, tailDuration)
					e.lastStatus = e.buildStatus(now, input, 0, metrics.minCell, metrics.maxCell, metrics.maxSpread, e.reason)
					return e.lastStatus
				}
				e.reason = fmt.Sprintf("stage 3 tail qualifying (55.2V): %v / %v (bank voltage %.2fV, current %.2fA, min cell %dmV, max spread %dmV)",
					tailDuration, TailQualificationDuration, input.Inverter.BankVoltageVolts, input.Inverter.TotalChargeCurrentAmps, metrics.minCell, metrics.maxSpread)
			} else {
				wasQualifying := !e.tailStartedAt.IsZero()
				e.tailStartedAt = time.Time{} // TAIL RESET

				var holdReasons []string
				if !nearTarget {
					holdReasons = append(holdReasons, fmt.Sprintf("bank voltage %.2fV not within ±%.2fV of 55.20V", input.Inverter.BankVoltageVolts, StageVoltageToleranceVolts))
				}
				if !cellsAboveMin {
					holdReasons = append(holdReasons, fmt.Sprintf("min cell %dmV < %dmV", metrics.minCell, MinCompletionCellMillivolts))
				}
				if !spreadWithinLimit {
					holdReasons = append(holdReasons, fmt.Sprintf("max spread %dmV > %dmV", metrics.maxSpread, MaxCompletionSpreadMillivolts))
				}
				switch {
				case input.Inverter.TotalChargeCurrentAmps < 0:
					holdReasons = append(holdReasons, fmt.Sprintf("inverter current %.2fA < 0 (discharging)", input.Inverter.TotalChargeCurrentAmps))
				case input.Inverter.TotalChargeCurrentAmps > MaxCompletionChargeCurrentAmps:
					holdReasons = append(holdReasons, fmt.Sprintf("inverter current %.2fA > %.1fA", input.Inverter.TotalChargeCurrentAmps, MaxCompletionChargeCurrentAmps))
				}
				tailNote := ""
				if wasQualifying {
					tailNote = " (tail timer reset)"
				}
				e.reason = fmt.Sprintf("stage 3 holding (55.2V): %s%s", strings.Join(holdReasons, ", "), tailNote)
			}
		}
		break
	}

	targetVoltage := 0.0
	switch e.stage {
	case Stage1:
		targetVoltage = Stage1VoltageVolts
	case Stage2:
		targetVoltage = Stage2VoltageVolts
	case Stage3:
		targetVoltage = Stage3VoltageVolts
	}

	e.lastStatus = e.buildStatus(now, input, targetVoltage, metrics.minCell, metrics.maxCell, metrics.maxSpread, e.reason)
	return e.lastStatus
}

// validateInverterTelemetry verifies that inverter measurements are valid, recent, and numeric.
func validateInverterTelemetry(now time.Time, inv InverterTelemetry) error {
	if !inv.Valid {
		return fmt.Errorf("%w: marked invalid (communication failure or stale inverter snapshot)", ErrInverterTelemetry)
	}
	if inv.Timestamp.IsZero() {
		return fmt.Errorf("%w: zero timestamp", ErrInverterTelemetry)
	}
	age := now.Sub(inv.Timestamp)
	if age < 0 || age > DefaultMaxInverterAge {
		return fmt.Errorf("%w: telemetry age %v exceeds limit %v", ErrInverterTelemetry, age, DefaultMaxInverterAge)
	}
	if math.IsNaN(inv.TotalChargeCurrentAmps) || math.IsInf(inv.TotalChargeCurrentAmps, 0) {
		return fmt.Errorf("%w: charge current is NaN or Inf", ErrInverterTelemetry)
	}
	if math.IsNaN(inv.BankVoltageVolts) || math.IsInf(inv.BankVoltageVolts, 0) {
		return fmt.Errorf("%w: bank voltage is NaN or Inf", ErrInverterTelemetry)
	}
	if inv.BankVoltageVolts <= 0 {
		return fmt.Errorf("%w: bank voltage must be positive, got %.2fV", ErrInverterTelemetry, inv.BankVoltageVolts)
	}
	return nil
}

// validateInput performs comprehensive safety gate checks on the incoming telemetry and settings.
func (e *Engine) validateInput(now time.Time, input Input) (packMetrics, error) {
	var metrics packMetrics

	// Gate 1: Inverter telemetry must be valid, fresh, and error-free across all stages.
	if err := validateInverterTelemetry(now, input.Inverter); err != nil {
		return metrics, err
	}

	// Gate 2: Pack count must match the exact expected monitored count (9).
	if len(input.Packs) != len(ExpectedAddresses) {
		return metrics, fmt.Errorf("%w: expected %d packs, got %d", ErrMissingPack, len(ExpectedAddresses), len(input.Packs))
	}

	expectedMap := make(map[uint8]bool, len(ExpectedAddresses))
	for _, addr := range ExpectedAddresses {
		expectedMap[addr] = true
	}

	seenAddresses := make(map[uint8]bool, len(input.Packs))
	seenBMSIDs := make(map[string]uint8, len(input.Packs))

	for _, pack := range input.Packs {
		addr := pack.Frame.Address

		// Gate 3: Address must be in the expected explicit set {0..7, 9}.
		if !expectedMap[addr] {
			return metrics, fmt.Errorf("%w: address %d is not in expected set", ErrExtraPack, addr)
		}
		if seenAddresses[addr] {
			return metrics, fmt.Errorf("%w: address %d reported multiple times", ErrDuplicateAddress, addr)
		}
		seenAddresses[addr] = true

		// Gate 4: Physical BMS ID must be present and distinct.
		// NOTE: Raw BMS ID strings are redacted to protect deployment identifiers;
		// error messages report only the offending bus addresses.
		if pack.Settings == nil || pack.Settings.Balance == nil {
			return metrics, fmt.Errorf("%w: pack %d has no balance settings", ErrMissingBMSID, addr)
		}
		bmsID := pack.Settings.BMSID()
		if bmsID == "" {
			return metrics, fmt.Errorf("%w: pack %d has empty BMS ID", ErrMissingBMSID, addr)
		}
		if prevAddr, exists := seenBMSIDs[bmsID]; exists {
			return metrics, fmt.Errorf("%w: duplicate physical BMS identifier detected between pack addresses %d and %d", ErrDuplicateBMSID, prevAddr, addr)
		}
		seenBMSIDs[bmsID] = addr

		// Gate 5: Telemetry freshness and confirmation.
		if !pack.Fresh {
			return metrics, fmt.Errorf("%w: pack %d telemetry marked stale", ErrStaleTelemetry, addr)
		}
		if pack.SeenAt.IsZero() {
			return metrics, fmt.Errorf("%w: pack %d telemetry has zero timestamp", ErrStaleTelemetry, addr)
		}
		telemetryAge := now.Sub(pack.SeenAt)
		if telemetryAge < 0 || telemetryAge > DefaultMaxTelemetryAge {
			return metrics, fmt.Errorf("%w: pack %d telemetry age %v exceeds limit %v", ErrStaleTelemetry, addr, telemetryAge, DefaultMaxTelemetryAge)
		}
		if addr == 0 && !pack.Confirmed {
			return metrics, fmt.Errorf("%w: address 0 telemetry is not confirmed", ErrUnconfirmedAddressZero)
		}

		// Gate 6: Settings freshness.
		if pack.Settings.Status != interpack.SettingsFresh || !pack.Settings.Fresh {
			return metrics, fmt.Errorf("%w: pack %d settings status %s (fresh=%v)", ErrStaleSettings, addr, pack.Settings.Status, pack.Settings.Fresh)
		}
		if pack.Settings.BalanceSeenAt.IsZero() || pack.Settings.ProtectionSeenAt.IsZero() {
			return metrics, fmt.Errorf("%w: pack %d settings block timestamps missing", ErrStaleSettings, addr)
		}
		balAge := now.Sub(pack.Settings.BalanceSeenAt)
		protAge := now.Sub(pack.Settings.ProtectionSeenAt)
		if balAge < 0 || balAge > DefaultMaxSettingsAge || protAge < 0 || protAge > DefaultMaxSettingsAge {
			return metrics, fmt.Errorf("%w: pack %d settings block age exceeds limit %v (balance: %v, protection: %v)",
				ErrStaleSettings, addr, DefaultMaxSettingsAge, balAge, protAge)
		}

		// Gate 7: Installed BMS safety settings match agreed operating contract.
		bal := pack.Settings.Balance
		if bal.BalanceStartMillivolts != ExpectedBalanceStartMillivolts {
			return metrics, fmt.Errorf("%w: pack %d balance start %dmV != %dmV", ErrSettingsMismatch, addr, bal.BalanceStartMillivolts, ExpectedBalanceStartMillivolts)
		}
		if bal.BalanceDeltaMillivolts != ExpectedBalanceDeltaMillivolts {
			return metrics, fmt.Errorf("%w: pack %d balance delta %dmV != %dmV", ErrSettingsMismatch, addr, bal.BalanceDeltaMillivolts, ExpectedBalanceDeltaMillivolts)
		}

		prot := pack.Settings.Protection
		if prot == nil {
			return metrics, fmt.Errorf("%w: pack %d missing protection settings", ErrSettingsMismatch, addr)
		}
		if prot.CellOVPAlarmMillivolts != ExpectedCellOVPAlarmMillivolts {
			return metrics, fmt.Errorf("%w: pack %d cell OVP alarm %dmV != %dmV", ErrSettingsMismatch, addr, prot.CellOVPAlarmMillivolts, ExpectedCellOVPAlarmMillivolts)
		}
		if prot.CellOVPProtectionMillivolts != ExpectedCellOVPProtectionMillivolts {
			return metrics, fmt.Errorf("%w: pack %d cell OVP protect %dmV != %dmV", ErrSettingsMismatch, addr, prot.CellOVPProtectionMillivolts, ExpectedCellOVPProtectionMillivolts)
		}
		if prot.PackOVPAlarmCentivolts != ExpectedPackOVPAlarmCentivolts {
			return metrics, fmt.Errorf("%w: pack %d pack OVP alarm %dcV != %dcV", ErrSettingsMismatch, addr, prot.PackOVPAlarmCentivolts, ExpectedPackOVPAlarmCentivolts)
		}
		if prot.PackOVPProtectionCentivolts != ExpectedPackOVPProtectionCentivolts {
			return metrics, fmt.Errorf("%w: pack %d pack OVP protect %dcV != %dcV", ErrSettingsMismatch, addr, prot.PackOVPProtectionCentivolts, ExpectedPackOVPProtectionCentivolts)
		}

		// Gate 8: No active faults or warnings (including unverified bits).
		if pack.Frame.Protection.FaultMask != 0 {
			return metrics, fmt.Errorf("%w: pack %d fault mask non-zero: 0x%08X", ErrActiveAlarm, addr, pack.Frame.Protection.FaultMask)
		}
		if pack.Frame.Protection.WarningMask != 0 {
			return metrics, fmt.Errorf("%w: pack %d warning mask non-zero: 0x%04X", ErrActiveAlarm, addr, pack.Frame.Protection.WarningMask)
		}
		if !pack.Frame.Protection.AlarmsClear {
			return metrics, fmt.Errorf("%w: pack %d alarms not clear", ErrActiveAlarm, addr)
		}
	}

	// Gate 9: Cell voltage boundaries and metrics aggregation.
	metrics.minCell = math.MaxUint16
	metrics.maxCell = 0
	metrics.maxSpread = 0

	for _, pack := range input.Packs {
		addr := pack.Frame.Address
		var packMin uint16 = math.MaxUint16
		var packMax uint16 = 0

		for cellIdx := 0; cellIdx < 16; cellIdx++ {
			mv := pack.Frame.CellMillivolts[cellIdx]
			if mv < 1000 || mv > 5000 {
				return metrics, fmt.Errorf("%w: pack %d cell %d voltage %dmV out of valid range", ErrCellVoltageInvalid, addr, cellIdx+1, mv)
			}
			if mv < packMin {
				packMin = mv
			}
			if mv > packMax {
				packMax = mv
			}
			if mv < metrics.minCell {
				metrics.minCell = mv
			}
			if mv > metrics.maxCell {
				metrics.maxCell = mv
			}
			if mv >= MaxCellVoltageCutoffMillivolts {
				return metrics, fmt.Errorf("%w: pack %d cell %d voltage %dmV >= %dmV cutoff",
					ErrCellOvervoltageCutoff, addr, cellIdx+1, mv, MaxCellVoltageCutoffMillivolts)
			}
		}

		packSpread := packMax - packMin
		if packSpread > metrics.maxSpread {
			metrics.maxSpread = packSpread
		}
	}

	return metrics, nil
}

// failClosed halts the engine into StateFaulted, clears targets to 0, and records reason.
func (e *Engine) failClosed(now time.Time, input Input, metrics packMetrics, err error) Status {
	e.state = StateFaulted
	e.err = err
	e.reason = err.Error()
	e.dwellStartedAt = time.Time{}
	e.tailStartedAt = time.Time{}
	e.lastStatus = e.buildStatus(now, input, 0, metrics.minCell, metrics.maxCell, metrics.maxSpread, e.reason)
	return e.lastStatus
}

// buildStatus constructs the full observable Status struct.
func (e *Engine) buildStatus(now time.Time, input Input, targetVoltage float64, minCell, maxCell, maxSpread uint16, reason string) Status {
	var elapsed, stageElapsed, dwellDuration, tailDuration time.Duration

	if !e.startedAt.IsZero() && !now.IsZero() && now.After(e.startedAt) {
		elapsed = now.Sub(e.startedAt)
	}
	if !e.stageStartedAt.IsZero() && !now.IsZero() && now.After(e.stageStartedAt) {
		stageElapsed = now.Sub(e.stageStartedAt)
	}
	if !e.dwellStartedAt.IsZero() && !now.IsZero() && now.After(e.dwellStartedAt) {
		dwellDuration = now.Sub(e.dwellStartedAt)
	}
	if !e.tailStartedAt.IsZero() && !now.IsZero() && now.After(e.tailStartedAt) {
		tailDuration = now.Sub(e.tailStartedAt)
	}

	totalCurrentCap := 0.0
	perInverterCurrentCap := 0.0
	if e.state == StateRunning {
		totalCurrentCap = TotalBankChargingCapAmps
		perInverterCurrentCap = PerInverterChargingCapAmps
	}

	var inverterCurrent float64
	var bankVoltage float64
	if input.Inverter.Valid {
		inverterCurrent = input.Inverter.TotalChargeCurrentAmps
		bankVoltage = input.Inverter.BankVoltageVolts
	}

	isTerminal := (e.state == StateCompleted || e.state == StateFaulted || e.state == StateTimedOut || e.state == StateStopped)

	return Status{
		State:                     e.state,
		Stage:                     e.stage,
		TargetVoltageVolts:        targetVoltage,
		TotalCurrentCapAmps:       totalCurrentCap,
		PerInverterCurrentCapAmps: perInverterCurrentCap,
		Reason:                    reason,
		StartedAt:                 e.startedAt,
		StageStartedAt:            e.stageStartedAt,
		Elapsed:                   elapsed,
		StageElapsed:              stageElapsed,
		DwellDuration:             dwellDuration,
		TailDuration:              tailDuration,
		TotalConnectedPacks:       TotalConnectedPacks,
		ExpectedMonitoredPacks:    ExpectedMonitoredPacks,
		ObservedMonitoredPacks:    len(input.Packs),
		UnmonitoredPacks:          UnmonitoredPacks,
		MaxCellMillivolts:         maxCell,
		MinCellMillivolts:         minCell,
		MaxSpreadMillivolts:       maxSpread,
		InverterCurrentAmps:       inverterCurrent,
		BankVoltageVolts:          bankVoltage,
		Terminal:                  isTerminal,
		err:                       e.err,
	}
}
