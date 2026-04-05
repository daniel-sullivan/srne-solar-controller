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
	"output_voltage":           {register.AddrOutputVoltage, func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"output_frequency":         {register.AddrOutputFrequency, func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.01) }},
	"output_priority":          {register.AddrOutputPriority, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charger_priority":         {register.AddrChargerPriority, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"max_charge_current":       {register.AddrMaxChargeCurrent, func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"mains_charge_current_lim": {register.AddrMainsChargeCurrentLim, func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"max_line_current":         {register.AddrMaxLineCurrent, func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},
	"derate_power":             {register.AddrDeratePower, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},

	// Toggles
	"parallel_mode":          {register.AddrParallelMode, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"power_saving_mode":      {register.AddrPowerSavingMode, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"overload_auto_restart":  {register.AddrOverloadAutoRestart, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"over_temp_auto_restart": {register.AddrOverTempAutoRestart, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"overload_bypass_enable": {register.AddrOverloadBypassEnable, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"alarm_enable":           {register.AddrAlarmEnable, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"bms_communication_en":   {register.AddrBMSCommunicationEn, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"bms_error_stop_enable":  {register.AddrBMSErrorStopEnable, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"record_fault_enable":    {register.AddrRecordFaultEnable, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"timed_charge_enable":    {register.AddrTimedChargeEnable, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},
	"timed_discharge_enable": {register.AddrTimedDischargeEnable, func(v string, _ float64) (uint16, error) { return encodeBool(v) }},

	// Battery system
	"nominal_capacity":        {register.AddrNominalBatteryCapAH, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"pv_charge_current_limit": {register.AddrPVChargeCurrentLimit, func(v string, _ float64) (uint16, error) { return encodeFloat(v, 0.1) }},

	// SOC thresholds
	"stop_charge_soc":       {register.AddrStopChargeSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"low_soc_alarm":         {register.AddrLowSOCAlarm, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"soc_switch_to_mains":   {register.AddrSOCSwitchToMains, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"soc_switch_to_battery": {register.AddrSOCSwitchToBattery, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},

	// Temperature limits
	"charge_max_temp":    {register.AddrChargeMaxTemp, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_min_temp":    {register.AddrChargeMinTemp, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_max_temp": {register.AddrDischargeMaxTemp, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_min_temp": {register.AddrDischargeMinTemp, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},

	// Timed charge periods
	"charge_start_time_1": {register.AddrChargeStartTime1, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_end_time_1":   {register.AddrChargeEndTime1, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_start_time_2": {register.AddrChargeStartTime2, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_end_time_2":   {register.AddrChargeEndTime2, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_start_time_3": {register.AddrChargeStartTime3, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_end_time_3":   {register.AddrChargeEndTime3, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"charge_1_stop_soc":   {register.AddrCharge1StopSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_2_stop_soc":   {register.AddrCharge2StopSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_3_stop_soc":   {register.AddrCharge3StopSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"charge_1_max_power":  {register.AddrCharge1MaxPower, func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"charge_2_max_power":  {register.AddrCharge2MaxPower, func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"charge_3_max_power":  {register.AddrCharge3MaxPower, func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},

	// Timed discharge periods
	"discharge_start_time_1": {register.AddrDischargeStartTime1, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_end_time_1":   {register.AddrDischargeEndTime1, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_start_time_2": {register.AddrDischargeStartTime2, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_end_time_2":   {register.AddrDischargeEndTime2, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_start_time_3": {register.AddrDischargeStartTime3, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_end_time_3":   {register.AddrDischargeEndTime3, func(v string, _ float64) (uint16, error) { return encodeTime(v) }},
	"discharge_1_stop_soc":   {register.AddrDischarge1StopSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_2_stop_soc":   {register.AddrDischarge2StopSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_3_stop_soc":   {register.AddrDischarge3StopSOC, func(v string, _ float64) (uint16, error) { return encodeInt(v) }},
	"discharge_1_max_power":  {register.AddrDischarge1MaxPower, func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"discharge_2_max_power":  {register.AddrDischarge2MaxPower, func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},
	"discharge_3_max_power":  {register.AddrDischarge3MaxPower, func(v string, _ float64) (uint16, error) { return encodeMul10(v) }},

	// 12V-base voltages (need system voltage for encoding)
	"over_voltage_protection": {register.AddrOverVoltageProtection, encode12VBase},
	"limited_charge_voltage":  {register.AddrLimitedChargeVoltage, encode12VBase},
	"equalizing_charge_volt":  {register.AddrEqualizingChargeVolt, encode12VBase},
	"boost_charge_voltage":    {register.AddrBoostChargeVoltage, encode12VBase},
	"float_charge_voltage":    {register.AddrFloatChargeVoltage, encode12VBase},
	"boost_return_voltage":    {register.AddrBoostReturnVoltage, encode12VBase},
	"over_discharge_voltage":  {register.AddrOverDischargeVoltage, encode12VBase},
	"over_discharge_return_v": {register.AddrOverDischargeReturnV, encode12VBase},
	"limited_discharge_volt":  {register.AddrLimitedDischargeVolt, encode12VBase},
	"under_voltage_warning":   {register.AddrUnderVoltageWarning, encode12VBase},
	"mains_switching_voltage": {register.AddrMainsSwitchingVoltage, encode12VBase},
	"inverter_switching_volt": {register.AddrInverterSwitchingVolt, encode12VBase},
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
