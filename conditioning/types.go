package conditioning

import (
	"errors"
	"fmt"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
)

// Operating contract and architecture constants.
const (
	// TotalConnectedPacks is the physical count of connected 16S 100Ah packs.
	TotalConnectedPacks = 10

	// ExpectedMonitoredPacks is the number of packs reporting over the RS485 bus.
	ExpectedMonitoredPacks = 9

	// UnmonitoredPacks is the number of connected packs without RS485 reporting.
	// NOTE: Pack 8's internal BMS cell sensing and charge cutoff work, but telemetry is broken.
	// The decision engine does not claim the unmonitored pack is safe or balanced.
	UnmonitoredPacks = 1

	// ParallelInverterUnits is the number of parallel SRNE hybrid inverters.
	ParallelInverterUnits = 2

	// Staged CV voltage targets (Volts).
	Stage1VoltageVolts = 54.4
	Stage2VoltageVolts = 54.8
	Stage3VoltageVolts = 55.2

	// StageVoltageToleranceVolts is the narrow tolerance window (±0.10V) around the stage target
	// within which measured bank voltage must remain to qualify for stage dwell and tail timing.
	StageVoltageToleranceVolts = 0.10

	// Bank and inverter charging current limits (Amperes).
	TotalBankChargingCapAmps   = 20.0
	PerInverterChargingCapAmps = 10.0

	// Timing thresholds.
	MinStageDuration          = 30 * time.Minute
	TailQualificationDuration = 30 * time.Minute
	OverallTimeout            = 8 * time.Hour

	// Spread thresholds (millivolts).
	MaxStageAdvanceSpreadMillivolts = 30 // Max-min cell spread to advance Stage 1->2 and 2->3
	MaxCompletionSpreadMillivolts   = 20 // Max-min cell spread for Stage 3 completion

	// Cell voltage thresholds (millivolts).
	MinCompletionCellMillivolts    = 3400 // All monitored cells must be >= 3400 mV for completion
	MaxCellVoltageCutoffMillivolts = 3550 // Any monitored cell >= 3550 mV causes immediate safety shutdown

	// Inverter tail current qualification threshold (Amperes).
	MaxCompletionChargeCurrentAmps = 10.0

	// Freshness thresholds.
	DefaultMaxTelemetryAge = 60 * time.Second
	DefaultMaxSettingsAge  = 3 * time.Minute
	DefaultMaxInverterAge  = 60 * time.Second

	// Installed live BMS safety settings required across all 9 monitored packs.
	ExpectedBalanceStartMillivolts      = 3400
	ExpectedBalanceDeltaMillivolts      = 30
	ExpectedCellOVPAlarmMillivolts      = 3600
	ExpectedCellOVPProtectionMillivolts = 3650
	ExpectedPackOVPAlarmCentivolts      = 5760 // 57.60 V
	ExpectedPackOVPProtectionCentivolts = 5840 // 58.40 V
)

// ExpectedAddresses defines the exact 9 JBD UP16S bus addresses required by the operator contract.
var ExpectedAddresses = []uint8{0, 1, 2, 3, 4, 5, 6, 7, 9}

// Standard errors returned or wrapped by the conditioning engine.
var (
	ErrMissingPack            = errors.New("conditioning: missing expected monitored pack")
	ErrExtraPack              = errors.New("conditioning: unexpected pack address reported")
	ErrDuplicateAddress       = errors.New("conditioning: duplicate pack address reported")
	ErrDuplicateBMSID         = errors.New("conditioning: duplicate BMS identifier across packs")
	ErrMissingBMSID           = errors.New("conditioning: pack missing valid BMS identifier")
	ErrStaleTelemetry         = errors.New("conditioning: stale telemetry received")
	ErrUnconfirmedAddressZero = errors.New("conditioning: address 0 telemetry is not confirmed")
	ErrStaleSettings          = errors.New("conditioning: stale or incomplete BMS settings")
	ErrSettingsMismatch       = errors.New("conditioning: installed BMS safety settings mismatch")
	ErrActiveAlarm            = errors.New("conditioning: active fault or warning mask on pack")
	ErrCellOvervoltageCutoff  = errors.New("conditioning: cell voltage reached or exceeded 3550mV cutoff")
	ErrCellVoltageInvalid     = errors.New("conditioning: cell voltage outside physical limits")
	ErrInverterTelemetry      = errors.New("conditioning: inverter telemetry missing, invalid, or stale")
	ErrAlreadyRunning         = errors.New("conditioning: engine is already running")
	ErrTimeout                = errors.New("conditioning: overall 8h timeout exceeded")
	ErrManualStop             = errors.New("conditioning: manual stop requested")
	ErrNonMonotonicTime       = errors.New("conditioning: non-monotonic time detected")
)

// State represents the lifecycle state of the conditioning engine.
type State string

const (
	StateIdle      State = "idle"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFaulted   State = "faulted"
	StateTimedOut  State = "timed_out"
	StateStopped   State = "stopped"
)

// IsTerminal returns true if the state is Completed, Faulted, TimedOut, or Stopped.
func (s State) IsTerminal() bool {
	return s == StateCompleted || s == StateFaulted || s == StateTimedOut || s == StateStopped
}

// Stage represents the CV stage within the conditioning schedule.
type Stage int

const (
	StageNone Stage = iota
	Stage1          // 54.4V, >=30min dwell within ±0.10V, advance when spread <= 30mV
	Stage2          // 54.8V, >=30min dwell within ±0.10V, advance when spread <= 30mV
	Stage3          // 55.2V, >=30min tail qualification within ±0.10V, cells >= 3400mV, spread <= 20mV, current <= 10A
)

func (s Stage) String() string {
	switch s {
	case Stage1:
		return "stage_1_54.4V"
	case Stage2:
		return "stage_2_54.8V"
	case Stage3:
		return "stage_3_55.2V"
	default:
		return "none"
	}
}

// InverterTelemetry contains independent measurements from the parallel SRNE inverters.
type InverterTelemetry struct {
	// TotalChargeCurrentAmps is the total measured battery bank charge current in Amperes.
	// Positive values indicate current flowing into the battery bank.
	TotalChargeCurrentAmps float64 `json:"total_charge_current_amps"`

	// BankVoltageVolts is the measured bank-terminal voltage from the inverters in Volts.
	BankVoltageVolts float64 `json:"bank_voltage_volts"`

	// Timestamp is the measurement observation time.
	Timestamp time.Time `json:"timestamp"`

	// Valid indicates that inverter communication succeeded and the measurement is reliable.
	Valid bool `json:"valid"`
}

// Input represents the full typed inputs required on each engine update.
type Input struct {
	// Packs contains the snapshot of packs from interpack.Store.Snapshot().
	Packs []interpack.Pack `json:"packs"`

	// Inverter contains independent current measurements from the inverters.
	Inverter InverterTelemetry `json:"inverter"`
}

// Status represents the full observable status of the conditioning engine.
type Status struct {
	// State is the operational state of the conditioning engine.
	State State `json:"state"`

	// Stage is the current stage in the CV schedule.
	Stage Stage `json:"stage"`

	// TargetVoltageVolts is the target CV voltage to present to the inverters (0V if not running).
	TargetVoltageVolts float64 `json:"target_voltage_volts"`

	// TotalCurrentCapAmps is the total bank charging current cap (20A when running, 0A otherwise).
	TotalCurrentCapAmps float64 `json:"total_current_cap_amps"`

	// PerInverterCurrentCapAmps is the current cap per inverter unit (10A when running, 0A otherwise).
	PerInverterCurrentCapAmps float64 `json:"per_inverter_current_cap_amps"`

	// Reason explains the current operational condition, hold cause, or stop reason.
	Reason string `json:"reason"`

	// Timing information.
	StartedAt      time.Time     `json:"started_at,omitempty"`
	StageStartedAt time.Time     `json:"stage_started_at,omitempty"`
	Elapsed        time.Duration `json:"elapsed"`
	StageElapsed   time.Duration `json:"stage_elapsed"`
	DwellDuration  time.Duration `json:"dwell_duration"`
	TailDuration   time.Duration `json:"tail_duration"`

	// Bank topology and monitoring visibility.
	TotalConnectedPacks    int `json:"total_connected_packs"`
	ExpectedMonitoredPacks int `json:"expected_monitored_packs"`
	ObservedMonitoredPacks int `json:"observed_monitored_packs"`
	UnmonitoredPacks       int `json:"unmonitored_packs"`

	// Monitored telemetry metrics across the 9 packs (0 if no packs observed).
	MaxCellMillivolts   uint16  `json:"max_cell_mv"`
	MinCellMillivolts   uint16  `json:"min_cell_mv"`
	MaxSpreadMillivolts uint16  `json:"max_spread_mv"`
	InverterCurrentAmps float64 `json:"inverter_current_amps"`
	BankVoltageVolts    float64 `json:"bank_voltage_volts"`

	// Terminal is true when the engine has halted and will not transition further.
	Terminal bool `json:"terminal"`

	// err is the unexported underlying error for errors.Is/As matching.
	err error
}

// IsTerminal returns true if the engine has stopped, completed, faulted, or timed out.
func (s Status) IsTerminal() bool { return s.Terminal }

// IsRunning returns true if the engine is actively executing the conditioning schedule.
func (s Status) IsRunning() bool { return s.State == StateRunning }

// IsCompleted returns true if conditioning completed all stages successfully.
func (s Status) IsCompleted() bool { return s.State == StateCompleted }

// IsFaulted returns true if the engine stopped due to a safety gate violation.
func (s Status) IsFaulted() bool { return s.State == StateFaulted }

// IsTimedOut returns true if the engine stopped due to exceeding the overall 8h timeout.
func (s Status) IsTimedOut() bool { return s.State == StateTimedOut }

// IsStopped returns true if the engine was halted by a manual stop command.
func (s Status) IsStopped() bool { return s.State == StateStopped }

// Err returns an error representation of the halt reason if the engine is faulted,
// timed out, or stopped; returns nil if idle, running, or completed.
func (s Status) Err() error {
	if s.err != nil {
		return s.err
	}
	switch s.State {
	case StateFaulted:
		return errors.New(s.Reason)
	case StateTimedOut:
		return fmt.Errorf("%w: %s", ErrTimeout, s.Reason)
	case StateStopped:
		return fmt.Errorf("%w: %s", ErrManualStop, s.Reason)
	default:
		return nil
	}
}
