package serve

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
	"github.com/daniel-sullivan/srne-solar-controller/conditioning"
	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

func healthyConditioningStore(at time.Time) *interpack.Store {
	store := &interpack.Store{}
	for _, addr := range conditioning.ExpectedAddresses {
		frame := interpack.Frame{Address: addr, Protection: interpack.ProtectionInfo{AlarmsClear: true}}
		for i := range frame.CellMillivolts {
			frame.CellMillivolts[i] = 3300
		}
		store.Record(frame, at)
		if addr == 0 {
			store.Record(frame, at)
		}
		store.RecordSettings(interpack.SettingsFrame{Address: addr, BlockType: interpack.BlockTypeBalance,
			Balance: &interpack.BalanceSettings{BalanceStartMillivolts: 3400, BalanceDeltaMillivolts: 30, BMSID: fmt.Sprintf("bms-%d", addr)}}, at)
		store.RecordSettings(interpack.SettingsFrame{Address: addr, BlockType: interpack.BlockTypeProtection,
			Protection: &interpack.ProtectionSettings{CellOVPAlarmMillivolts: 3600, CellOVPProtectionMillivolts: 3650,
				PackOVPAlarmCentivolts: 5760, PackOVPProtectionCentivolts: 5840}}, at)
	}
	return store
}

func TestConditioningStartAndStopRestoreBothUnits(t *testing.T) {
	u0, u1 := mock.NewInverter(conditioningRegisters()), mock.NewInverter(conditioningRegisters())
	for _, u := range []*mock.Inverter{u0, u1} {
		if err := u.Connect(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = u0.Close(); _ = u1.Close() })
	sys := inverter.NewSystem([]modbus.Client{u0, u1}, []string{"u0", "u1"})
	hub := NewHub(sys, time.Hour, time.Hour)
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(runCtx)
	if err := hub.ReserveConditioning(); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := hub.WithConditioning(ctx, func(context.Context, *inverter.System) error { return nil }); err != nil {
		t.Fatal(err)
	}
	hub.ReleaseConditioning()
	now := time.Now()
	hub.mu.Lock()
	hub.latest = &inverter.Snapshot{Time: now, Parallel: true, Units: []inverter.UnitSnapshot{
		{Battery: inverter.BatteryData{Voltage: 52, Current: -5}},
		{Battery: inverter.BatteryData{Voltage: 52, Current: -5}},
	}}
	hub.mu.Unlock()
	path := filepath.Join(t.TempDir(), "state.json")
	svc := NewConditioningService(hub, healthyConditioningStore(now), path)
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !svc.Status().Active {
		t.Fatal("conditioning did not become active")
	}
	for i := 0; i < 2; i++ {
		got, err := sys.ReadUnitChargeSettings(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		if got.RawRegisters[register.AddrMaxChargeCurrent] != 100 || got.RawRegisters[register.AddrBMSCommunicationEn] != 0 {
			t.Fatalf("unit %d did not enter 10A conditioning mode", i)
		}
	}
	if _, err := LoadConditioningJournal(path); err != nil {
		t.Fatal("conditioning journal missing while active:", err)
	}
	if err := svc.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if svc.Status().Active || svc.Status().RestorePending {
		t.Fatal("conditioning did not finish restoration")
	}
	for i := 0; i < 2; i++ {
		got, err := sys.ReadUnitChargeSettings(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		if got.RawRegisters[register.AddrMaxChargeCurrent] != 1000 || got.RawRegisters[register.AddrBMSCommunicationEn] != 2 || got.RawRegisters[register.AddrStopChargeSOC] != 90 {
			t.Fatalf("unit %d original charge profile was not restored", i)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("journal retained after restoration: %v", err)
	}
}

func TestRecoveryRemainsPendingIfJournalDisappears(t *testing.T) {
	hub := NewHub(nil, time.Hour, time.Hour)
	svc := NewConditioningService(hub, nil, filepath.Join(t.TempDir(), "state.json"))
	svc.ExpectRecovery()
	if err := hub.ReserveConditioning(); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(runCtx)
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	err := svc.Recover(ctx)
	if err == nil {
		t.Fatalf("missing journal must remain pending: %v", err)
	}
	if !svc.Status().RestorePending {
		t.Fatal("recovery flag cleared after journal disappeared")
	}
	if err := hub.ReserveConditioning(); err == nil {
		t.Fatal("write reservation released after journal disappeared")
	}
}

func TestRecoverRestoresExactProfilesOnBothUnits(t *testing.T) {
	u0, u1 := mock.NewInverter(conditioningRegisters()), mock.NewInverter(conditioningRegisters())
	for _, u := range []*mock.Inverter{u0, u1} {
		if err := u.Connect(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = u0.Close(); _ = u1.Close() })
	sys := inverter.NewSystem([]modbus.Client{u0, u1}, []string{"u0", "u1"})
	saved, err := sys.ReadChargeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveConditioningJournal(path, saved); err != nil {
		t.Fatal(err)
	}
	svc := NewConditioningService(nil, nil, path)
	if err := svc.applyStart(context.Background(), sys, saved); err != nil {
		t.Fatal(err)
	}
	hub := NewHub(sys, time.Hour, time.Hour)
	svc.hub = hub
	svc.ExpectRecovery()
	if err := hub.ReserveConditioning(); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(runCtx)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if svc.Status().RestorePending {
		t.Fatal("recovery still pending after both profiles restored")
	}
	if _, err := LoadConditioningJournal(path); err == nil {
		t.Fatal("journal was not cleared after verified restoration")
	}
	got, err := sys.ReadChargeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := range saved {
		for _, addr := range conditioningJournalRegisters {
			if got[i].RawRegisters[addr] != saved[i].RawRegisters[addr] {
				t.Fatalf("unit %d register 0x%04x = %d, want %d", i, addr, got[i].RawRegisters[addr], saved[i].RawRegisters[addr])
			}
		}
	}
	if err := hub.ReserveConditioning(); err != nil {
		t.Fatal("restored hub did not release its write reservation:", err)
	}
}

func TestCorruptRecoveryJournalSuppressesBothChargeCurrents(t *testing.T) {
	u0, u1 := mock.NewInverter(conditioningRegisters()), mock.NewInverter(conditioningRegisters())
	for _, u := range []*mock.Inverter{u0, u1} {
		if err := u.Connect(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = u0.Close(); _ = u1.Close() })
	sys := inverter.NewSystem([]modbus.Client{u0, u1}, []string{"u0", "u1"})
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	hub := NewHub(sys, time.Hour, time.Hour)
	svc := NewConditioningService(hub, nil, path)
	svc.ExpectRecovery()
	if err := hub.ReserveConditioning(); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(runCtx)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := svc.Recover(ctx); err == nil {
		t.Fatal("corrupt journal should not be accepted as restored")
	}
	if !svc.Status().RestorePending {
		t.Fatal("corrupt journal released pending state")
	}
	for i := 0; i < 2; i++ {
		got, err := sys.ReadUnitChargeSettings(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		if got.RawRegisters[register.AddrMaxChargeCurrent] != 0 {
			t.Fatalf("unit %d kept charging after corrupt journal", i)
		}
	}
}

func conditioningRegisters() map[uint16]uint16 {
	return map[uint16]uint16{
		register.AddrSystemVoltage: 48, register.AddrBatteryType: 6,
		register.AddrLimitedChargeVoltage: 144, register.AddrEqualizingChargeVolt: 146,
		register.AddrBoostChargeVoltage: 146, register.AddrFloatChargeVoltage: 146,
		register.AddrStopChargeSOC: 90, register.AddrEqualizingChargeEn: 1,
		register.AddrMaxChargeCurrent: 1000, register.AddrBMSCommunicationEn: 2,
	}
}

func TestApplyStartSetsBothParallelUnitsToConditioningProfile(t *testing.T) {
	u0, u1 := mock.NewInverter(conditioningRegisters()), mock.NewInverter(conditioningRegisters())
	if err := u0.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := u1.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u0.Close(); _ = u1.Close() })
	sys := inverter.NewSystem([]modbus.Client{u0, u1}, []string{"u0", "u1"})
	units, err := sys.ReadChargeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewConditioningService(nil, nil, "")
	if err := svc.applyStart(context.Background(), sys, units); err != nil {
		t.Fatal(err)
	}
	got, err := sys.ReadChargeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if p.RawRegisters[register.AddrMaxChargeCurrent] != inverter.EncodeCurrent(conditioning.PerInverterChargingCapAmps) {
			t.Fatalf("unit %d current = %d", p.UnitIndex, p.RawRegisters[register.AddrMaxChargeCurrent])
		}
		if p.RawRegisters[register.AddrBMSCommunicationEn] != 0 || p.RawRegisters[register.AddrEqualizingChargeEn] != 0 {
			t.Fatalf("unit %d communication/equalizing not disabled", p.UnitIndex)
		}
		want := inverter.Encode12VBaseVoltage(conditioning.Stage1VoltageVolts, p.SystemVoltage)
		for _, a := range []uint16{register.AddrLimitedChargeVoltage, register.AddrEqualizingChargeVolt, register.AddrBoostChargeVoltage, register.AddrFloatChargeVoltage} {
			if p.RawRegisters[a] != want {
				t.Fatalf("unit %d register 0x%04x = %d, want %d", p.UnitIndex, a, p.RawRegisters[a], want)
			}
		}
	}
}

type conditioningIgnoredWrite struct {
	*mock.Inverter
	addr uint16
}

func (c *conditioningIgnoredWrite) WriteSingleRegister(addr, value uint16) error {
	if addr == c.addr {
		return nil
	}
	return c.Inverter.WriteSingleRegister(addr, value)
}

func TestApplyStartRefusesWhenZeroCurrentCannotBeVerified(t *testing.T) {
	u0, u1 := mock.NewInverter(conditioningRegisters()), mock.NewInverter(conditioningRegisters())
	if err := u0.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := u1.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = u0.Close(); _ = u1.Close() })
	bad := &conditioningIgnoredWrite{Inverter: u0, addr: register.AddrMaxChargeCurrent}
	sys := inverter.NewSystem([]modbus.Client{bad, u1}, []string{"u0", "u1"})
	units, err := sys.ReadChargeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewConditioningService(nil, nil, "")
	if err := svc.applyStart(context.Background(), sys, units); err == nil {
		t.Fatal("expected refused zero-current write")
	}
	got, err := sys.ReadUnitChargeSettings(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawRegisters[register.AddrBMSCommunicationEn] != 2 {
		t.Fatalf("CAN changed after zero-current refusal: %d", got.RawRegisters[register.AddrBMSCommunicationEn])
	}
}

func TestSuppressChargeAttemptsBothUnitsAfterFirstRefusal(t *testing.T) {
	u0, u1 := mock.NewInverter(conditioningRegisters()), mock.NewInverter(conditioningRegisters())
	for _, u := range []*mock.Inverter{u0, u1} {
		if err := u.Connect(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = u0.Close(); _ = u1.Close() })
	bad := &conditioningIgnoredWrite{Inverter: u0, addr: register.AddrMaxChargeCurrent}
	sys := inverter.NewSystem([]modbus.Client{bad, u1}, []string{"u0", "u1"})
	if err := suppressCharge(context.Background(), sys); err == nil {
		t.Fatal("first unit refusal was not reported")
	}
	got, err := sys.ReadUnitChargeSettings(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawRegisters[register.AddrMaxChargeCurrent] != 0 {
		t.Fatal("second unit was not suppressed after first unit refused")
	}
}

func TestValidateParallelSnapshotAllowsPreconditioningCurrent(t *testing.T) {
	s := &inverter.Snapshot{Parallel: true, Units: []inverter.UnitSnapshot{
		{Battery: inverter.BatteryData{Voltage: 52, Current: -28.2}},
		{Battery: inverter.BatteryData{Voltage: 52, Current: -18.9}},
	}}
	if err := validateParallelSnapshot(s); err != nil {
		t.Fatalf("preconditioning current should not gate start: %v", err)
	}
}

func TestValidateActiveStageDetectsControlledRegisterDrift(t *testing.T) {
	base := inverter.UnitChargeSettings{UnitIndex: 0, SystemVoltage: 48, BatteryType: 6,
		RawRegisters: map[uint16]uint16{
			register.AddrBMSCommunicationEn: 0, register.AddrEqualizingChargeEn: 0,
			register.AddrMaxChargeCurrent:     inverter.EncodeCurrent(conditioning.PerInverterChargingCapAmps),
			register.AddrLimitedChargeVoltage: inverter.Encode12VBaseVoltage(conditioning.Stage1VoltageVolts, 48),
			register.AddrEqualizingChargeVolt: inverter.Encode12VBaseVoltage(conditioning.Stage1VoltageVolts, 48),
			register.AddrBoostChargeVoltage:   inverter.Encode12VBaseVoltage(conditioning.Stage1VoltageVolts, 48),
			register.AddrFloatChargeVoltage:   inverter.Encode12VBaseVoltage(conditioning.Stage1VoltageVolts, 48),
		}}
	if err := validateActiveStage([]inverter.UnitChargeSettings{base}, conditioning.Status{TargetVoltageVolts: conditioning.Stage1VoltageVolts}); err != nil {
		t.Fatalf("valid active stage rejected: %v", err)
	}
	base.RawRegisters[register.AddrFloatChargeVoltage]++
	if err := validateActiveStage([]inverter.UnitChargeSettings{base}, conditioning.Status{TargetVoltageVolts: conditioning.Stage1VoltageVolts}); err == nil {
		t.Fatal("expected float-voltage drift to be rejected")
	}
}
