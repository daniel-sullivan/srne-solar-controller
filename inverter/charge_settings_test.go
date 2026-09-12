package inverter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

func newTestUnit(regs map[uint16]uint16) *mock.Inverter {
	inv := mock.NewInverter(regs)
	_ = inv.Connect()
	return inv
}

// ignoredWriteClient wraps mock.Inverter and simulates an inverter that accepts a write
// but ignores it (keeps the old register value), avoiding mutex reentrancy.
type ignoredWriteClient struct {
	*mock.Inverter
	ignoredAddr uint16
}

func (c *ignoredWriteClient) WriteSingleRegister(addr uint16, value uint16) error {
	if addr == c.ignoredAddr {
		// Simulate write accepted by dongle/bus but ignored by firmware (e.g. locked register)
		return nil
	}
	return c.Inverter.WriteSingleRegister(addr, value)
}

// TestReadChargeSettingsPreservesPerUnitValues verifies that in a two-unit parallel system,
// per-unit charge settings with different prior values are read back independently
// without assuming or forcing both units to match.
func TestReadChargeSettingsPreservesPerUnitValues(t *testing.T) {
	// Unit 0: Installed initial profile (100A, 58.4V, CAN enabled)
	u0Regs := map[uint16]uint16{
		register.AddrSystemVoltage:         48,
		register.AddrBatteryType:           6,    // LiFePO4(BMS)
		register.AddrLimitedChargeVoltage:  144,  // 57.6V
		register.AddrEqualizingChargeVolt:  146,  // 58.4V
		register.AddrBoostChargeVoltage:    146,  // 58.4V
		register.AddrFloatChargeVoltage:    146,  // 58.4V
		register.AddrStopChargeSOC:         90,   // 90%
		register.AddrCutoffSOC:             0x00, // 0%
		register.AddrEqualizingChargeEn:    1,    // enabled
		register.AddrMaxChargeCurrent:      1000, // 100.0A
		register.AddrMainsChargeCurrentLim: 0,    // 0A
		register.AddrPVChargeCurrentLimit:  1000, // 100.0A
		register.AddrOverVoltageProtection: 150,  // 60.0V
		register.AddrBMSCommunicationEn:    2,    // CAN
	}

	// Unit 1: Distinct prior values (e.g. 80A, 57.6V, equalizing disabled, RS485 enabled)
	u1Regs := map[uint16]uint16{
		register.AddrSystemVoltage:         48,
		register.AddrBatteryType:           6,
		register.AddrLimitedChargeVoltage:  142,  // 56.8V
		register.AddrEqualizingChargeVolt:  144,  // 57.6V
		register.AddrBoostChargeVoltage:    144,  // 57.6V
		register.AddrFloatChargeVoltage:    144,  // 57.6V
		register.AddrStopChargeSOC:         85,   // 85%
		register.AddrCutoffSOC:             0x05, // 5%
		register.AddrEqualizingChargeEn:    0,    // disabled
		register.AddrMaxChargeCurrent:      800,  // 80.0A
		register.AddrMainsChargeCurrentLim: 200,  // 20.0A
		register.AddrPVChargeCurrentLimit:  800,  // 80.0A
		register.AddrOverVoltageProtection: 148,  // 59.2V
		register.AddrBMSCommunicationEn:    1,    // RS485
	}

	client0 := newTestUnit(u0Regs)
	client1 := newTestUnit(u1Regs)

	sys := NewSystem([]modbus.Client{client0, client1}, []string{"192.168.1.10", "192.168.1.11"})

	profiles, err := sys.ReadChargeSettings(context.Background())
	require.NoError(t, err)
	require.Len(t, profiles, 2)

	// Verify Unit 0
	assert.Equal(t, 0, profiles[0].UnitIndex)
	assert.Equal(t, "192.168.1.10", profiles[0].Host)
	assert.Equal(t, uint16(6), profiles[0].BatteryType)
	assert.InDelta(t, 57.6, profiles[0].LimitedChargeVoltage, 0.05)
	assert.InDelta(t, 58.4, profiles[0].EqualizingChargeVolt, 0.05)
	assert.InDelta(t, 58.4, profiles[0].BoostChargeVoltage, 0.05)
	assert.InDelta(t, 58.4, profiles[0].FloatChargeVoltage, 0.05)
	assert.InDelta(t, 100.0, profiles[0].MaxChargeCurrent, 0.05)
	assert.Equal(t, uint16(2), profiles[0].BMSCommunicationEn)
	assert.True(t, profiles[0].EqualizingChargeEn)
	assert.Equal(t, float64(90), profiles[0].StopChargeSOC)

	// Verify Unit 1 retains its different prior values
	assert.Equal(t, 1, profiles[1].UnitIndex)
	assert.Equal(t, "192.168.1.11", profiles[1].Host)
	assert.InDelta(t, 56.8, profiles[1].LimitedChargeVoltage, 0.05)
	assert.InDelta(t, 57.6, profiles[1].EqualizingChargeVolt, 0.05)
	assert.InDelta(t, 57.6, profiles[1].BoostChargeVoltage, 0.05)
	assert.InDelta(t, 57.6, profiles[1].FloatChargeVoltage, 0.05)
	assert.InDelta(t, 80.0, profiles[1].MaxChargeCurrent, 0.05)
	assert.Equal(t, uint16(1), profiles[1].BMSCommunicationEn)
	assert.False(t, profiles[1].EqualizingChargeEn)
	assert.Equal(t, float64(85), profiles[1].StopChargeSOC)

	// Verify raw registers are preserved
	assert.Equal(t, uint16(1000), profiles[0].RawRegisters[register.AddrMaxChargeCurrent])
	assert.Equal(t, uint16(800), profiles[1].RawRegisters[register.AddrMaxChargeCurrent])
}

func TestReadChargeSettingsRequiresVoltageScaleAndProfile(t *testing.T) {
	for _, missing := range []uint16{register.AddrSystemVoltage, register.AddrBatteryType} {
		regs := map[uint16]uint16{
			register.AddrSystemVoltage:        48,
			register.AddrBatteryType:          6,
			register.AddrLimitedChargeVoltage: 136,
			register.AddrEqualizingChargeVolt: 136,
			register.AddrBoostChargeVoltage:   136,
			register.AddrFloatChargeVoltage:   136,
			register.AddrStopChargeSOC:        90,
			register.AddrEqualizingChargeEn:   0,
			register.AddrMaxChargeCurrent:     100,
			register.AddrBMSCommunicationEn:   2,
		}
		delete(regs, missing)
		sys := NewSystem([]modbus.Client{newTestUnit(regs)}, []string{"unit-0"})
		_, err := sys.ReadChargeSettings(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unit 0")
	}
}

// TestWriteUnitRegisterVerifiedAndReadback tests writing individual registers to specific units
// and verifying that readback confirms the write took effect.
func TestWriteUnitRegisterVerifiedAndReadback(t *testing.T) {
	client0 := newTestUnit(map[uint16]uint16{
		register.AddrSystemVoltage:        48,
		register.AddrBatteryType:          6,
		register.AddrLimitedChargeVoltage: 144, // 57.6V
		register.AddrEqualizingChargeVolt: 146, // 58.4V
		register.AddrBoostChargeVoltage:   146, // 58.4V
		register.AddrFloatChargeVoltage:   146, // 58.4V
		register.AddrStopChargeSOC:        90,
		register.AddrEqualizingChargeEn:   1,
		register.AddrMaxChargeCurrent:     1000, // 100A
		register.AddrBMSCommunicationEn:   2,
	})

	sys := NewSystem([]modbus.Client{client0}, []string{"inv-0"})

	// Write 10A current limit (raw 100)
	err := sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrMaxChargeCurrent, 100)
	require.NoError(t, err)

	val, ok := client0.GetRegister(register.AddrMaxChargeCurrent)
	require.True(t, ok)
	assert.Equal(t, uint16(100), val)

	// Readback via ReadUnitChargeSettings reflects the verified change
	settings, err := sys.ReadUnitChargeSettings(context.Background(), 0)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, settings.MaxChargeCurrent, 0.01)
}

// TestWriteRegisterVerifiedPartialFailure verifies that WriteRegisterVerified writes
// sequentially to all units, and if a slave unit fails/rejects the write, it halts
// immediately and returns an explicit error identifying the failed unit and register.
func TestWriteRegisterVerifiedPartialFailure(t *testing.T) {
	client0 := newTestUnit(map[uint16]uint16{
		register.AddrMaxChargeCurrent: 1000,
	})
	client1 := newTestUnit(map[uint16]uint16{
		register.AddrMaxChargeCurrent: 1000,
	})

	// Inject error on unit 1
	client1.WriteHook = func(addr, val uint16) error {
		if addr == register.AddrMaxChargeCurrent {
			return &modbus.ModbusError{FunctionCode: modbus.FuncWriteSingleRegister, ExceptionCode: 0x04}
		}
		return nil
	}

	sys := NewSystem([]modbus.Client{client0, client1}, []string{"inv-master", "inv-slave"})

	err := sys.WriteRegisterVerified(context.Background(), register.AddrMaxChargeCurrent, 100)
	require.Error(t, err)

	// Error must identify unit 1 and register 0xE20A
	assert.Contains(t, err.Error(), "unit 1")
	assert.Contains(t, err.Error(), "inv-slave")
	assert.Contains(t, err.Error(), "0xE20A")

	// Unit 0 was updated and verified
	val0, _ := client0.GetRegister(register.AddrMaxChargeCurrent)
	assert.Equal(t, uint16(100), val0)

	// Unit 1 was not updated
	val1, _ := client1.GetRegister(register.AddrMaxChargeCurrent)
	assert.Equal(t, uint16(1000), val1)

	// Verify structured error inspection
	var writeErr *RegisterWriteError
	require.True(t, errors.As(err, &writeErr))
	assert.Equal(t, 1, writeErr.UnitIndex)
	assert.Equal(t, uint16(register.AddrMaxChargeCurrent), writeErr.Address)
}

// TestWriteUnitRegisterVerifiedIgnoredOrLocked simulates an inverter that accepts or ignores
// a write without updating the internal register (or where the register is locked).
// If battery type is 6 (LiFePO4(BMS)) and writing a voltage register, it must return
// a clear error explaining that battery type 6 LiFePO4(BMS) refused the voltage write
// rather than modifying the battery type unapproved.
func TestWriteUnitRegisterVerifiedIgnoredOrLocked(t *testing.T) {
	inner := newTestUnit(map[uint16]uint16{
		register.AddrSystemVoltage:      48,
		register.AddrBatteryType:        6,   // LiFePO4(BMS)
		register.AddrFloatChargeVoltage: 146, // 58.4V
		register.AddrBMSCommunicationEn: 0,   // CAN suspended
	})

	// Wrap in ignoredWriteClient to simulate locked AddrFloatChargeVoltage write without reentering locks
	client := &ignoredWriteClient{
		Inverter:    inner,
		ignoredAddr: register.AddrFloatChargeVoltage,
	}

	sys := NewSystem([]modbus.Client{client}, []string{"192.168.1.10"})

	// Attempt to set CV stage 54.4V (raw 136)
	err := sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrFloatChargeVoltage, 136)
	require.Error(t, err)

	// Error message must report mismatch and explain battery type 6 lock
	assert.Contains(t, err.Error(), "unit 0")
	assert.Contains(t, err.Error(), "0xE009")
	assert.Contains(t, err.Error(), "wrote 136, read 146")
	assert.Contains(t, err.Error(), "battery type 6 LiFePO4(BMS) refused voltage write")
	assert.Contains(t, err.Error(), "unapproved battery type change prohibited")

	// Ensure battery type was NOT changed unapproved
	bt, _ := client.GetRegister(register.AddrBatteryType)
	assert.Equal(t, uint16(6), bt, "battery type must never be modified automatically")
}

// TestBMSCommunicationEnRoundTrip verifies that BMSCommunicationEn (0xE215) can round-trip
// between 2 (CAN) and 0 (suspended/disabled) without boolean truncation, both via
// verified per-unit writes and via WriteSetting.
func TestBMSCommunicationEnRoundTrip(t *testing.T) {
	client := newTestUnit(map[uint16]uint16{
		register.AddrSystemVoltage:      48,
		register.AddrBMSCommunicationEn: 2, // starts at CAN
	})

	sys := NewSystem([]modbus.Client{client}, []string{"inv-0"})

	// 1. Suspend CAN via WriteUnitRegisterVerified (2 -> 0)
	require.NoError(t, sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrBMSCommunicationEn, 0))
	val, ok := client.GetRegister(register.AddrBMSCommunicationEn)
	require.True(t, ok)
	assert.Equal(t, uint16(0), val, "BMS comm should be suspended (0)")

	// 2. Restore CAN via WriteUnitRegisterVerified (0 -> 2)
	require.NoError(t, sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrBMSCommunicationEn, 2))
	val, ok = client.GetRegister(register.AddrBMSCommunicationEn)
	require.True(t, ok)
	assert.Equal(t, uint16(2), val, "BMS comm should be restored to CAN (2)")

	// 3. Test generic WriteSetting path with enum strings
	require.NoError(t, sys.WriteSetting(context.Background(), "bms_communication_en", "0"))
	val, _ = client.GetRegister(register.AddrBMSCommunicationEn)
	assert.Equal(t, uint16(0), val)

	require.NoError(t, sys.WriteSetting(context.Background(), "bms_communication_en", "2"))
	val, _ = client.GetRegister(register.AddrBMSCommunicationEn)
	assert.Equal(t, uint16(2), val, "WriteSetting('bms_communication_en', '2') must succeed and not be restricted to bool")

	// Also test textual names
	require.NoError(t, sys.WriteSetting(context.Background(), "bms_communication_en", "off"))
	val, _ = client.GetRegister(register.AddrBMSCommunicationEn)
	assert.Equal(t, uint16(0), val)

	require.NoError(t, sys.WriteSetting(context.Background(), "bms_communication_en", "can"))
	val, _ = client.GetRegister(register.AddrBMSCommunicationEn)
	assert.Equal(t, uint16(2), val)
}

// TestRestoreUnitChargeSettingsSequencing verifies that restoring settings executes in safe
// sequence: charging voltages and equalizing enable are written first, BMS comm enable (2) is
// restored second (establishing CAN), and prior max charge current (E20A) is restored LAST.
// Unrelated optional current limits (e.g. PV/mains caps) are not touched.
func TestRestoreUnitChargeSettingsSequencing(t *testing.T) {
	var writeOrder []uint16

	client := newTestUnit(map[uint16]uint16{
		register.AddrSystemVoltage:         48,
		register.AddrBatteryType:           6,
		register.AddrLimitedChargeVoltage:  144, // 57.6V
		register.AddrEqualizingChargeVolt:  146, // 58.4V
		register.AddrBoostChargeVoltage:    146, // 58.4V
		register.AddrFloatChargeVoltage:    146, // 58.4V
		register.AddrStopChargeSOC:         90,
		register.AddrEqualizingChargeEn:    1,
		register.AddrMaxChargeCurrent:      1000, // 100A
		register.AddrBMSCommunicationEn:    2,    // CAN
		register.AddrMainsChargeCurrentLim: 0,
		register.AddrPVChargeCurrentLimit:  1000,
		register.AddrOverVoltageProtection: 150,
	})

	sys := NewSystem([]modbus.Client{client}, []string{"inv-0"})

	// Snapshot original profile
	orig, err := sys.ReadUnitChargeSettings(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, uint16(2), orig.BMSCommunicationEn)
	assert.InDelta(t, 100.0, orig.MaxChargeCurrent, 0.01)

	// Modify settings as if in manual conditioning (suspend CAN, CV stage 54.4V, 10A current)
	require.NoError(t, sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrBMSCommunicationEn, 0))
	require.NoError(t, sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrMaxChargeCurrent, 100))
	require.NoError(t, sys.WriteUnitRegisterVerified(context.Background(), 0, register.AddrFloatChargeVoltage, 136)) // 54.4V

	// Track write order during restore
	client.WriteHook = func(addr, val uint16) error {
		writeOrder = append(writeOrder, addr)
		return nil
	}

	// Restore original settings
	require.NoError(t, sys.RestoreUnitChargeSettings(context.Background(), 0, *orig))

	// Verify write ordering:
	// Voltages and equalization must come before BMS comm (CAN)
	// Max charge current (E20A) MUST be restored LAST!
	require.NotEmpty(t, writeOrder)
	assert.Equal(t, uint16(register.AddrMaxChargeCurrent), writeOrder[len(writeOrder)-1],
		"Max charge current (0xE20A) must be restored LAST after voltages, equalization, and CAN are verified")

	// BMS comm enable must precede max charge current
	bmsIdx := -1
	currentIdx := -1
	for i, addr := range writeOrder {
		if addr == register.AddrBMSCommunicationEn {
			bmsIdx = i
		}
		if addr == register.AddrMaxChargeCurrent {
			currentIdx = i
		}
	}
	require.True(t, bmsIdx >= 0, "BMS comm enable must be in write sequence")
	require.True(t, currentIdx >= 0, "Max charge current must be in write sequence")
	assert.Less(t, bmsIdx, currentIdx, "BMS comm enable must be restored before max charge current")

	// Verify unrelated current limits (AddrPVChargeCurrentLimit, AddrMainsChargeCurrentLim) were NOT written
	for _, addr := range writeOrder {
		assert.NotEqual(t, uint16(register.AddrPVChargeCurrentLimit), addr, "PV charge limit should not be written without need")
		assert.NotEqual(t, uint16(register.AddrMainsChargeCurrentLim), addr, "Mains charge limit should not be written without need")
	}

	// Verify all restored values in registers
	valFloat, _ := client.GetRegister(register.AddrFloatChargeVoltage)
	assert.Equal(t, uint16(146), valFloat, "float voltage restored to 58.4V (raw 146)")

	valCurr, _ := client.GetRegister(register.AddrMaxChargeCurrent)
	assert.Equal(t, uint16(1000), valCurr, "max charge current restored to 100A (raw 1000)")

	valBMS, _ := client.GetRegister(register.AddrBMSCommunicationEn)
	assert.Equal(t, uint16(2), valBMS, "BMS comm restored to CAN (2)")
}

// TestVoltageAndCurrentEncoding verifies the scaling math for 12V-base voltages and 0.1A currents.
func TestVoltageAndCurrentEncoding(t *testing.T) {
	sysV := 48.0

	// 54.4V -> raw 136
	assert.Equal(t, uint16(136), Encode12VBaseVoltage(54.4, sysV))
	assert.InDelta(t, 54.4, Decode12VBaseVoltage(136, sysV), 0.01)

	// 54.8V -> raw 137
	assert.Equal(t, uint16(137), Encode12VBaseVoltage(54.8, sysV))
	assert.InDelta(t, 54.8, Decode12VBaseVoltage(137, sysV), 0.01)

	// 55.2V -> raw 138
	assert.Equal(t, uint16(138), Encode12VBaseVoltage(55.2, sysV))
	assert.InDelta(t, 55.2, Decode12VBaseVoltage(138, sysV), 0.01)

	// 57.6V -> raw 144
	assert.Equal(t, uint16(144), Encode12VBaseVoltage(57.6, sysV))
	assert.InDelta(t, 57.6, Decode12VBaseVoltage(144, sysV), 0.01)

	// 58.4V -> raw 146
	assert.Equal(t, uint16(146), Encode12VBaseVoltage(58.4, sysV))
	assert.InDelta(t, 58.4, Decode12VBaseVoltage(146, sysV), 0.01)

	// 10A -> raw 100
	assert.Equal(t, uint16(100), EncodeCurrent(10.0))
	assert.InDelta(t, 10.0, DecodeCurrent(100), 0.01)

	// 100A -> raw 1000
	assert.Equal(t, uint16(1000), EncodeCurrent(100.0))
	assert.InDelta(t, 100.0, DecodeCurrent(1000), 0.01)
}
