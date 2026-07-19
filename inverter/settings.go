package inverter

import (
	"context"
	"fmt"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// Settings holds all readable/writable settings from the inverter.
type Settings struct {
	Battery  BatterySettings  `json:"battery"`
	Timed    TimedSettings    `json:"timed"`
	Inverter InverterSettings `json:"inverter"`
}

// BatterySettings holds battery configuration parameters.
type BatterySettings struct {
	// System
	SystemVoltage        float64 `json:"system_voltage"`   // V
	NominalCapacity      float64 `json:"nominal_capacity"` // AH
	BatteryType          uint16  `json:"battery_type"`
	BatteryTypeName      string  `json:"battery_type_name"`
	PVChargeCurrentLimit float64 `json:"pv_charge_current_limit"` // A

	// Charge voltages
	OverVoltageProtection float64 `json:"over_voltage_protection"` // V
	LimitedChargeVoltage  float64 `json:"limited_charge_voltage"`  // V
	EqualizingChargeVolt  float64 `json:"equalizing_charge_volt"`  // V
	BoostChargeVoltage    float64 `json:"boost_charge_voltage"`    // V
	FloatChargeVoltage    float64 `json:"float_charge_voltage"`    // V
	BoostReturnVoltage    float64 `json:"boost_return_voltage"`    // V

	// Discharge voltages
	OverDischargeVoltage  float64 `json:"over_discharge_voltage"`  // V
	OverDischargeReturnV  float64 `json:"over_discharge_return_v"` // V
	LimitedDischargeVolt  float64 `json:"limited_discharge_volt"`  // V
	UnderVoltageWarning   float64 `json:"under_voltage_warning"`   // V
	MainsSwitchingVoltage float64 `json:"mains_switching_voltage"` // V
	InverterSwitchingVolt float64 `json:"inverter_switching_volt"` // V
	OverDischargeDelay    float64 `json:"over_discharge_delay"`    // s

	// SOC thresholds
	CutoffChargeSOC    uint8   `json:"cutoff_charge_soc"`     // %
	CutoffDischargeSOC uint8   `json:"cutoff_discharge_soc"`  // %
	StopChargeSOC      float64 `json:"stop_charge_soc"`       // %
	LowSOCAlarm        float64 `json:"low_soc_alarm"`         // %
	SOCSwitchToMains   float64 `json:"soc_switch_to_mains"`   // %
	SOCSwitchToBattery float64 `json:"soc_switch_to_battery"` // %

	// Charge timing
	EqualizingChargeTime float64 `json:"equalizing_charge_time"` // min
	BoostChargeTime      float64 `json:"boost_charge_time"`      // min
	EqualizingChargeIntv float64 `json:"equalizing_charge_intv"` // days
	EqualizingChargeTO   float64 `json:"equalizing_charge_to"`   // min
	StopChargeCurrentLi  float64 `json:"stop_charge_current_li"` // A
	LiActivationCurrent  float64 `json:"li_activation_current"`  // A
	BMSChargeLCMode      float64 `json:"bms_charge_lc_mode"`

	// Temperature limits
	ChargeMaxTemp      float64 `json:"charge_max_temp"`      // °C
	ChargeMinTemp      float64 `json:"charge_min_temp"`      // °C
	DischargeMaxTemp   float64 `json:"discharge_max_temp"`   // °C
	DischargeMinTemp   float64 `json:"discharge_min_temp"`   // °C
	BatteryHeaterStart float64 `json:"battery_heater_start"` // °C
	BatteryHeaterStop  float64 `json:"battery_heater_stop"`  // °C
	TempCompCoeff      float64 `json:"temp_comp_coeff"`      // mV/°C/2V
}

// TimePeriod represents a charge or discharge time window.
type TimePeriod struct {
	StartHour   uint8   `json:"start_hour"`
	StartMinute uint8   `json:"start_minute"`
	EndHour     uint8   `json:"end_hour"`
	EndMinute   uint8   `json:"end_minute"`
	StopSOC     float64 `json:"stop_soc"`     // %
	StopVoltage float64 `json:"stop_voltage"` // V (discharge only)
	MaxPower    float64 `json:"max_power"`    // W
}

// TimedSettings holds timed charge/discharge configuration.
type TimedSettings struct {
	ChargeEnabled    bool          `json:"charge_enabled"`
	ChargePeriods    [3]TimePeriod `json:"charge_periods"`
	DischargeEnabled bool          `json:"discharge_enabled"`
	DischargePeriods [3]TimePeriod `json:"discharge_periods"`
}

// InverterSettings holds inverter operating parameters.
type InverterSettings struct {
	// Output
	OutputVoltage      float64 `json:"output_voltage"`   // V
	OutputFrequency    float64 `json:"output_frequency"` // Hz
	OutputPhase        uint16  `json:"output_phase"`
	OutputPriority     uint16  `json:"output_priority"`
	OutputPriorityName string  `json:"output_priority_name"`

	// Charging
	ChargerPriority       uint16  `json:"charger_priority"`
	ChargerPriorityName   string  `json:"charger_priority_name"`
	ChargeSourceSelection uint16  `json:"charge_source_selection"`  // 0xE04D (firmware V1.96+); read-only diagnostic
	MaxChargeCurrent      float64 `json:"max_charge_current"`       // A
	MainsChargeCurrentLim float64 `json:"mains_charge_current_lim"` // A

	// Power limits
	DeratePower    float64 `json:"derate_power"`     // W
	MaxLineCurrent float64 `json:"max_line_current"` // A
	MaxLinePower   float64 `json:"max_line_power"`   // W

	// Protection
	OverloadAutoRestart  uint16 `json:"overload_auto_restart"`
	OverTempAutoRestart  uint16 `json:"over_temp_auto_restart"`
	OverloadBypassEnable uint16 `json:"overload_bypass_enable"`
	PowerSavingMode      uint16 `json:"power_saving_mode"`
	ACInputRange         uint16 `json:"ac_input_range"`

	// Alarms
	AlarmEnable       uint16 `json:"alarm_enable"`
	AlarmOnInputLoss  uint16 `json:"alarm_on_input_loss"`
	RecordFaultEnable uint16 `json:"record_fault_enable"`

	// Communication
	RS485Address       uint16 `json:"rs485_address"`
	ParallelMode       uint16 `json:"parallel_mode"`
	BMSProtocol        uint16 `json:"bms_protocol"`
	BMSCommunicationEn uint16 `json:"bms_communication_en"`
	BMSErrorStopEnable uint16 `json:"bms_error_stop_enable"`

	// Generator
	GeneratorWorkMode   uint16  `json:"generator_work_mode"`
	GenChargeMaxCurrent float64 `json:"gen_charge_max_current"` // A
	GeneratorRatedPower float64 `json:"generator_rated_power"`  // W
}

// ReadSettings reads all settings from the first unit (master).
func (s *System) ReadSettings(ctx context.Context) (*Settings, error) {
	s.mu.RLock()
	if len(s.units) == 0 {
		s.mu.RUnlock()
		return nil, fmt.Errorf("no units")
	}
	u := s.units[0]
	s.mu.RUnlock()

	u.session.Ctx = ctx
	return readAllSettings(u.session)
}

func readAllSettings(session *modbus.Session) (*Settings, error) {
	var settings Settings

	if err := bulkRead(session, register.BatterySettings); err != nil {
		return nil, fmt.Errorf("battery settings: %w", err)
	}
	readBatterySettings(session, &settings.Battery)

	if err := bulkRead(session, register.TimedChargeDischarge); err != nil {
		return nil, fmt.Errorf("timed settings: %w", err)
	}
	readTimedSettings(session, &settings.Timed)

	if err := bulkRead(session, register.InverterSettings); err != nil {
		return nil, fmt.Errorf("inverter settings: %w", err)
	}
	readInverterSettings(session, &settings.Inverter)

	return &settings, nil
}

func readBatterySettings(s *modbus.Session, b *BatterySettings) {
	b.SystemVoltage = readScaled(s, register.AddrSystemVoltage, nil)
	b.NominalCapacity = readScaled(s, register.AddrNominalBatteryCapAH, nil)
	b.BatteryType, _ = s.Lookup(register.AddrBatteryType)
	b.BatteryTypeName = register.BatteryType(b.BatteryType)
	b.PVChargeCurrentLimit = readScaled(s, register.AddrPVChargeCurrentLimit, register.Mul01)

	b.OverVoltageProtection = readScaled(s, register.AddrOverVoltageProtection, register.Voltage12V)
	b.LimitedChargeVoltage = readScaled(s, register.AddrLimitedChargeVoltage, register.Voltage12V)
	b.EqualizingChargeVolt = readScaled(s, register.AddrEqualizingChargeVolt, register.Voltage12V)
	b.BoostChargeVoltage = readScaled(s, register.AddrBoostChargeVoltage, register.Voltage12V)
	b.FloatChargeVoltage = readScaled(s, register.AddrFloatChargeVoltage, register.Voltage12V)
	b.BoostReturnVoltage = readScaled(s, register.AddrBoostReturnVoltage, register.Voltage12V)

	b.OverDischargeVoltage = readScaled(s, register.AddrOverDischargeVoltage, register.Voltage12V)
	b.OverDischargeReturnV = readScaled(s, register.AddrOverDischargeReturnV, register.Voltage12V)
	b.LimitedDischargeVolt = readScaled(s, register.AddrLimitedDischargeVolt, register.Voltage12V)
	b.UnderVoltageWarning = readScaled(s, register.AddrUnderVoltageWarning, register.Voltage12V)
	b.MainsSwitchingVoltage = readScaled(s, register.AddrMainsSwitchingVoltage, register.Voltage12V)
	b.InverterSwitchingVolt = readScaled(s, register.AddrInverterSwitchingVolt, register.Voltage12V)
	b.OverDischargeDelay = readScaled(s, register.AddrOverDischargeDelay, nil)

	if raw, err := s.Lookup(register.AddrCutoffSOC); err == nil {
		b.CutoffChargeSOC = uint8(raw >> 8)
		b.CutoffDischargeSOC = uint8(raw & 0xFF)
	}
	b.StopChargeSOC = readScaled(s, register.AddrStopChargeSOC, nil)
	b.LowSOCAlarm = readScaled(s, register.AddrLowSOCAlarm, nil)
	b.SOCSwitchToMains = readScaled(s, register.AddrSOCSwitchToMains, nil)
	b.SOCSwitchToBattery = readScaled(s, register.AddrSOCSwitchToBattery, nil)

	b.EqualizingChargeTime = readScaled(s, register.AddrEqualizingChargeTime, nil)
	b.BoostChargeTime = readScaled(s, register.AddrBoostChargeTime, nil)
	b.EqualizingChargeIntv = readScaled(s, register.AddrEqualizingChargeIntv, nil)
	b.EqualizingChargeTO = readScaled(s, register.AddrEqualizingChargeTO, nil)
	b.StopChargeCurrentLi = readScaled(s, register.AddrStopChargeCurrentLi, register.Mul01)
	b.LiActivationCurrent = readScaled(s, register.AddrLiActivationCurrent, register.Mul01)
	b.BMSChargeLCMode = readScaled(s, register.AddrBMSChargeLCMode, nil)

	b.ChargeMaxTemp = readSigned(s, register.AddrChargeMaxTemp, nil)
	b.ChargeMinTemp = readSigned(s, register.AddrChargeMinTemp, nil)
	b.DischargeMaxTemp = readSigned(s, register.AddrDischargeMaxTemp, nil)
	b.DischargeMinTemp = readSigned(s, register.AddrDischargeMinTemp, nil)
	b.BatteryHeaterStart = readSigned(s, register.AddrBatteryHeaterStart, nil)
	b.BatteryHeaterStop = readSigned(s, register.AddrBatteryHeaterStop, nil)
	b.TempCompCoeff = readScaled(s, register.AddrTempCompensationCoeff, nil)
}

func readTimedSettings(s *modbus.Session, t *TimedSettings) {
	chargeStartAddrs := [3]uint16{register.AddrChargeStartTime1, register.AddrChargeStartTime2, register.AddrChargeStartTime3}
	chargeEndAddrs := [3]uint16{register.AddrChargeEndTime1, register.AddrChargeEndTime2, register.AddrChargeEndTime3}
	chargeSOCAddrs := [3]uint16{register.AddrCharge1StopSOC, register.AddrCharge2StopSOC, register.AddrCharge3StopSOC}
	chargeVoltAddrs := [3]uint16{register.AddrCharge1StopVoltage, register.AddrCharge2StopVoltage, register.AddrCharge3StopVoltage}
	chargePowerAddrs := [3]uint16{register.AddrCharge1MaxPower, register.AddrCharge2MaxPower, register.AddrCharge3MaxPower}

	for i := 0; i < 3; i++ {
		readTimePeriod(s, &t.ChargePeriods[i], chargeStartAddrs[i], chargeEndAddrs[i])
		t.ChargePeriods[i].StopSOC = readScaled(s, chargeSOCAddrs[i], nil)
		t.ChargePeriods[i].StopVoltage = readScaled(s, chargeVoltAddrs[i], register.Mul01)
		t.ChargePeriods[i].MaxPower = readScaled(s, chargePowerAddrs[i], register.Mul10)
	}
	if raw, err := s.Lookup(register.AddrTimedChargeEnable); err == nil {
		t.ChargeEnabled = raw != 0
	}

	dischargeStartAddrs := [3]uint16{register.AddrDischargeStartTime1, register.AddrDischargeStartTime2, register.AddrDischargeStartTime3}
	dischargeEndAddrs := [3]uint16{register.AddrDischargeEndTime1, register.AddrDischargeEndTime2, register.AddrDischargeEndTime3}
	dischargeSOCAddrs := [3]uint16{register.AddrDischarge1StopSOC, register.AddrDischarge2StopSOC, register.AddrDischarge3StopSOC}
	dischargeVoltAddrs := [3]uint16{register.AddrDischarge1StopVolt, register.AddrDischarge2StopVolt, register.AddrDischarge3StopVolt}
	dischargePowerAddrs := [3]uint16{register.AddrDischarge1MaxPower, register.AddrDischarge2MaxPower, register.AddrDischarge3MaxPower}

	for i := 0; i < 3; i++ {
		readTimePeriod(s, &t.DischargePeriods[i], dischargeStartAddrs[i], dischargeEndAddrs[i])
		t.DischargePeriods[i].StopSOC = readScaled(s, dischargeSOCAddrs[i], nil)
		t.DischargePeriods[i].StopVoltage = readScaled(s, dischargeVoltAddrs[i], register.Mul01)
		t.DischargePeriods[i].MaxPower = readScaled(s, dischargePowerAddrs[i], register.Mul10)
	}
	if raw, err := s.Lookup(register.AddrTimedDischargeEnable); err == nil {
		t.DischargeEnabled = raw != 0
	}
}

func readTimePeriod(s *modbus.Session, p *TimePeriod, startAddr, endAddr uint16) {
	if raw, err := s.Lookup(startAddr); err == nil {
		p.StartHour = uint8(raw >> 8)
		p.StartMinute = uint8(raw & 0xFF)
	}
	if raw, err := s.Lookup(endAddr); err == nil {
		p.EndHour = uint8(raw >> 8)
		p.EndMinute = uint8(raw & 0xFF)
	}
}

func readInverterSettings(s *modbus.Session, inv *InverterSettings) {
	inv.OutputVoltage = readScaled(s, register.AddrOutputVoltage, register.Mul01)
	inv.OutputFrequency = readScaled(s, register.AddrOutputFrequency, register.Mul001)
	inv.OutputPhase, _ = s.Lookup(register.AddrOutputPhase)
	inv.OutputPriority, _ = s.Lookup(register.AddrOutputPriority)
	inv.OutputPriorityName = register.OutputPriority(inv.OutputPriority)

	inv.ChargerPriority, _ = s.Lookup(register.AddrChargerPriority)
	inv.ChargerPriorityName = register.ChargerPriority(inv.ChargerPriority)
	inv.ChargeSourceSelection, _ = s.Lookup(register.AddrChargeSourceSelection)
	inv.MaxChargeCurrent = readScaled(s, register.AddrMaxChargeCurrent, register.Mul01)
	inv.MainsChargeCurrentLim = readScaled(s, register.AddrMainsChargeCurrentLim, register.Mul01)

	inv.DeratePower = readScaled(s, register.AddrDeratePower, nil)
	inv.MaxLineCurrent = readScaled(s, register.AddrMaxLineCurrent, register.Mul01)
	inv.MaxLinePower = readScaled(s, register.AddrMaxLinePower, register.Mul10)

	inv.OverloadAutoRestart, _ = s.Lookup(register.AddrOverloadAutoRestart)
	inv.OverTempAutoRestart, _ = s.Lookup(register.AddrOverTempAutoRestart)
	inv.OverloadBypassEnable, _ = s.Lookup(register.AddrOverloadBypassEnable)
	inv.PowerSavingMode, _ = s.Lookup(register.AddrPowerSavingMode)
	inv.ACInputRange, _ = s.Lookup(register.AddrACInputRange)

	inv.AlarmEnable, _ = s.Lookup(register.AddrAlarmEnable)
	inv.AlarmOnInputLoss, _ = s.Lookup(register.AddrAlarmOnInputLoss)
	inv.RecordFaultEnable, _ = s.Lookup(register.AddrRecordFaultEnable)

	inv.RS485Address, _ = s.Lookup(register.AddrInvRS485Address)
	inv.ParallelMode, _ = s.Lookup(register.AddrParallelMode)
	inv.BMSProtocol, _ = s.Lookup(register.AddrBMSProtocol)
	inv.BMSCommunicationEn, _ = s.Lookup(register.AddrBMSCommunicationEn)
	inv.BMSErrorStopEnable, _ = s.Lookup(register.AddrBMSErrorStopEnable)

	inv.GeneratorWorkMode, _ = s.Lookup(register.AddrGeneratorWorkMode)
	inv.GenChargeMaxCurrent = readScaled(s, register.AddrGenChargeMaxCurrent, register.Mul01)
	inv.GeneratorRatedPower = readScaled(s, register.AddrGeneratorRatedPower, nil)
}
