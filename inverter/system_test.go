package inverter

import (
	"context"
	"math"
	"testing"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// newTestUnit creates a unit with a mock inverter pre-loaded with typical register values.
func newTestUnit(regs map[uint16]uint16) *unit {
	inv := mock.NewInverter(regs)
	_ = inv.Connect()
	session := modbus.NewSession(inv)
	return &unit{
		info:    UnitInfo{Host: "test"},
		session: session,
	}
}

// newTestClient creates a connected mock client with the given registers.
func newTestClient(regs map[uint16]uint16) modbus.Client {
	inv := mock.NewInverter(regs)
	_ = inv.Connect()
	return inv
}

// typicalRegisters returns a register map with realistic values for an ASP48100U200-H.
func typicalRegisters() map[uint16]uint16 {
	regs := make(map[uint16]uint16)

	// Product info (P00)
	regs[register.AddrMaxVoltageRatedCurrent] = 0x3064 // 48V/100A
	regs[register.AddrProductType] = 4
	for i := uint16(0); i < 8; i++ {
		regs[register.AddrProductModel+i] = 0x2020 // spaces
	}
	regs[register.AddrSoftwareVersionCPU1] = 818
	regs[register.AddrSoftwareVersionCPU2] = 100
	regs[register.AddrHardwareVersionControl] = 100
	regs[register.AddrHardwareVersionPower] = 100
	regs[register.AddrRS485Address] = 1
	regs[register.AddrModelCode] = 4
	regs[register.AddrProtocolVersion] = 107
	for i := uint16(0); i < 20; i++ {
		regs[register.AddrSerialNumber+i] = uint16('A') // "AAAA..."
	}

	// Battery data (P01)
	regs[register.AddrBatterySOC] = 85
	regs[register.AddrBatteryVoltage] = 530 // 53.0V
	regs[register.AddrBatteryCurrent] = 100 // 10.0A charging
	regs[register.AddrTemps] = 0x2819       // controller=40°C, battery=25°C
	regs[register.AddrDCLoadVoltage] = 530  // 53.0V
	regs[register.AddrDCLoadCurrent] = 0    // 0A
	regs[register.AddrDCLoadPower] = 0
	regs[register.AddrPV1Voltage] = 1200 // 120.0V
	regs[register.AddrPV1Current] = 50   // 5.0A
	regs[register.AddrPV1Power] = 600    // 600W
	regs[register.AddrChargeStatus] = 1  // Quick Charge
	regs[register.AddrFaultAlarmBits] = 0
	regs[register.AddrFaultAlarmBits+1] = 0
	regs[register.AddrTotalChargePower] = 600
	regs[register.AddrPV2Voltage] = 1100 // 110.0V
	regs[register.AddrPV2Current] = 40   // 4.0A
	regs[register.AddrPV2Power] = 440    // 440W

	// Inverter data (P02) — fill contiguous range 0x0200-0x0224
	// The bulk reader bridges gaps ≤4, so we need all addresses in the span.
	for addr := uint16(0x0200); addr <= 0x0239; addr++ {
		regs[addr] = 0
	}
	regs[register.AddrFaultBits] = 0
	regs[register.AddrFaultBits+1] = 0
	regs[register.AddrFaultBitsExt] = 0
	regs[register.AddrFaultBitsExt+1] = 0
	regs[register.AddrFaultCode1] = 0
	regs[register.AddrFaultCode2] = 0
	regs[register.AddrFaultCode3] = 0
	regs[register.AddrFaultCode4] = 0
	regs[register.AddrCurrentTime] = 0x1903   // 2025-03
	regs[register.AddrCurrentTime+1] = 0x1D0E // 29th 14:
	regs[register.AddrCurrentTime+2] = 0x1E00 // 30:00
	regs[register.AddrMachineState] = 5       // Inverter Mode
	regs[register.AddrBusVoltage] = 3900      // 390.0V
	regs[register.AddrGridVoltageL1] = 0
	regs[register.AddrGridCurrentL1] = 0
	regs[register.AddrGridFrequency] = 0
	regs[register.AddrInverterVoltageL1] = 1200 // 120.0V
	regs[register.AddrInverterCurrentL1] = 50   // 5.0A
	regs[register.AddrInverterFrequency] = 6000 // 60.00Hz
	regs[register.AddrLoadCurrentL1] = 40       // 4.0A
	regs[register.AddrLoadPowerFactor] = 980    // 0.98
	regs[register.AddrLoadPowerL1] = 470
	regs[register.AddrLoadApparentPowerL1] = 480
	regs[register.AddrMainsChargeCurrent] = 0
	regs[register.AddrLoadRatioL1] = 5
	regs[register.AddrHeatsinkATemp] = 450       // 45.0°C
	regs[register.AddrHeatsinkBTemp] = 500       // 50.0°C
	regs[register.AddrHeatsinkCTemp] = 420       // 42.0°C
	regs[register.AddrHeatsinkDTemp] = 350       // 35.0°C
	regs[register.AddrPVChargeCurrentBatt] = 110 // 11.0A

	// L2 (split-phase)
	regs[register.AddrGridVoltageL2] = 0
	regs[register.AddrGridCurrentL2] = 0
	regs[register.AddrInverterVoltageL2] = 1200 // 120.0V
	regs[register.AddrInverterCurrentL2] = 30   // 3.0A
	regs[register.AddrLoadCurrentL2] = 25       // 2.5A
	regs[register.AddrLoadPowerL2] = 290
	regs[register.AddrLoadApparentPowerL2] = 300
	regs[register.AddrLoadRatioL2] = 3

	// L3 (not present — zero)
	regs[register.AddrGridVoltageL3] = 0
	regs[register.AddrGridCurrentL3] = 0
	regs[register.AddrInverterVoltageL3] = 0
	regs[register.AddrInverterCurrentL3] = 0
	regs[register.AddrLoadCurrentL3] = 0
	regs[register.AddrLoadPowerL3] = 0
	regs[register.AddrLoadApparentPowerL3] = 0
	regs[register.AddrLoadRatioL3] = 0

	// Inverter settings (P07) — fill range for bulk reader
	for addr := uint16(0xE200); addr <= 0xE221; addr++ {
		regs[addr] = 0
	}
	regs[register.AddrInvRS485Address] = 1
	regs[register.AddrParallelMode] = 1 // parallel enabled
	regs[register.AddrOutputPriority] = 0
	regs[register.AddrMainsChargeCurrentLim] = 200
	regs[register.AddrEqualizingChargeEn] = 0
	regs[register.AddrNGFunctionEnable] = 0
	regs[register.AddrOutputVoltage] = 1200
	regs[register.AddrOutputFrequency] = 6000
	regs[register.AddrMaxChargeCurrent] = 1000
	regs[register.AddrACInputRange] = 0
	regs[register.AddrPowerSavingMode] = 0
	regs[register.AddrOverloadAutoRestart] = 1
	regs[register.AddrOverTempAutoRestart] = 1
	regs[register.AddrChargerPriority] = 0
	regs[register.AddrAlarmEnable] = 1
	regs[register.AddrAlarmOnInputLoss] = 0
	regs[register.AddrOverloadBypassEnable] = 0
	regs[register.AddrRecordFaultEnable] = 1
	regs[register.AddrBMSErrorStopEnable] = 0
	regs[register.AddrBMSCommunicationEn] = 1
	regs[register.AddrBMSProtocol] = 0

	// Battery settings (P05) — needed for system voltage lookup
	regs[register.AddrSystemVoltage] = 48

	// Statistics (P08) — fill contiguous range to satisfy bulk reader gap bridging
	for addr := uint16(0xF000); addr <= 0xF04B; addr++ {
		regs[addr] = 0
	}
	regs[register.AddrPVGenDay7] = 10
	regs[register.AddrPVGenDay6] = 12
	regs[register.AddrPVGenDay5] = 15
	regs[register.AddrPVGenDay4] = 8
	regs[register.AddrPVGenDay3] = 20
	regs[register.AddrPVGenDay2] = 18
	regs[register.AddrPVGenDay1] = 22
	regs[register.AddrBattChargeDay7] = 8
	regs[register.AddrBattChargeDay6] = 10
	regs[register.AddrBattChargeDay5] = 12
	regs[register.AddrBattChargeDay4] = 6
	regs[register.AddrBattChargeDay3] = 16
	regs[register.AddrBattChargeDay2] = 14
	regs[register.AddrBattChargeDay1] = 18
	regs[register.AddrBattDischargeDay7] = 5
	regs[register.AddrBattDischargeDay6] = 7
	regs[register.AddrBattDischargeDay5] = 9
	regs[register.AddrBattDischargeDay4] = 4
	regs[register.AddrBattDischargeDay3] = 11
	regs[register.AddrBattDischargeDay2] = 10
	regs[register.AddrBattDischargeDay1] = 13
	regs[register.AddrMainsChargeDay7] = 0
	regs[register.AddrMainsChargeDay6] = 0
	regs[register.AddrMainsChargeDay5] = 0
	regs[register.AddrMainsChargeDay4] = 0
	regs[register.AddrMainsChargeDay3] = 0
	regs[register.AddrMainsChargeDay2] = 0
	regs[register.AddrMainsChargeDay1] = 0
	regs[register.AddrLoadDay7] = 30
	regs[register.AddrLoadDay6] = 35
	regs[register.AddrLoadDay5] = 40
	regs[register.AddrLoadDay4] = 25
	regs[register.AddrLoadDay3] = 50
	regs[register.AddrLoadDay2] = 45
	regs[register.AddrLoadDay1] = 55
	regs[register.AddrMainsLoadDay7] = 0
	regs[register.AddrMainsLoadDay6] = 0
	regs[register.AddrMainsLoadDay5] = 0
	regs[register.AddrMainsLoadDay4] = 0
	regs[register.AddrMainsLoadDay3] = 0
	regs[register.AddrMainsLoadDay2] = 0
	regs[register.AddrMainsLoadDay1] = 0
	regs[register.AddrBatteryChargeToday] = 20
	regs[register.AddrBatteryDischargeToday] = 15
	regs[register.AddrPVGenerationToday] = 50    // 5.0 kWh
	regs[register.AddrLoadConsumptionToday] = 40 // 4.0 kWh
	regs[register.AddrTotalRunningDays] = 365
	regs[register.AddrTotalOverdischargeCount] = 2
	regs[register.AddrTotalFullChargeCount] = 100
	regs[register.AddrAccumBatteryCharge] = 5000 // low word
	regs[register.AddrAccumBatteryCharge+1] = 0  // high word
	regs[register.AddrAccumBatteryDischarge] = 4500
	regs[register.AddrAccumBatteryDischarge+1] = 0
	regs[register.AddrAccumPVGeneration] = 10000 // 1000.0 kWh
	regs[register.AddrAccumPVGeneration+1] = 0
	regs[register.AddrAccumLoadConsumption] = 8000 // 800.0 kWh
	regs[register.AddrAccumLoadConsumption+1] = 0
	regs[register.AddrAccumMainsCharge] = 0
	regs[register.AddrAccumMainsCharge+1] = 0
	regs[register.AddrAccumInverterHours] = 8000
	regs[register.AddrAccumBypassHours] = 100

	return regs
}

func TestSingleUnitSnapshot(t *testing.T) {
	u := newTestUnit(typicalRegisters())

	snap, err := readUnitSnapshot(u.session)
	if err != nil {
		t.Fatalf("readUnitSnapshot: %v", err)
	}
	if len(snap.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}

	// Battery
	if snap.Battery.SOC != 85 {
		t.Errorf("SOC = %v, want 85", snap.Battery.SOC)
	}
	if !approx(snap.Battery.Voltage, 53.0, 0.1) {
		t.Errorf("Voltage = %v, want ~53.0", snap.Battery.Voltage)
	}
	if !approx(snap.Battery.Current, 10.0, 0.1) {
		t.Errorf("Current = %v, want ~10.0", snap.Battery.Current)
	}
	if snap.Battery.ControllerTemp != 40 {
		t.Errorf("ControllerTemp = %v, want 40", snap.Battery.ControllerTemp)
	}
	if snap.Battery.BatteryTemp != 25 {
		t.Errorf("BatteryTemp = %v, want 25", snap.Battery.BatteryTemp)
	}
	if snap.Battery.ChargeStatusName != "Quick Charge" {
		t.Errorf("ChargeStatus = %q, want Quick Charge", snap.Battery.ChargeStatusName)
	}

	// PV
	if !approx(snap.PV.PV1Voltage, 120.0, 0.1) {
		t.Errorf("PV1Voltage = %v, want ~120.0", snap.PV.PV1Voltage)
	}
	if snap.PV.PV1Power != 600 {
		t.Errorf("PV1Power = %v, want 600", snap.PV.PV1Power)
	}
	if snap.PV.TotalPower != 1040 {
		t.Errorf("TotalPower = %v, want 1040", snap.PV.TotalPower)
	}

	// Inverter
	if snap.Inverter.MachineStateName != "Inverter Mode" {
		t.Errorf("MachineState = %q, want Inverter Mode", snap.Inverter.MachineStateName)
	}
	if !approx(snap.Inverter.Frequency, 60.0, 0.01) {
		t.Errorf("Frequency = %v, want ~60.0", snap.Inverter.Frequency)
	}

	// Load
	if snap.Load.TotalPower != 760 { // 470 L1 + 290 L2
		t.Errorf("Load TotalPower = %v, want 760", snap.Load.TotalPower)
	}

	// Stats
	if !approx(snap.Stats.PVGenerationToday, 5.0, 0.1) {
		t.Errorf("PVGenToday = %v, want ~5.0", snap.Stats.PVGenerationToday)
	}
}

func TestParallelAggregation(t *testing.T) {
	// Two units with different PV production but same battery
	regs1 := typicalRegisters()
	regs2 := typicalRegisters()

	// Unit 2 has different PV power
	regs2[register.AddrPV1Power] = 800
	regs2[register.AddrPV2Power] = 500
	regs2[register.AddrTotalChargePower] = 800
	// Different battery current
	regs2[register.AddrBatteryCurrent] = 150 // 15.0A
	// Different heatsink temps
	regs2[register.AddrHeatsinkBTemp] = 550 // 55.0°C (hotter)

	u1 := newTestUnit(regs1)
	u2 := newTestUnit(regs2)

	snap1, _ := readUnitSnapshot(u1.session)
	snap2, _ := readUnitSnapshot(u2.session)

	result := aggregate([]UnitSnapshot{snap1, snap2})

	// Battery SOC: averaged (both 85)
	if !approx(result.Battery.SOC, 85.0, 0.1) {
		t.Errorf("Aggregated SOC = %v, want 85", result.Battery.SOC)
	}

	// Battery current: summed (10 + 15 = 25)
	if !approx(result.Battery.Current, 25.0, 0.1) {
		t.Errorf("Aggregated current = %v, want 25.0", result.Battery.Current)
	}

	// PV power: summed (600+440) + (800+500) = 2340
	if result.PV.TotalPower != 2340 {
		t.Errorf("Aggregated PV total = %v, want 2340", result.PV.TotalPower)
	}

	// Heatsink temp: max (50 vs 55 = 55)
	if !approx(result.Inverter.HeatsinkTempB, 55.0, 0.1) {
		t.Errorf("Aggregated HeatsinkB = %v, want 55.0", result.Inverter.HeatsinkTempB)
	}

	// Load power: summed (760 + 760 = 1520)
	if result.Load.TotalPower != 1520 {
		t.Errorf("Aggregated load = %v, want 1520", result.Load.TotalPower)
	}

	// Stats: summed
	if !approx(result.Stats.PVGenerationToday, 10.0, 0.1) { // 5.0 + 5.0
		t.Errorf("Aggregated PVGen = %v, want 10.0", result.Stats.PVGenerationToday)
	}
}

func TestStaleFallback(t *testing.T) {
	regs := typicalRegisters()
	inv := mock.NewInverter(regs)
	_ = inv.Connect()
	session := modbus.NewSession(inv)

	u := &unit{
		info:    UnitInfo{Host: "test"},
		session: session,
	}

	// Take a good snapshot first
	snap, _ := readUnitSnapshot(session)
	snap.Host = "test"
	u.lastSnapshot = snap

	// Now make the mock fail
	inv.ReadHook = func(_, _ uint16) error {
		return &modbus.ModbusError{FunctionCode: 0x03, ExceptionCode: 0x02}
	}

	// Build a system with this unit
	sys := &System{units: []*unit{u}}

	// First failure: data returned but not yet stale
	result := sys.Snapshot(context.Background())
	if result.Units[0].Stale {
		t.Error("expected not stale after 1 failure")
	}
	if result.Battery.SOC != 85 { // should use fallback
		t.Errorf("expected fallback SOC=85, got %v", result.Battery.SOC)
	}

	// Accumulate failures to trigger stale
	for i := 0; i < MaxStaleSnapshots; i++ {
		sys.Snapshot(context.Background())
	}
	result = sys.Snapshot(context.Background())
	if !result.Units[0].Stale {
		t.Error("expected stale after repeated failures")
	}
}

func TestNewSystemWithClients(t *testing.T) {
	regs1 := typicalRegisters()
	regs2 := typicalRegisters()
	regs2[register.AddrPV1Power] = 900

	// Fill product info range for Init's bulkRead
	for _, regs := range []map[uint16]uint16{regs1, regs2} {
		for addr := uint16(0x000A); addr <= 0x0048; addr++ {
			if _, ok := regs[addr]; !ok {
				regs[addr] = 0
			}
		}
	}

	c1 := newTestClient(regs1)
	c2 := newTestClient(regs2)

	sys := NewSystem([]modbus.Client{c1, c2})
	if err := sys.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if !sys.IsParallel() {
		t.Error("expected parallel mode detected")
	}

	snap := sys.Snapshot(context.Background())
	if len(snap.Units) != 2 {
		t.Fatalf("expected 2 units, got %d", len(snap.Units))
	}

	// PV1 power summed: 600 + 900 = 1500
	expectedPV1 := float64(600 + 900)
	if snap.PV.PV1Power != expectedPV1 {
		t.Errorf("PV1Power = %v, want %v", snap.PV.PV1Power, expectedPV1)
	}
}

func approx(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}
