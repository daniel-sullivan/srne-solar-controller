package inverter

import "time"

// Snapshot is the aggregated system state across all inverter units.
// For single-inverter systems, values pass through directly.
// For parallel systems, values are aggregated per the rules in aggregate.go.
type Snapshot struct {
	Time     time.Time      `json:"time"`
	Parallel bool           `json:"parallel"`
	Units    []UnitSnapshot `json:"units"`
	Battery  BatteryData    `json:"battery"`
	PV       PVData         `json:"pv"`
	Load     LoadData       `json:"load"`
	Grid     GridData       `json:"grid"`
	Inverter InverterData   `json:"inverter"`
	Stats    StatsData      `json:"stats"`
}

// UnitSnapshot holds the raw per-inverter data before aggregation.
type UnitSnapshot struct {
	Host     string       `json:"host"`
	Serial   string       `json:"serial"`
	SlaveID  uint8        `json:"slave_id"`
	Stale    bool         `json:"stale"`  // true if this data is from a previous snapshot
	Errors   []string     `json:"errors"` // non-fatal read errors for this unit
	Battery  BatteryData  `json:"battery"`
	PV       PVData       `json:"pv"`
	Load     LoadData     `json:"load"`
	Grid     GridData     `json:"grid"`
	Inverter InverterData `json:"inverter"`
	Stats    StatsData    `json:"stats"`
}

// BatteryData holds battery and BMS realtime values.
type BatteryData struct {
	SOC              float64 `json:"soc"`             // %
	Voltage          float64 `json:"voltage"`         // V
	Current          float64 `json:"current"`         // A (signed: + = charging)
	ControllerTemp   float64 `json:"controller_temp"` // °C
	BatteryTemp      float64 `json:"battery_temp"`    // °C
	ChargeStatus     uint16  `json:"charge_status"`
	ChargeStatusName string  `json:"charge_status_name"`
	TotalChargePower float64 `json:"total_charge_power"` // W
	FaultAlarmBits   uint32  `json:"fault_alarm_bits"`
	// BMS fields (zero if unavailable)
	BMSVoltage            float64 `json:"bms_voltage"`              // V
	BMSCurrent            float64 `json:"bms_current"`              // A
	BMSTemperature        float64 `json:"bms_temperature"`          // °C
	BMSChargeLimitVolt    float64 `json:"bms_charge_limit_volt"`    // V
	BMSDischargeLimitVolt float64 `json:"bms_discharge_limit_volt"` // V
	BMSChargeLimitCurr    float64 `json:"bms_charge_limit_curr"`    // A
}

// PVData holds photovoltaic input data.
type PVData struct {
	PV1Voltage float64 `json:"pv1_voltage"` // V
	PV1Current float64 `json:"pv1_current"` // A
	PV1Power   float64 `json:"pv1_power"`   // W
	PV2Voltage float64 `json:"pv2_voltage"` // V
	PV2Current float64 `json:"pv2_current"` // A
	PV2Power   float64 `json:"pv2_power"`   // W
	TotalPower float64 `json:"total_power"` // W (PV1 + PV2, summed across units)
}

// PhaseData holds per-phase AC measurements.
type PhaseData struct {
	GridVoltage       float64 `json:"grid_voltage"`        // V
	GridCurrent       float64 `json:"grid_current"`        // A
	InverterVoltage   float64 `json:"inverter_voltage"`    // V
	InverterCurrent   float64 `json:"inverter_current"`    // A
	LoadCurrent       float64 `json:"load_current"`        // A
	LoadPower         float64 `json:"load_power"`          // W
	LoadApparentPower float64 `json:"load_apparent_power"` // VA
	LoadRatio         float64 `json:"load_ratio"`          // %
}

// LoadData holds load consumption data.
type LoadData struct {
	TotalPower         float64 `json:"total_power"`          // W (sum of all phases)
	TotalApparentPower float64 `json:"total_apparent_power"` // VA
	PowerFactor        float64 `json:"power_factor"`
	DCVoltage          float64 `json:"dc_voltage"` // V
	DCCurrent          float64 `json:"dc_current"` // A
	DCPower            float64 `json:"dc_power"`   // W
}

// GridData holds grid/mains data with per-phase detail.
type GridData struct {
	Frequency       float64   `json:"frequency"`         // Hz
	MainsChargeCurr float64   `json:"mains_charge_curr"` // A
	L1              PhaseData `json:"l1"`
	L2              PhaseData `json:"l2"`
	L3              PhaseData `json:"l3"`
}

// InverterData holds inverter operational state.
type InverterData struct {
	MachineState     uint16    `json:"machine_state"`
	MachineStateName string    `json:"machine_state_name"`
	BusVoltage       float64   `json:"bus_voltage"`     // V
	Frequency        float64   `json:"frequency"`       // Hz
	HeatsinkTempA    float64   `json:"heatsink_temp_a"` // °C (DC-DC)
	HeatsinkTempB    float64   `json:"heatsink_temp_b"` // °C (DC-AC)
	HeatsinkTempC    float64   `json:"heatsink_temp_c"` // °C (Transformer)
	HeatsinkTempD    float64   `json:"heatsink_temp_d"` // °C (Ambient)
	FaultBits        uint32    `json:"fault_bits"`
	FaultBitsExt     uint32    `json:"fault_bits_ext"`
	FaultCodes       [4]uint16 `json:"fault_codes"`
}

// StatsData holds energy statistics.
type StatsData struct {
	PVGenerationToday     float64    `json:"pv_generation_today"`     // kWh
	LoadConsumptionToday  float64    `json:"load_consumption_today"`  // kWh
	BatteryChargeToday    float64    `json:"battery_charge_today"`    // AH
	BatteryDischargeToday float64    `json:"battery_discharge_today"` // AH
	GridChargeToday       float64    `json:"grid_charge_today"`       // AH
	EnergyImportToday     float64    `json:"energy_import_today"`     // kWh
	EnergyExportToday     float64    `json:"energy_export_today"`     // kWh
	AccumPVGeneration     float64    `json:"accum_pv_generation"`     // kWh
	AccumLoadConsumption  float64    `json:"accum_load_consumption"`  // kWh
	AccumBatteryCharge    float64    `json:"accum_battery_charge"`    // AH
	AccumBatteryDischarge float64    `json:"accum_battery_discharge"` // AH
	AccumMainsCharge      float64    `json:"accum_mains_charge"`      // kWh
	TotalRunningDays      float64    `json:"total_running_days"`
	PVGenHistory          [7]float64 `json:"pv_gen_history"`         // AH, [0]=day-7 .. [6]=day-1
	BattChargeHistory     [7]float64 `json:"batt_charge_history"`    // AH
	BattDischargeHistory  [7]float64 `json:"batt_discharge_history"` // AH
	MainsChargeHistory    [7]float64 `json:"mains_charge_history"`   // AH
	LoadHistory           [7]float64 `json:"load_history"`           // kWh
	MainsLoadHistory      [7]float64 `json:"mains_load_history"`     // kWh
}
