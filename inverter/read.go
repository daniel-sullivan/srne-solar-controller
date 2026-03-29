package inverter

import (
	"fmt"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// readUnitSnapshot reads all realtime data from a single inverter into typed structs.
func readUnitSnapshot(session *modbus.Session) (UnitSnapshot, error) {
	var snap UnitSnapshot
	var errs []string

	if err := readBatteryData(session, &snap.Battery); err != nil {
		errs = append(errs, fmt.Sprintf("battery: %v", err))
	}
	readPVData(session, &snap.PV)
	if err := readInverterAndLoadData(session, &snap.Load, &snap.Grid, &snap.Inverter); err != nil {
		errs = append(errs, fmt.Sprintf("inverter: %v", err))
	}
	if err := readStatsData(session, &snap.Stats); err != nil {
		errs = append(errs, fmt.Sprintf("stats: %v", err))
	}

	snap.Errors = errs
	return snap, nil
}

// bulkRead reads all registers in a group into the session cache.
// Optional registers that fail are silently skipped.
func bulkRead(session *modbus.Session, group register.Group) error {
	const maxPerRead = 32

	type span struct {
		start, end uint16
		optional   bool
	}
	var spans []span

	for _, reg := range group.Registers {
		end := reg.Address + reg.RegCount()
		if reg.Optional {
			spans = append(spans, span{reg.Address, end, true})
			continue
		}
		if len(spans) > 0 {
			last := &spans[len(spans)-1]
			gap := int(reg.Address) - int(last.end)
			if !last.optional && gap >= 0 && gap <= 4 && int(end-last.start) <= maxPerRead {
				last.end = end
				continue
			}
		}
		spans = append(spans, span{reg.Address, end, false})
	}

	for _, s := range spans {
		count := s.end - s.start
		values, err := session.ReadRegisters(s.start, count)
		if err != nil {
			if !s.optional {
				return fmt.Errorf("read 0x%04X-0x%04X: %w", s.start, s.end-1, err)
			}
			continue
		}
		session.Store(s.start, values)
	}
	return nil
}

// Helper to read a scaled float from session cache. Returns 0 on error.
func readScaled(session *modbus.Session, addr uint16, scale register.ScaleFunc) float64 {
	raw, err := session.Lookup(addr)
	if err != nil {
		return 0
	}
	if scale == nil {
		return float64(raw)
	}
	return scale(float64(raw), session.Lookup)
}

// readSigned reads a signed 16-bit value and applies scale.
func readSigned(session *modbus.Session, addr uint16, scale register.ScaleFunc) float64 {
	raw, err := session.Lookup(addr)
	if err != nil {
		return 0
	}
	if scale == nil {
		return float64(int16(raw))
	}
	return scale(float64(int16(raw)), session.Lookup)
}

// readU32 reads a 32-bit value from two consecutive registers.
func readU32(session *modbus.Session, addr uint16) uint32 {
	lo, err := session.Lookup(addr)
	if err != nil {
		return 0
	}
	hi, err := session.Lookup(addr + 1)
	if err != nil {
		return uint32(lo)
	}
	return uint32(lo) | (uint32(hi) << 16)
}

func readBatteryData(session *modbus.Session, b *BatteryData) error {
	if err := bulkRead(session, register.BatteryData); err != nil {
		return err
	}

	b.SOC = readScaled(session, register.AddrBatterySOC, nil)
	b.Voltage = readScaled(session, register.AddrBatteryVoltage, register.Mul01)
	b.Current = readSigned(session, register.AddrBatteryCurrent, register.Mul01)

	// Packed temps: high byte = controller, low byte = battery
	if raw, err := session.Lookup(register.AddrTemps); err == nil {
		b.ControllerTemp = float64(int8(raw >> 8))
		b.BatteryTemp = float64(int8(raw & 0xFF))
	}

	b.ChargeStatus, _ = session.Lookup(register.AddrChargeStatus)
	b.ChargeStatusName = register.ChargeStatus(b.ChargeStatus)
	b.TotalChargePower = readScaled(session, register.AddrTotalChargePower, nil)
	b.FaultAlarmBits = readU32(session, register.AddrFaultAlarmBits)

	// BMS fields (optional — zero if unavailable)
	b.BMSVoltage = readScaled(session, register.AddrBMSBatteryVoltage, register.Mul01)
	b.BMSCurrent = readSigned(session, register.AddrBMSBatteryCurrent, register.Mul01)
	b.BMSTemperature = readSigned(session, register.AddrBMSBatteryTemperature, register.Mul01)
	b.BMSChargeLimitVolt = readScaled(session, register.AddrBMSChargeLimitVoltage, register.Mul01)
	b.BMSDischargeLimitVolt = readScaled(session, register.AddrBMSDischargeLimitVolt, register.Mul01)
	b.BMSChargeLimitCurr = readScaled(session, register.AddrBMSChargeLimitCurrent, register.Mul01)

	return nil
}

func readPVData(session *modbus.Session, pv *PVData) {
	// PV registers are already in the battery group bulk read
	pv.PV1Voltage = readScaled(session, register.AddrPV1Voltage, register.Mul01)
	pv.PV1Current = readScaled(session, register.AddrPV1Current, register.Mul01)
	pv.PV1Power = readScaled(session, register.AddrPV1Power, nil)
	pv.PV2Voltage = readScaled(session, register.AddrPV2Voltage, register.Mul01)
	pv.PV2Current = readScaled(session, register.AddrPV2Current, register.Mul01)
	pv.PV2Power = readScaled(session, register.AddrPV2Power, nil)
	pv.TotalPower = pv.PV1Power + pv.PV2Power
}

func readInverterAndLoadData(session *modbus.Session, load *LoadData, grid *GridData, inv *InverterData) error {
	if err := bulkRead(session, register.InverterData); err != nil {
		return err
	}

	// Inverter state
	inv.MachineState, _ = session.Lookup(register.AddrMachineState)
	inv.MachineStateName = register.MachineState(inv.MachineState)
	inv.BusVoltage = readScaled(session, register.AddrBusVoltage, register.Mul01)
	inv.Frequency = readScaled(session, register.AddrInverterFrequency, register.Mul001)
	inv.HeatsinkTempA = readSigned(session, register.AddrHeatsinkATemp, register.Mul01)
	inv.HeatsinkTempB = readSigned(session, register.AddrHeatsinkBTemp, register.Mul01)
	inv.HeatsinkTempC = readSigned(session, register.AddrHeatsinkCTemp, register.Mul01)
	inv.HeatsinkTempD = readSigned(session, register.AddrHeatsinkDTemp, register.Mul01)
	inv.FaultBits = readU32(session, register.AddrFaultBits)
	inv.FaultBitsExt = readU32(session, register.AddrFaultBitsExt)
	inv.FaultCodes[0], _ = session.Lookup(register.AddrFaultCode1)
	inv.FaultCodes[1], _ = session.Lookup(register.AddrFaultCode2)
	inv.FaultCodes[2], _ = session.Lookup(register.AddrFaultCode3)
	inv.FaultCodes[3], _ = session.Lookup(register.AddrFaultCode4)

	// Grid
	grid.Frequency = readScaled(session, register.AddrGridFrequency, register.Mul001)
	grid.MainsChargeCurr = readScaled(session, register.AddrMainsChargeCurrent, register.Mul01)
	readPhaseData(session, &grid.L1,
		register.AddrGridVoltageL1, register.AddrGridCurrentL1,
		register.AddrInverterVoltageL1, register.AddrInverterCurrentL1,
		register.AddrLoadCurrentL1, register.AddrLoadPowerL1,
		register.AddrLoadApparentPowerL1, register.AddrLoadRatioL1)
	readPhaseData(session, &grid.L2,
		register.AddrGridVoltageL2, register.AddrGridCurrentL2,
		register.AddrInverterVoltageL2, register.AddrInverterCurrentL2,
		register.AddrLoadCurrentL2, register.AddrLoadPowerL2,
		register.AddrLoadApparentPowerL2, register.AddrLoadRatioL2)
	readPhaseData(session, &grid.L3,
		register.AddrGridVoltageL3, register.AddrGridCurrentL3,
		register.AddrInverterVoltageL3, register.AddrInverterCurrentL3,
		register.AddrLoadCurrentL3, register.AddrLoadPowerL3,
		register.AddrLoadApparentPowerL3, register.AddrLoadRatioL3)

	// Load
	load.PowerFactor = readSigned(session, register.AddrLoadPowerFactor, register.Mul001)
	load.DCVoltage = readScaled(session, register.AddrDCLoadVoltage, register.Mul01)
	load.DCCurrent = readScaled(session, register.AddrDCLoadCurrent, register.Mul001)
	load.DCPower = readScaled(session, register.AddrDCLoadPower, nil)
	load.TotalPower = grid.L1.LoadPower + grid.L2.LoadPower + grid.L3.LoadPower
	load.TotalApparentPower = grid.L1.LoadApparentPower + grid.L2.LoadApparentPower + grid.L3.LoadApparentPower

	return nil
}

func readPhaseData(session *modbus.Session, p *PhaseData,
	gridV, gridI, invV, invI, loadI, loadP, loadVA, loadRatio uint16) {
	p.GridVoltage = readScaled(session, gridV, register.Mul01)
	p.GridCurrent = readScaled(session, gridI, register.Mul01)
	p.InverterVoltage = readScaled(session, invV, register.Mul01)
	p.InverterCurrent = readScaled(session, invI, register.Mul01)
	p.LoadCurrent = readScaled(session, loadI, register.Mul01)
	p.LoadPower = readScaled(session, loadP, nil)
	p.LoadApparentPower = readScaled(session, loadVA, nil)
	p.LoadRatio = readScaled(session, loadRatio, nil)
}

func readStatsData(session *modbus.Session, s *StatsData) error {
	if err := bulkRead(session, register.Statistics); err != nil {
		return err
	}

	s.PVGenerationToday = readScaled(session, register.AddrPVGenerationToday, register.Mul01)
	s.LoadConsumptionToday = readScaled(session, register.AddrLoadConsumptionToday, register.Mul01)
	s.BatteryChargeToday = readScaled(session, register.AddrBatteryChargeToday, nil)
	s.BatteryDischargeToday = readScaled(session, register.AddrBatteryDischargeToday, nil)
	s.GridChargeToday = readScaled(session, register.AddrGridChargeToday, nil)
	s.EnergyImportToday = readScaled(session, register.AddrEnergyImportToday, register.Mul01)
	s.EnergyExportToday = readScaled(session, register.AddrEnergyExportToday, register.Mul01)

	s.AccumPVGeneration = float64(readU32(session, register.AddrAccumPVGeneration)) * 0.1
	s.AccumLoadConsumption = float64(readU32(session, register.AddrAccumLoadConsumption)) * 0.1
	s.AccumBatteryCharge = float64(readU32(session, register.AddrAccumBatteryCharge))
	s.AccumBatteryDischarge = float64(readU32(session, register.AddrAccumBatteryDischarge))
	s.AccumMainsCharge = float64(readU32(session, register.AddrAccumMainsCharge)) * 0.1
	s.TotalRunningDays = readScaled(session, register.AddrTotalRunningDays, nil)

	// 7-day history arrays
	pvAddrs := [7]uint16{register.AddrPVGenDay7, register.AddrPVGenDay6, register.AddrPVGenDay5,
		register.AddrPVGenDay4, register.AddrPVGenDay3, register.AddrPVGenDay2, register.AddrPVGenDay1}
	battCAddrs := [7]uint16{register.AddrBattChargeDay7, register.AddrBattChargeDay6, register.AddrBattChargeDay5,
		register.AddrBattChargeDay4, register.AddrBattChargeDay3, register.AddrBattChargeDay2, register.AddrBattChargeDay1}
	battDAddrs := [7]uint16{register.AddrBattDischargeDay7, register.AddrBattDischargeDay6, register.AddrBattDischargeDay5,
		register.AddrBattDischargeDay4, register.AddrBattDischargeDay3, register.AddrBattDischargeDay2, register.AddrBattDischargeDay1}
	mainsAddrs := [7]uint16{register.AddrMainsChargeDay7, register.AddrMainsChargeDay6, register.AddrMainsChargeDay5,
		register.AddrMainsChargeDay4, register.AddrMainsChargeDay3, register.AddrMainsChargeDay2, register.AddrMainsChargeDay1}
	loadAddrs := [7]uint16{register.AddrLoadDay7, register.AddrLoadDay6, register.AddrLoadDay5,
		register.AddrLoadDay4, register.AddrLoadDay3, register.AddrLoadDay2, register.AddrLoadDay1}
	mainsLAddrs := [7]uint16{register.AddrMainsLoadDay7, register.AddrMainsLoadDay6, register.AddrMainsLoadDay5,
		register.AddrMainsLoadDay4, register.AddrMainsLoadDay3, register.AddrMainsLoadDay2, register.AddrMainsLoadDay1}

	for i := 0; i < 7; i++ {
		s.PVGenHistory[i] = readScaled(session, pvAddrs[i], nil)
		s.BattChargeHistory[i] = readScaled(session, battCAddrs[i], nil)
		s.BattDischargeHistory[i] = readScaled(session, battDAddrs[i], nil)
		s.MainsChargeHistory[i] = readScaled(session, mainsAddrs[i], nil)
		s.LoadHistory[i] = readScaled(session, loadAddrs[i], register.Mul01)
		s.MainsLoadHistory[i] = readScaled(session, mainsLAddrs[i], register.Mul01)
	}

	return nil
}
