package inverter

import "math"

// aggregate combines per-unit snapshots into a single system Snapshot.
// For a single unit, values pass through directly.
//
// Aggregation rules for parallel systems:
//   - Battery SOC/voltage: average (shared bank, slight measurement variance)
//   - Power, current, energy: sum across units
//   - Temperatures: max (hottest unit is the constraint)
//   - AC voltage, frequency: average
//   - Fault bits: OR (any fault from any unit)
//   - Machine state: worst (highest priority fault state wins)
//   - Settings: not aggregated (read from master at connect time)
func aggregate(units []UnitSnapshot) Snapshot {
	n := len(units)
	if n == 0 {
		return Snapshot{}
	}
	if n == 1 {
		u := units[0]
		return Snapshot{
			Battery:  u.Battery,
			PV:       u.PV,
			Load:     u.Load,
			Grid:     u.Grid,
			Inverter: u.Inverter,
			Stats:    u.Stats,
		}
	}
	return Snapshot{
		Battery:  aggregateBattery(units),
		PV:       aggregatePV(units),
		Load:     aggregateLoad(units),
		Grid:     aggregateGrid(units),
		Inverter: aggregateInverter(units),
		Stats:    aggregateStats(units),
	}
}

func aggregateBattery(units []UnitSnapshot) BatteryData {
	n := float64(len(units))
	var b BatteryData
	for _, u := range units {
		b.SOC += u.Battery.SOC
		b.Voltage += u.Battery.Voltage
		b.Current += u.Battery.Current // sum: each unit contributes current
		b.ControllerTemp = math.Max(b.ControllerTemp, u.Battery.ControllerTemp)
		b.BatteryTemp = math.Max(b.BatteryTemp, u.Battery.BatteryTemp)
		b.TotalChargePower += u.Battery.TotalChargePower
		b.FaultAlarmBits |= u.Battery.FaultAlarmBits
		b.BMSVoltage += u.Battery.BMSVoltage
		b.BMSCurrent += u.Battery.BMSCurrent
		b.BMSTemperature = math.Max(b.BMSTemperature, u.Battery.BMSTemperature)
		b.BMSChargeLimitVolt += u.Battery.BMSChargeLimitVolt
		b.BMSDischargeLimitVolt += u.Battery.BMSDischargeLimitVolt
		b.BMSChargeLimitCurr += u.Battery.BMSChargeLimitCurr
	}
	// Average shared-bank values
	b.SOC /= n
	b.Voltage /= n
	b.BMSVoltage /= n
	b.BMSChargeLimitVolt /= n
	b.BMSDischargeLimitVolt /= n

	// Use the charge status from the first unit that's actively charging
	for _, u := range units {
		if u.Battery.ChargeStatus != 0 {
			b.ChargeStatus = u.Battery.ChargeStatus
			b.ChargeStatusName = u.Battery.ChargeStatusName
			break
		}
	}
	if b.ChargeStatusName == "" {
		b.ChargeStatus = units[0].Battery.ChargeStatus
		b.ChargeStatusName = units[0].Battery.ChargeStatusName
	}

	return b
}

func aggregatePV(units []UnitSnapshot) PVData {
	var pv PVData
	for _, u := range units {
		pv.PV1Power += u.PV.PV1Power
		pv.PV2Power += u.PV.PV2Power
		pv.PV1Current += u.PV.PV1Current
		pv.PV2Current += u.PV.PV2Current
	}
	// Voltage: average (each unit sees similar PV voltage)
	n := float64(len(units))
	for _, u := range units {
		pv.PV1Voltage += u.PV.PV1Voltage
		pv.PV2Voltage += u.PV.PV2Voltage
	}
	pv.PV1Voltage /= n
	pv.PV2Voltage /= n
	pv.TotalPower = pv.PV1Power + pv.PV2Power
	return pv
}

func aggregateLoad(units []UnitSnapshot) LoadData {
	n := float64(len(units))
	var l LoadData
	for _, u := range units {
		l.TotalPower += u.Load.TotalPower
		l.TotalApparentPower += u.Load.TotalApparentPower
		l.PowerFactor += u.Load.PowerFactor
		l.DCVoltage += u.Load.DCVoltage
		l.DCCurrent += u.Load.DCCurrent
		l.DCPower += u.Load.DCPower
	}
	l.PowerFactor /= n // average
	l.DCVoltage /= n   // average
	return l
}

func aggregateGrid(units []UnitSnapshot) GridData {
	n := float64(len(units))
	var g GridData
	for _, u := range units {
		g.Frequency += u.Grid.Frequency
		g.MainsChargeCurr += u.Grid.MainsChargeCurr
	}
	g.Frequency /= n
	g.L1 = aggregatePhase(units, func(u UnitSnapshot) PhaseData { return u.Grid.L1 })
	g.L2 = aggregatePhase(units, func(u UnitSnapshot) PhaseData { return u.Grid.L2 })
	g.L3 = aggregatePhase(units, func(u UnitSnapshot) PhaseData { return u.Grid.L3 })
	return g
}

func aggregatePhase(units []UnitSnapshot, get func(UnitSnapshot) PhaseData) PhaseData {
	n := float64(len(units))
	var p PhaseData
	for _, u := range units {
		ph := get(u)
		p.GridVoltage += ph.GridVoltage
		p.GridCurrent += ph.GridCurrent
		p.InverterVoltage += ph.InverterVoltage
		p.InverterCurrent += ph.InverterCurrent
		p.LoadCurrent += ph.LoadCurrent
		p.LoadPower += ph.LoadPower
		p.LoadApparentPower += ph.LoadApparentPower
		p.LoadRatio += ph.LoadRatio
	}
	// Voltages: average. Currents/power: sum.
	p.GridVoltage /= n
	p.InverterVoltage /= n
	p.LoadRatio /= n
	return p
}

func aggregateInverter(units []UnitSnapshot) InverterData {
	n := float64(len(units))
	var inv InverterData
	// Use the worst machine state (highest value = more severe)
	for _, u := range units {
		if u.Inverter.MachineState > inv.MachineState {
			inv.MachineState = u.Inverter.MachineState
			inv.MachineStateName = u.Inverter.MachineStateName
		}
		inv.BusVoltage += u.Inverter.BusVoltage
		inv.Frequency += u.Inverter.Frequency
		inv.HeatsinkTempA = math.Max(inv.HeatsinkTempA, u.Inverter.HeatsinkTempA)
		inv.HeatsinkTempB = math.Max(inv.HeatsinkTempB, u.Inverter.HeatsinkTempB)
		inv.HeatsinkTempC = math.Max(inv.HeatsinkTempC, u.Inverter.HeatsinkTempC)
		inv.HeatsinkTempD = math.Max(inv.HeatsinkTempD, u.Inverter.HeatsinkTempD)
		inv.FaultBits |= u.Inverter.FaultBits
		inv.FaultBitsExt |= u.Inverter.FaultBitsExt
		for i := 0; i < 4; i++ {
			if inv.FaultCodes[i] == 0 {
				inv.FaultCodes[i] = u.Inverter.FaultCodes[i]
			}
		}
	}
	inv.BusVoltage /= n
	inv.Frequency /= n
	return inv
}

func aggregateStats(units []UnitSnapshot) StatsData {
	var s StatsData
	for _, u := range units {
		s.PVGenerationToday += u.Stats.PVGenerationToday
		s.LoadConsumptionToday += u.Stats.LoadConsumptionToday
		s.BatteryChargeToday += u.Stats.BatteryChargeToday
		s.BatteryDischargeToday += u.Stats.BatteryDischargeToday
		s.GridChargeToday += u.Stats.GridChargeToday
		s.EnergyImportToday += u.Stats.EnergyImportToday
		s.EnergyExportToday += u.Stats.EnergyExportToday
		s.AccumPVGeneration += u.Stats.AccumPVGeneration
		s.AccumLoadConsumption += u.Stats.AccumLoadConsumption
		s.AccumBatteryCharge += u.Stats.AccumBatteryCharge
		s.AccumBatteryDischarge += u.Stats.AccumBatteryDischarge
		s.AccumMainsCharge += u.Stats.AccumMainsCharge
		for i := 0; i < 7; i++ {
			s.PVGenHistory[i] += u.Stats.PVGenHistory[i]
			s.BattChargeHistory[i] += u.Stats.BattChargeHistory[i]
			s.BattDischargeHistory[i] += u.Stats.BattDischargeHistory[i]
			s.MainsChargeHistory[i] += u.Stats.MainsChargeHistory[i]
			s.LoadHistory[i] += u.Stats.LoadHistory[i]
			s.MainsLoadHistory[i] += u.Stats.MainsLoadHistory[i]
		}
	}
	// Running days: max (they should be similar)
	for _, u := range units {
		s.TotalRunningDays = math.Max(s.TotalRunningDays, u.Stats.TotalRunningDays)
	}
	return s
}
