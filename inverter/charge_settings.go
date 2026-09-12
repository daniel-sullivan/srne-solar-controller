package inverter

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/register"
)

const (
	verifiedWriteAttempts   = 3
	verifiedWriteRetryDelay = 300 * time.Millisecond
)

// RequiredChargeRegisters defines the registers that must be readable on every inverter unit
// to form a valid charge settings snapshot.
var RequiredChargeRegisters = []uint16{
	register.AddrSystemVoltage,        // voltage scaling must be known
	register.AddrBatteryType,          // profile determines whether voltage writes are permitted
	register.AddrLimitedChargeVoltage, // 0xE006
	register.AddrEqualizingChargeVolt, // 0xE007
	register.AddrBoostChargeVoltage,   // 0xE008
	register.AddrFloatChargeVoltage,   // 0xE009
	register.AddrStopChargeSOC,        // 0xE01D
	register.AddrEqualizingChargeEn,   // 0xE206
	register.AddrMaxChargeCurrent,     // 0xE20A
	register.AddrBMSCommunicationEn,   // 0xE215
}

// OptionalChargeRegisters defines additional charge profile caps and configuration
// registers that gate the installed profile when supported by the unit firmware.
var OptionalChargeRegisters = []uint16{
	register.AddrOverVoltageProtection, // 0xE005
	register.AddrCutoffSOC,             // 0xE00F
	register.AddrPVChargeCurrentLimit,  // 0xE001
	register.AddrMainsChargeCurrentLim, // 0xE205
}

// UnitChargeSettings captures the narrow charge profile and cap registers for a single
// inverter unit, preserving both typed engineering values and raw register values.
type UnitChargeSettings struct {
	UnitIndex int    `json:"unit_index"`
	Host      string `json:"host"`
	Serial    string `json:"serial"`

	// System & Battery Profile
	SystemVoltage float64 `json:"system_voltage"` // V (0xE003)
	BatteryType   uint16  `json:"battery_type"`   // (0xE004, e.g. 6 = LiFePO4(BMS))

	// Voltage Thresholds (12V-base scaled to system voltage, Volts)
	OverVoltageProtection float64 `json:"over_voltage_protection"` // V (0xE005)
	LimitedChargeVoltage  float64 `json:"limited_charge_voltage"`  // V (0xE006)
	EqualizingChargeVolt  float64 `json:"equalizing_charge_volt"`  // V (0xE007)
	BoostChargeVoltage    float64 `json:"boost_charge_voltage"`    // V (0xE008)
	FloatChargeVoltage    float64 `json:"float_charge_voltage"`    // V (0xE009)

	// Current Limits (Amperes)
	MaxChargeCurrent      float64 `json:"max_charge_current"`       // A (0xE20A)
	MainsChargeCurrentLim float64 `json:"mains_charge_current_lim"` // A (0xE205)
	PVChargeCurrentLimit  float64 `json:"pv_charge_current_limit"`  // A (0xE001)

	// Control & Communication
	BMSCommunicationEn uint16 `json:"bms_communication_en"` // 0=disabled, 1=RS485, 2=CAN (0xE215)
	EqualizingChargeEn bool   `json:"equalizing_charge_en"` // (0xE206)

	// SOC Thresholds (observation/snapshot)
	StopChargeSOC   float64 `json:"stop_charge_soc"`   // % (0xE01D)
	CutoffChargeSOC uint8   `json:"cutoff_charge_soc"` // % (0xE00F high byte)

	// Raw registers captured during readback
	RawRegisters map[uint16]uint16 `json:"raw_registers"`
}

// RegisterWriteError is returned when a verified register write fails, times out,
// or when the readback value mismatches the written value.
type RegisterWriteError struct {
	UnitIndex          int
	Host               string
	Address            uint16
	Wrote              uint16
	Read               uint16
	BatteryType        uint16
	BMSCommunicationEn uint16
	Err                error
}

func (e *RegisterWriteError) Error() string {
	unitDesc := fmt.Sprintf("unit %d", e.UnitIndex)
	if e.Host != "" {
		unitDesc = fmt.Sprintf("unit %d (%s)", e.UnitIndex, e.Host)
	}

	if e.Err != nil {
		if isVoltageRegister(e.Address) && e.BatteryType == 6 {
			return fmt.Sprintf("%s: write 0x%04X: %v (battery type 6 LiFePO4(BMS) refused voltage write; unapproved battery type change prohibited)",
				unitDesc, e.Address, e.Err)
		}
		return fmt.Sprintf("%s: write 0x%04X: %v", unitDesc, e.Address, e.Err)
	}

	if isVoltageRegister(e.Address) && e.BatteryType == 6 {
		return fmt.Sprintf("%s: write 0x%04X not applied: wrote %d, read %d (battery type 6 LiFePO4(BMS) refused voltage write; unapproved battery type change prohibited)",
			unitDesc, e.Address, e.Wrote, e.Read)
	}

	return fmt.Sprintf("%s: write 0x%04X not applied: wrote %d, read %d (register locked or unsupported)",
		unitDesc, e.Address, e.Wrote, e.Read)
}

func (e *RegisterWriteError) Unwrap() error {
	return e.Err
}

func isVoltageRegister(addr uint16) bool {
	switch addr {
	case register.AddrLimitedChargeVoltage,
		register.AddrEqualizingChargeVolt,
		register.AddrBoostChargeVoltage,
		register.AddrFloatChargeVoltage,
		register.AddrOverVoltageProtection,
		register.AddrBoostReturnVoltage,
		register.AddrOverDischargeReturnV,
		register.AddrUnderVoltageWarning,
		register.AddrOverDischargeVoltage,
		register.AddrLimitedDischargeVolt,
		register.AddrMainsSwitchingVoltage,
		register.AddrInverterSwitchingVolt:
		return true
	default:
		return false
	}
}

// Encode12VBaseVoltage encodes a target voltage in Volts to the raw 12V-base register value.
func Encode12VBaseVoltage(volts float64, sysVoltage float64) uint16 {
	if sysVoltage == 0 {
		sysVoltage = 48.0
	}
	raw := volts * 12.0 / (0.1 * sysVoltage)
	return uint16(math.Round(raw))
}

// Decode12VBaseVoltage decodes a raw 12V-base register value to Volts using the system voltage.
func Decode12VBaseVoltage(raw uint16, sysVoltage float64) float64 {
	if sysVoltage == 0 {
		sysVoltage = 48.0
	}
	return float64(raw) * 0.1 * (sysVoltage / 12.0)
}

// EncodeCurrent encodes an amperage value to the raw 0.1A scaled register value.
func EncodeCurrent(amps float64) uint16 {
	return uint16(math.Round(amps / 0.1))
}

// DecodeCurrent decodes a raw register value to Amperes.
func DecodeCurrent(raw uint16) float64 {
	return float64(raw) * 0.1
}

// ReadUnitChargeSettings reads the narrow charge profile and cap registers from a specific unit.
// Preserves raw register values and parses them into typed engineering units.
func (s *System) ReadUnitChargeSettings(ctx context.Context, unitIdx int) (*UnitChargeSettings, error) {
	s.mu.RLock()
	if unitIdx < 0 || unitIdx >= len(s.units) {
		count := len(s.units)
		s.mu.RUnlock()
		return nil, fmt.Errorf("invalid unit index %d (system has %d units)", unitIdx, count)
	}
	u := s.units[unitIdx]
	s.mu.RUnlock()

	u.session.Ctx = ctx
	raw := make(map[uint16]uint16)

	unitDesc := fmt.Sprintf("unit %d", unitIdx)
	if u.info.Host != "" {
		unitDesc = fmt.Sprintf("unit %d (%s)", unitIdx, u.info.Host)
	}

	// Required registers: must succeed
	for _, addr := range RequiredChargeRegisters {
		vals, err := u.session.ReadRegisters(addr, 1)
		if err != nil {
			return nil, fmt.Errorf("%s: read register 0x%04X: %w", unitDesc, addr, err)
		}
		if len(vals) != 1 {
			return nil, fmt.Errorf("%s: read register 0x%04X returned %d values", unitDesc, addr, len(vals))
		}
		raw[addr] = vals[0]
	}
	if raw[register.AddrSystemVoltage] == 0 {
		return nil, fmt.Errorf("%s: system voltage register is zero", unitDesc)
	}

	// Optional charge cap registers: read if available
	for _, addr := range OptionalChargeRegisters {
		vals, err := u.session.ReadRegisters(addr, 1)
		if err == nil && len(vals) > 0 {
			raw[addr] = vals[0]
		}
	}

	// Cache the raw values into the session for fast lookup
	for addr, v := range raw {
		u.session.Store(addr, []uint16{v})
	}

	sysVoltage := 48.0
	if rawSysV, ok := raw[register.AddrSystemVoltage]; ok && rawSysV > 0 {
		sysVoltage = float64(rawSysV)
	}

	settings := &UnitChargeSettings{
		UnitIndex:            unitIdx,
		Host:                 u.info.Host,
		Serial:               u.info.Serial,
		SystemVoltage:        sysVoltage,
		BatteryType:          raw[register.AddrBatteryType],
		LimitedChargeVoltage: Decode12VBaseVoltage(raw[register.AddrLimitedChargeVoltage], sysVoltage),
		EqualizingChargeVolt: Decode12VBaseVoltage(raw[register.AddrEqualizingChargeVolt], sysVoltage),
		BoostChargeVoltage:   Decode12VBaseVoltage(raw[register.AddrBoostChargeVoltage], sysVoltage),
		FloatChargeVoltage:   Decode12VBaseVoltage(raw[register.AddrFloatChargeVoltage], sysVoltage),
		StopChargeSOC:        float64(raw[register.AddrStopChargeSOC]),
		EqualizingChargeEn:   raw[register.AddrEqualizingChargeEn] != 0,
		MaxChargeCurrent:     float64(raw[register.AddrMaxChargeCurrent]) * 0.1,
		BMSCommunicationEn:   raw[register.AddrBMSCommunicationEn],
		RawRegisters:         raw,
	}

	if v, ok := raw[register.AddrOverVoltageProtection]; ok {
		settings.OverVoltageProtection = Decode12VBaseVoltage(v, sysVoltage)
	}
	if v, ok := raw[register.AddrCutoffSOC]; ok {
		settings.CutoffChargeSOC = uint8(v >> 8)
	}
	if v, ok := raw[register.AddrMainsChargeCurrentLim]; ok {
		settings.MainsChargeCurrentLim = float64(v) * 0.1
	}
	if v, ok := raw[register.AddrPVChargeCurrentLimit]; ok {
		settings.PVChargeCurrentLimit = float64(v) * 0.1
	}

	return settings, nil
}

// ReadChargeSettings reads the charge profile and cap registers from all units in the system.
// Each unit's settings are read individually to preserve distinct per-unit values.
func (s *System) ReadChargeSettings(ctx context.Context) ([]UnitChargeSettings, error) {
	s.mu.RLock()
	numUnits := len(s.units)
	s.mu.RUnlock()

	if numUnits == 0 {
		return nil, fmt.Errorf("no units in system")
	}

	settings := make([]UnitChargeSettings, numUnits)
	for i := 0; i < numUnits; i++ {
		p, err := s.ReadUnitChargeSettings(ctx, i)
		if err != nil {
			return nil, err
		}
		settings[i] = *p
	}
	return settings, nil
}

// WriteUnitRegisterVerified writes a register to a specific inverter unit, then reads it
// back to confirm the value was applied. Retries on failure up to verifiedWriteAttempts times.
// Returns a RegisterWriteError if the write fails, times out, or fails readback verification.
func (s *System) WriteUnitRegisterVerified(ctx context.Context, unitIdx int, addr uint16, value uint16) error {
	s.mu.RLock()
	if unitIdx < 0 || unitIdx >= len(s.units) {
		count := len(s.units)
		s.mu.RUnlock()
		return fmt.Errorf("invalid unit index %d (system has %d units)", unitIdx, count)
	}
	u := s.units[unitIdx]
	s.mu.RUnlock()

	u.session.Ctx = ctx
	var lastErr error
	var readVal uint16

	for attempt := 1; attempt <= verifiedWriteAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(verifiedWriteRetryDelay):
			}
		}

		if err := u.session.WriteSingleRegister(addr, value); err != nil {
			lastErr = err
			continue
		}

		got, err := u.session.ReadRegisters(addr, 1)
		if err != nil {
			lastErr = err
			continue
		}

		readVal = got[0]
		if readVal == value {
			return nil
		}

		// Readback mismatch
		lastErr = nil
	}

	var batteryType uint16
	if bt, err := u.session.Lookup(register.AddrBatteryType); err == nil {
		batteryType = bt
	}
	var bmsComm uint16
	if bc, err := u.session.Lookup(register.AddrBMSCommunicationEn); err == nil {
		bmsComm = bc
	}

	return &RegisterWriteError{
		UnitIndex:          unitIdx,
		Host:               u.info.Host,
		Address:            addr,
		Wrote:              value,
		Read:               readVal,
		BatteryType:        batteryType,
		BMSCommunicationEn: bmsComm,
		Err:                lastErr,
	}
}

// WriteRegisterVerified writes a register to all units sequentially, verifying each unit's
// readback. If any unit fails or mismatches, writing aborts immediately and returns an
// explicit error identifying the unit and register.
func (s *System) WriteRegisterVerified(ctx context.Context, addr uint16, value uint16) error {
	s.mu.RLock()
	numUnits := len(s.units)
	s.mu.RUnlock()

	if numUnits == 0 {
		return fmt.Errorf("no units in system")
	}

	for i := 0; i < numUnits; i++ {
		if err := s.WriteUnitRegisterVerified(ctx, i, addr, value); err != nil {
			return err
		}
	}
	return nil
}

// RestoreUnitChargeSettings restores a unit's charge configuration from a previously
// captured UnitChargeSettings snapshot.
//
// Safety and restoration sequencing:
// Restoration ensures the charging current remains suppressed (or at safe 0A) until
// target voltages, equalization enable, and BMS communication (CAN) are fully applied
// and verified. Specifically:
//  1. Charge voltages (E006, E007, E008, E009) and equalization enable (E206) are restored first.
//  2. BMS communication enable (E215) is restored and verified, establishing CAN comms.
//  3. The unit's prior max charge current (E20A) is restored LAST, only after voltage,
//     equalization, and CAN are all confirmed active.
//
// Stop charge SOC (0xE01D) and battery type (0xE004) are preserved and not modified.
// Unrelated optional current limits (e.g. PV/mains caps) are not touched without need.
func (s *System) RestoreUnitChargeSettings(ctx context.Context, unitIdx int, target UnitChargeSettings) error {
	s.mu.RLock()
	if unitIdx < 0 || unitIdx >= len(s.units) {
		count := len(s.units)
		s.mu.RUnlock()
		return fmt.Errorf("invalid unit index %d (system has %d units)", unitIdx, count)
	}
	s.mu.RUnlock()

	sysV := target.SystemVoltage
	if sysV == 0 {
		sysV = 48.0
	}

	getVal := func(addr uint16, fallback func() uint16) (uint16, bool) {
		if target.RawRegisters != nil {
			if v, ok := target.RawRegisters[addr]; ok {
				return v, true
			}
		}
		if fallback != nil {
			return fallback(), true
		}
		return 0, false
	}

	// 1. Charge voltages (E006, E007, E008, E009)
	if val, ok := getVal(register.AddrLimitedChargeVoltage, func() uint16 { return Encode12VBaseVoltage(target.LimitedChargeVoltage, sysV) }); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrLimitedChargeVoltage, val); err != nil {
			return err
		}
	}
	if val, ok := getVal(register.AddrEqualizingChargeVolt, func() uint16 { return Encode12VBaseVoltage(target.EqualizingChargeVolt, sysV) }); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrEqualizingChargeVolt, val); err != nil {
			return err
		}
	}
	if val, ok := getVal(register.AddrBoostChargeVoltage, func() uint16 { return Encode12VBaseVoltage(target.BoostChargeVoltage, sysV) }); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrBoostChargeVoltage, val); err != nil {
			return err
		}
	}
	if val, ok := getVal(register.AddrFloatChargeVoltage, func() uint16 { return Encode12VBaseVoltage(target.FloatChargeVoltage, sysV) }); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrFloatChargeVoltage, val); err != nil {
			return err
		}
	}

	// 2. Equalizing enable (E206)
	if val, ok := getVal(register.AddrEqualizingChargeEn, func() uint16 {
		if target.EqualizingChargeEn {
			return 1
		}
		return 0
	}); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrEqualizingChargeEn, val); err != nil {
			return err
		}
	}

	// 3. BMS communication enable (restore CAN and verify)
	if val, ok := getVal(register.AddrBMSCommunicationEn, func() uint16 { return target.BMSCommunicationEn }); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrBMSCommunicationEn, val); err != nil {
			return err
		}
	}

	// 4. Prior max charge current (E20A) - RESTORED LAST after voltages, equalization, and CAN are verified!
	if val, ok := getVal(register.AddrMaxChargeCurrent, func() uint16 { return EncodeCurrent(target.MaxChargeCurrent) }); ok {
		if err := s.WriteUnitRegisterVerified(ctx, unitIdx, register.AddrMaxChargeCurrent, val); err != nil {
			return err
		}
	}

	return nil
}
