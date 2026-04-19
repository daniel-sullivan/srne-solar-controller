package inverter

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// settingField defines how to encode a human-readable value back to a raw register value.
type settingField struct {
	Addr       uint16
	EncodeFunc func(value string, sysVoltage float64) (uint16, error)
	// For packed registers: non-zero PackedMask enables read-modify-write.
	// Only the bits selected by PackedMask are written; the rest are preserved.
	PackedMask  uint16
	PackedShift uint8
}

// encode helpers
func encodeFloat(value string, scale float64) (uint16, error) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return uint16(math.Round(f / scale)), nil
}

func encodeInt(value string) (uint16, error) {
	i, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(i), nil
}

func encodeSignedInt(value string) (uint16, error) {
	i, err := strconv.ParseInt(value, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(int16(i)), nil
}

func encodeBool(value string) (uint16, error) {
	switch value {
	case "1", "true", "on":
		return 1, nil
	case "0", "false", "off":
		return 0, nil
	default:
		return 0, fmt.Errorf("invalid boolean: %q", value)
	}
}

// encodeTime packs "HH:MM" into a uint16 (high byte = hour, low byte = minute).
func encodeTime(value string) (uint16, error) {
	var h, m int
	n, err := fmt.Sscanf(value, "%d:%d", &h, &m)
	if err != nil || n != 2 {
		return 0, fmt.Errorf("invalid time %q, expected HH:MM", value)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("time out of range: %q", value)
	}
	return uint16(h)<<8 | uint16(m), nil
}

// encodeMul10 encodes a value with ×10 scaling (e.g. power in 10W units).
func encodeMul10(value string) (uint16, error) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return uint16(math.Round(f / 10.0)), nil
}

func encode12VBase(value string, sysVoltage float64) (uint16, error) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	// Voltage12V scale: raw * 0.1 * (sysV / 12)
	// Reverse: raw = value / (0.1 * sysV / 12) = value * 12 / (0.1 * sysV)
	if sysVoltage == 0 {
		sysVoltage = 48
	}
	raw := f * 12.0 / (0.1 * sysVoltage)
	return uint16(math.Round(raw)), nil
}

// settingFields maps field names to register addresses and encoding functions.
var settingFields = map[string]settingField{
	// Inverter output
	"output_voltage":           {Addr: register.AddrOutputVoltage, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"output_frequency":         {Addr: register.AddrOutputFrequency, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.01) }},
	"output_priority":          {Addr: register.AddrOutputPriority, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charger_priority":         {Addr: register.AddrChargerPriority, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"max_charge_current":       {Addr: register.AddrMaxChargeCurrent, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"mains_charge_current_lim": {Addr: register.AddrMainsChargeCurrentLim, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"max_line_current":         {Addr: register.AddrMaxLineCurrent, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"derate_power":             {Addr: register.AddrDeratePower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},

	// Toggles
	"parallel_mode":          {Addr: register.AddrParallelMode, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"power_saving_mode":      {Addr: register.AddrPowerSavingMode, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"overload_auto_restart":  {Addr: register.AddrOverloadAutoRestart, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"over_temp_auto_restart": {Addr: register.AddrOverTempAutoRestart, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"overload_bypass_enable": {Addr: register.AddrOverloadBypassEnable, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"alarm_enable":           {Addr: register.AddrAlarmEnable, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"bms_communication_en":   {Addr: register.AddrBMSCommunicationEn, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"bms_error_stop_enable":  {Addr: register.AddrBMSErrorStopEnable, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"record_fault_enable":    {Addr: register.AddrRecordFaultEnable, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"timed_charge_enable":    {Addr: register.AddrTimedChargeEnable, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"timed_discharge_enable": {Addr: register.AddrTimedDischargeEnable, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeBool(v) }},

	// Battery system
	"nominal_capacity":        {Addr: register.AddrNominalBatteryCapAH, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"pv_charge_current_limit": {Addr: register.AddrPVChargeCurrentLimit, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},

	// SOC thresholds
	"stop_charge_soc":       {Addr: register.AddrStopChargeSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"low_soc_alarm":         {Addr: register.AddrLowSOCAlarm, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"soc_switch_to_mains":   {Addr: register.AddrSOCSwitchToMains, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"soc_switch_to_battery": {Addr: register.AddrSOCSwitchToBattery, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},

	// Packed SOC cutoffs (0xE00F: high byte = charge, low byte = discharge)
	"cutoff_charge_soc":    {Addr: register.AddrCutoffSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }, PackedMask: 0xFF00, PackedShift: 8},
	"cutoff_discharge_soc": {Addr: register.AddrCutoffSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }, PackedMask: 0x00FF, PackedShift: 0},

	// Temperature limits
	"charge_max_temp":    {Addr: register.AddrChargeMaxTemp, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeSignedInt(v) }},
	"charge_min_temp":    {Addr: register.AddrChargeMinTemp, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeSignedInt(v) }},
	"discharge_max_temp": {Addr: register.AddrDischargeMaxTemp, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeSignedInt(v) }},
	"discharge_min_temp": {Addr: register.AddrDischargeMinTemp, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeSignedInt(v) }},

	// Timed charge periods
	"charge_start_time_1": {Addr: register.AddrChargeStartTime1, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_end_time_1":   {Addr: register.AddrChargeEndTime1, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_start_time_2": {Addr: register.AddrChargeStartTime2, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_end_time_2":   {Addr: register.AddrChargeEndTime2, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_start_time_3": {Addr: register.AddrChargeStartTime3, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_end_time_3":   {Addr: register.AddrChargeEndTime3, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_1_stop_soc":   {Addr: register.AddrCharge1StopSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_2_stop_soc":   {Addr: register.AddrCharge2StopSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_3_stop_soc":   {Addr: register.AddrCharge3StopSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_1_max_power":  {Addr: register.AddrCharge1MaxPower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"charge_2_max_power":  {Addr: register.AddrCharge2MaxPower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"charge_3_max_power":  {Addr: register.AddrCharge3MaxPower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},

	// Timed discharge periods
	"discharge_start_time_1": {Addr: register.AddrDischargeStartTime1, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_end_time_1":   {Addr: register.AddrDischargeEndTime1, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_start_time_2": {Addr: register.AddrDischargeStartTime2, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_end_time_2":   {Addr: register.AddrDischargeEndTime2, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_start_time_3": {Addr: register.AddrDischargeStartTime3, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_end_time_3":   {Addr: register.AddrDischargeEndTime3, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_1_stop_soc":   {Addr: register.AddrDischarge1StopSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_2_stop_soc":   {Addr: register.AddrDischarge2StopSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_3_stop_soc":   {Addr: register.AddrDischarge3StopSOC, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_1_max_power":  {Addr: register.AddrDischarge1MaxPower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"discharge_2_max_power":  {Addr: register.AddrDischarge2MaxPower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"discharge_3_max_power":  {Addr: register.AddrDischarge3MaxPower, EncodeFunc: func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},

	// 12V-base voltages (need system voltage for encoding)
	"over_voltage_protection": {Addr: register.AddrOverVoltageProtection, EncodeFunc: encode12VBase},
	"limited_charge_voltage":  {Addr: register.AddrLimitedChargeVoltage, EncodeFunc: encode12VBase},
	"equalizing_charge_volt":  {Addr: register.AddrEqualizingChargeVolt, EncodeFunc: encode12VBase},
	"boost_charge_voltage":    {Addr: register.AddrBoostChargeVoltage, EncodeFunc: encode12VBase},
	"float_charge_voltage":    {Addr: register.AddrFloatChargeVoltage, EncodeFunc: encode12VBase},
	"boost_return_voltage":    {Addr: register.AddrBoostReturnVoltage, EncodeFunc: encode12VBase},
	"over_discharge_voltage":  {Addr: register.AddrOverDischargeVoltage, EncodeFunc: encode12VBase},
	"over_discharge_return_v": {Addr: register.AddrOverDischargeReturnV, EncodeFunc: encode12VBase},
	"limited_discharge_volt":  {Addr: register.AddrLimitedDischargeVolt, EncodeFunc: encode12VBase},
	"under_voltage_warning":   {Addr: register.AddrUnderVoltageWarning, EncodeFunc: encode12VBase},
	"mains_switching_voltage": {Addr: register.AddrMainsSwitchingVoltage, EncodeFunc: encode12VBase},
	"inverter_switching_volt": {Addr: register.AddrInverterSwitchingVolt, EncodeFunc: encode12VBase},
}

// WriteSetting writes a named setting with a human-readable value to all units.
func (s *System) WriteSetting(ctx context.Context, field string, value string) error {
	sf, ok := settingFields[field]
	if !ok {
		return fmt.Errorf("unknown setting: %q", field)
	}

	// Get system voltage for 12V-base encoding
	var sysVoltage float64
	s.mu.RLock()
	if len(s.units) > 0 {
		v, err := s.units[0].session.Lookup(register.AddrSystemVoltage)
		if err == nil {
			sysVoltage = float64(v)
		}
	}
	s.mu.RUnlock()

	raw, err := sf.EncodeFunc(value, sysVoltage)
	if err != nil {
		return fmt.Errorf("encode %q=%q: %w", field, value, err)
	}

	// Packed register: read-modify-write to preserve other bits
	if sf.PackedMask != 0 {
		s.mu.RLock()
		session := s.units[0].session
		s.mu.RUnlock()
		current, lookupErr := session.Lookup(sf.Addr)
		if lookupErr != nil {
			return fmt.Errorf("read packed register 0x%04X: %w", sf.Addr, lookupErr)
		}
		raw = (current &^ sf.PackedMask) | ((raw << sf.PackedShift) & sf.PackedMask)
	}

	return s.WriteRegister(ctx, sf.Addr, raw)
}

// SettingFields returns the list of valid field names for WriteSetting.
func SettingFields() []string {
	fields := make([]string, 0, len(settingFields))
	for k := range settingFields {
		fields = append(fields, k)
	}
	return fields
}
