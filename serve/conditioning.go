package serve

// Manual battery conditioning orchestration.  The decision engine remains
// deterministic; this type owns the reversible inverter transaction around it.
import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
	"github.com/daniel-sullivan/srne-solar-controller/conditioning"
	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

var (
	ErrConditioningActive   = errors.New("battery conditioning already active")
	ErrConditioningRecovery = errors.New("battery conditioning restoration pending")
)

const conditioningRestoreTimeout = 2 * time.Minute

type ConditioningServiceStatus struct {
	Enabled        bool                `json:"enabled"`
	Ready          bool                `json:"ready"`
	Engine         conditioning.Status `json:"engine"`
	Active         bool                `json:"active"`
	RestorePending bool                `json:"restore_pending"`
	Reason         string              `json:"reason"`
	Error          string              `json:"error,omitempty"`
}

type ConditioningService struct {
	hub              *Hub
	store            *interpack.Store
	journalPath      string
	engine           *conditioning.Engine
	engineMu         sync.Mutex
	mu               sync.Mutex
	opMu             sync.Mutex
	active, recovery bool
	reason           string
	appliedTarget    float64
	appliedAt        time.Time
}

func NewConditioningService(hub *Hub, store *interpack.Store, journalPath string) *ConditioningService {
	return &ConditioningService{hub: hub, store: store, journalPath: journalPath, engine: conditioning.NewEngine()}
}

// ExpectRecovery marks a journal observed at startup before the hub becomes
// available to other writers. A vanished journal must then fail closed.
func (s *ConditioningService) ExpectRecovery() {
	s.mu.Lock()
	s.recovery = true
	s.reason = "restoration pending"
	s.mu.Unlock()
}

func (s *ConditioningService) Status() ConditioningServiceStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	enabled := s.hub != nil && s.store != nil && s.journalPath != ""
	var latest *inverter.Snapshot
	if s.hub != nil {
		latest = s.hub.Latest()
	}
	s.engineMu.Lock()
	engineStatus := s.engine.Status()
	s.engineMu.Unlock()
	probe := conditioning.NewEngine()
	probeStatus := conditioning.Status{}
	if enabled {
		now := time.Now()
		probeStatus = probe.Start(now, s.input(now))
	}
	result := ConditioningServiceStatus{
		Enabled: enabled, Ready: enabled && !s.active && !s.recovery && validateParallelSnapshot(latest) == nil && !probeStatus.IsFaulted(),
		Engine: engineStatus, Active: s.active, RestorePending: s.recovery, Reason: s.reason,
	}
	if err := engineStatus.Err(); err != nil {
		result.Error = err.Error()
	}
	return result
}

// Start captures and journals both units, then applies the safe first-stage
// profile.  The reservation is retained for the whole session.
func (s *ConditioningService) Start(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		// A repeated start request is harmless while the same session is active.
		return nil
	}
	if s.recovery {
		s.mu.Unlock()
		return ErrConditioningRecovery
	}
	s.mu.Unlock()
	if s.hub == nil || s.store == nil || s.journalPath == "" {
		return errors.New("conditioning is disabled: hub, monitor, and journal path are required")
	}
	if err := s.hub.ReserveConditioning(); err != nil {
		return err
	}
	ok := false
	defer func() {
		s.mu.Lock()
		recovery := s.recovery
		s.mu.Unlock()
		if !ok && !recovery {
			s.hub.ReleaseConditioning()
			s.engineMu.Lock()
			s.engine.Reset()
			s.engineMu.Unlock()
		}
	}()
	err := s.hub.WithConditioning(ctx, func(ctx context.Context, sys *inverter.System) error {
		latest := s.hub.Latest()
		if err := validateParallelSnapshot(latest); err != nil {
			return err
		}
		units, err := sys.ReadChargeSettings(ctx)
		if err != nil {
			return err
		}
		if err := validateProfiles(units); err != nil {
			return err
		}
		now := time.Now()
		preflight := s.input(now)
		s.engineMu.Lock()
		preflightStatus := s.engine.Start(now, preflight)
		s.engineMu.Unlock()
		if preflightStatus.IsFaulted() {
			return preflightStatus.Err()
		}
		if err := SaveConditioningJournal(s.journalPath, units); err != nil {
			if _, statErr := os.Stat(s.journalPath); statErr == nil {
				s.markRecovery(err)
				s.engineMu.Lock()
				s.engine.Abort(time.Now(), err)
				s.engineMu.Unlock()
			}
			return err
		}
		if err := s.applyStart(ctx, sys, units); err != nil {
			s.markRecovery(err)
			s.engineMu.Lock()
			s.engine.Abort(time.Now(), err)
			s.engineMu.Unlock()
			recoveryCtx, cancel := context.WithTimeout(context.Background(), conditioningRestoreTimeout)
			restoreErr := restoreUnits(recoveryCtx, sys, units)
			cancel()
			if restoreErr == nil {
				if clearErr := ClearConditioningJournal(s.journalPath); clearErr == nil {
					s.mu.Lock()
					s.recovery = false
					s.mu.Unlock()
				} else {
					return fmt.Errorf("%w; clear conditioning journal: %v", err, clearErr)
				}
			}
			if restoreErr != nil {
				s.markRecovery(restoreErr)
				return errors.Join(err, fmt.Errorf("restore conditioning settings: %w", restoreErr))
			}
			return err
		}
		st := preflightStatus
		s.mu.Lock()
		s.active = true
		s.reason = st.Reason
		s.appliedTarget = conditioning.Stage1VoltageVolts
		s.appliedAt = time.Now()
		s.mu.Unlock()
		ok = true
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *ConditioningService) Tick(ctx context.Context, now time.Time) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	active := s.active
	appliedAt := s.appliedAt
	s.mu.Unlock()
	if !active {
		return nil
	}
	// The snapshot taken before a voltage/current write can still show the
	// old charging current. Wait for a post-write observation before applying
	// current limits or counting stage dwell time.
	latest := s.hub.Latest()
	if latest == nil || !latest.Time.After(appliedAt) {
		if now.Sub(appliedAt) >= conditioning.DefaultMaxInverterAge {
			return s.stopOnFault(errors.New("no fresh inverter snapshot after conditioning write"))
		}
		return nil
	}
	s.engineMu.Lock()
	previousTarget := s.engine.Status().TargetVoltageVolts
	st := s.engine.Update(now, s.input(now))
	s.engineMu.Unlock()
	s.mu.Lock()
	s.reason = st.Reason
	s.mu.Unlock()
	if st.IsTerminal() {
		return s.restore(ctx)
	}
	if err := s.hub.WithConditioning(ctx, func(ctx context.Context, sys *inverter.System) error {
		units, err := sys.ReadChargeSettings(ctx)
		if err != nil {
			return err
		}
		if err := validateActiveProfiles(units); err != nil {
			return err
		}
		activeStatus := st
		s.mu.Lock()
		activeStatus.TargetVoltageVolts = s.appliedTarget
		s.mu.Unlock()
		if err := validateActiveStage(units, activeStatus); err != nil {
			return err
		}
		latest := s.hub.Latest()
		if latest == nil || len(latest.Units) != 2 {
			return errors.New("conditioning inverter telemetry unavailable")
		}
		c0, c1 := -latest.Units[0].Battery.Current, -latest.Units[1].Battery.Current
		if c0 > conditioning.PerInverterChargingCapAmps+0.1 || c1 > conditioning.PerInverterChargingCapAmps+0.1 || c0+c1 > conditioning.TotalBankChargingCapAmps+0.1 {
			return errors.New("inverter charging current exceeds conditioning safety cap")
		}
		return nil
	}); err != nil {
		return s.stopOnFault(err)
	}
	// Apply a newly selected stage after the engine has accepted the telemetry.
	// The current cap is restored last, so a voltage transition cannot create an
	// uncontrolled charge surge.
	if st.TargetVoltageVolts > 0 && st.TargetVoltageVolts != previousTarget {
		if err := s.hub.WithConditioning(ctx, func(ctx context.Context, sys *inverter.System) error {
			return s.applyStage(ctx, sys, st.TargetVoltageVolts)
		}); err != nil {
			return s.stopOnFault(err)
		}
		s.mu.Lock()
		s.appliedTarget = st.TargetVoltageVolts
		s.appliedAt = time.Now()
		s.mu.Unlock()
	}
	return nil
}

func (s *ConditioningService) Stop(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if !active {
		s.mu.Lock()
		recovery := s.recovery
		s.mu.Unlock()
		if recovery {
			return s.recoverLocked(ctx)
		}
		return nil
	}
	s.engineMu.Lock()
	s.engine.Stop(time.Now())
	s.engineMu.Unlock()
	return s.restore(ctx)
}

// Recover restores a journal left by a failed start, stop, timeout, or restart.
func (s *ConditioningService) Recover(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.recoverLocked(ctx)
}

func (s *ConditioningService) recoverLocked(ctx context.Context) error {
	j, err := LoadConditioningJournal(s.journalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.mu.Lock()
			pending := s.recovery
			if pending {
				s.reason = "restoration pending: conditioning journal disappeared"
			} else {
				s.recovery = false
			}
			s.mu.Unlock()
			if pending {
				missing := errors.New("conditioning journal disappeared while restoration is pending")
				return errors.Join(missing, s.suppressPendingCharge(ctx))
			}
			return nil
		}
		if s.hub != nil {
			_ = s.hub.ReserveConditioning()
		}
		s.mu.Lock()
		s.recovery = true
		s.reason = "restoration pending: " + err.Error()
		s.mu.Unlock()
		return errors.Join(err, s.suppressPendingCharge(ctx))
	}
	s.mu.Lock()
	s.recovery = true
	s.reason = "restoration pending"
	s.mu.Unlock()
	if err := s.hub.ReserveConditioning(); err != nil && !errors.Is(err, ErrConditioningReserved) {
		return err
	}
	err = s.hub.WithConditioning(ctx, func(ctx context.Context, sys *inverter.System) error { return restoreUnits(ctx, sys, j.Units) })
	if err != nil {
		return err
	}
	if err := ClearConditioningJournal(s.journalPath); err != nil {
		return err
	}
	s.hub.ReleaseConditioning()
	s.mu.Lock()
	s.recovery = false
	s.reason = "restored"
	s.mu.Unlock()
	return nil
}

func (s *ConditioningService) restore(ctx context.Context) error {
	s.mu.Lock()
	s.active = false
	s.recovery = true
	s.reason = "restoration pending"
	s.mu.Unlock()
	j, err := LoadConditioningJournal(s.journalPath)
	if err != nil {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), conditioningRestoreTimeout)
		defer cancel()
		return errors.Join(err, s.suppressPendingCharge(recoveryCtx))
	}
	recoveryCtx, cancel := context.WithTimeout(context.Background(), conditioningRestoreTimeout)
	defer cancel()
	err = s.hub.WithConditioning(recoveryCtx, func(ctx context.Context, sys *inverter.System) error { return restoreUnits(ctx, sys, j.Units) })
	if err != nil {
		return err
	}
	if err := ClearConditioningJournal(s.journalPath); err != nil {
		return err
	}
	s.hub.ReleaseConditioning()
	s.mu.Lock()
	s.active = false
	s.recovery = false
	s.reason = "restored"
	s.mu.Unlock()
	return nil
}

func (s *ConditioningService) suppressPendingCharge(ctx context.Context) error {
	if s.hub == nil {
		return errors.New("cannot suppress charging without inverter hub")
	}
	return s.hub.WithConditioning(ctx, suppressCharge)
}

func (s *ConditioningService) stopOnFault(err error) error {
	s.mu.Lock()
	s.reason = err.Error()
	s.mu.Unlock()
	s.engineMu.Lock()
	s.engine.Abort(time.Now(), err)
	s.engineMu.Unlock()
	return s.restore(context.Background())
}

func (s *ConditioningService) input(now time.Time) conditioning.Input {
	latest := s.hub.Latest()
	var inv conditioning.InverterTelemetry
	if latest != nil && len(latest.Units) == 2 {
		inv.Valid = validateParallelSnapshot(latest) == nil
		inv.Timestamp = latest.Time
		inv.BankVoltageVolts = (latest.Units[0].Battery.Voltage + latest.Units[1].Battery.Voltage) / 2
		inv.TotalChargeCurrentAmps = -(latest.Units[0].Battery.Current + latest.Units[1].Battery.Current)
	}
	return conditioning.InputFromStore(s.store, now, inv, conditioning.DefaultMaxTelemetryAge, conditioning.DefaultMaxSettingsAge)
}

func validateParallelSnapshot(s *inverter.Snapshot) error {
	if s == nil || !s.Parallel || len(s.Units) != 2 {
		return errors.New("conditioning requires two fresh parallel inverter units")
	}
	for _, u := range s.Units {
		if u.Stale || len(u.Errors) > 0 {
			return errors.New("conditioning requires fresh error-free inverter telemetry")
		}
	}
	if s.Units[0].Battery.Voltage < 40 || s.Units[1].Battery.Voltage < 40 || abs(s.Units[0].Battery.Voltage-s.Units[1].Battery.Voltage) > .2 {
		return errors.New("parallel inverter battery voltages disagree")
	}
	return nil
}
func validateProfiles(u []inverter.UnitChargeSettings) error {
	if len(u) != 2 {
		return errors.New("conditioning requires exactly two inverter profiles")
	}
	for _, p := range u {
		if p.SystemVoltage != 48 || p.BatteryType != 6 || p.BMSCommunicationEn != 2 || p.StopChargeSOC != 90 {
			return fmt.Errorf("unit %d charge profile precondition failed", p.UnitIndex)
		}
	}
	return nil
}

func validateActiveProfiles(u []inverter.UnitChargeSettings) error {
	if len(u) != 2 {
		return errors.New("conditioning requires exactly two inverter profiles")
	}
	for _, p := range u {
		if p.SystemVoltage < 40 || p.BatteryType != 6 || p.BMSCommunicationEn != 0 {
			return fmt.Errorf("unit %d active charge profile drift detected", p.UnitIndex)
		}
	}
	return nil
}

func validateActiveStage(u []inverter.UnitChargeSettings, st conditioning.Status) error {
	for _, p := range u {
		want := inverter.Encode12VBaseVoltage(st.TargetVoltageVolts, p.SystemVoltage)
		if p.SystemVoltage != 48 || p.RawRegisters[register.AddrBMSCommunicationEn] != 0 || p.RawRegisters[register.AddrEqualizingChargeEn] != 0 || p.RawRegisters[register.AddrMaxChargeCurrent] != inverter.EncodeCurrent(conditioning.PerInverterChargingCapAmps) {
			return fmt.Errorf("unit %d controlled charge register drift detected", p.UnitIndex)
		}
		for _, a := range []uint16{register.AddrLimitedChargeVoltage, register.AddrEqualizingChargeVolt, register.AddrBoostChargeVoltage, register.AddrFloatChargeVoltage} {
			if p.RawRegisters[a] != want {
				return fmt.Errorf("unit %d stage voltage register drift detected", p.UnitIndex)
			}
		}
	}
	return nil
}
func (s *ConditioningService) applyStart(ctx context.Context, sys *inverter.System, units []inverter.UnitChargeSettings) error {
	for i := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrMaxChargeCurrent, 0); err != nil {
			return err
		}
	}
	for i := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrBMSCommunicationEn, 0); err != nil {
			return err
		}
	}
	for i := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrEqualizingChargeEn, 0); err != nil {
			return err
		}
		for _, a := range []uint16{register.AddrLimitedChargeVoltage, register.AddrEqualizingChargeVolt, register.AddrBoostChargeVoltage, register.AddrFloatChargeVoltage} {
			v := inverter.Encode12VBaseVoltage(conditioning.Stage1VoltageVolts, units[i].SystemVoltage)
			if err := sys.WriteUnitRegisterVerified(ctx, i, a, v); err != nil {
				return err
			}
		}
	}
	for i := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrMaxChargeCurrent, inverter.EncodeCurrent(conditioning.PerInverterChargingCapAmps)); err != nil {
			return err
		}
	}
	return nil
}

func (s *ConditioningService) applyStage(ctx context.Context, sys *inverter.System, target float64) error {
	units, err := sys.ReadChargeSettings(ctx)
	if err != nil {
		return err
	}
	for i := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrMaxChargeCurrent, 0); err != nil {
			return err
		}
	}
	for i := range units {
		v := inverter.Encode12VBaseVoltage(target, units[i].SystemVoltage)
		for _, a := range []uint16{register.AddrLimitedChargeVoltage, register.AddrEqualizingChargeVolt, register.AddrBoostChargeVoltage, register.AddrFloatChargeVoltage} {
			if err := sys.WriteUnitRegisterVerified(ctx, i, a, v); err != nil {
				return err
			}
		}
	}
	for i := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrMaxChargeCurrent, inverter.EncodeCurrent(conditioning.PerInverterChargingCapAmps)); err != nil {
			return err
		}
	}
	return nil
}
func restoreUnits(ctx context.Context, sys *inverter.System, units []inverter.UnitChargeSettings) error {
	if len(units) != 2 {
		return errors.New("conditioning restoration requires two saved inverter units")
	}
	infos := sys.Units()
	if len(infos) != len(units) {
		return errors.New("conditioning restoration inverter count changed")
	}
	for i, p := range units {
		if p.SystemVoltage != 48 {
			return fmt.Errorf("unit %d saved system voltage is not 48V", p.UnitIndex)
		}
		if p.UnitIndex != i || (p.Host != "" && infos[i].Host != p.Host) || (p.Serial != "" && infos[i].Serial != p.Serial) {
			return fmt.Errorf("unit %d identity changed since conditioning started", i)
		}
	}
	// Suppress charging on both units before any potentially slow profile read.
	// Attempt both even if one unit refuses the write.
	if err := suppressCharge(ctx, sys); err != nil {
		return err
	}
	current, err := sys.ReadChargeSettings(ctx)
	if err != nil {
		return fmt.Errorf("read current inverter settings for restore: %w", err)
	}
	for i, p := range current {
		if p.SystemVoltage != 48 || p.BatteryType != 6 || p.UnitIndex != i {
			return fmt.Errorf("unit %d current profile is unsafe for restoration", i)
		}
	}
	for i, p := range units {
		for _, a := range []uint16{register.AddrLimitedChargeVoltage, register.AddrEqualizingChargeVolt, register.AddrBoostChargeVoltage, register.AddrFloatChargeVoltage, register.AddrEqualizingChargeEn} {
			v := p.RawRegisters[a]
			if err := sys.WriteUnitRegisterVerified(ctx, i, a, v); err != nil {
				return err
			}
		}
	}
	for i, p := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrBMSCommunicationEn, p.RawRegisters[register.AddrBMSCommunicationEn]); err != nil {
			return err
		}
	}
	for i, p := range units {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrMaxChargeCurrent, p.RawRegisters[register.AddrMaxChargeCurrent]); err != nil {
			return err
		}
	}
	return nil
}

func suppressCharge(ctx context.Context, sys *inverter.System) error {
	if len(sys.Units()) != 2 {
		return errors.New("conditioning charge suppression requires two inverter units")
	}
	var failures []error
	for i := 0; i < 2; i++ {
		if err := sys.WriteUnitRegisterVerified(ctx, i, register.AddrMaxChargeCurrent, 0); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func (s *ConditioningService) markRecovery(err error) {
	s.mu.Lock()
	s.recovery = true
	s.reason = "restoration pending: " + err.Error()
	s.mu.Unlock()
}
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
