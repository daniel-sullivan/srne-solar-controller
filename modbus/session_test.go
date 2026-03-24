package modbus_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

func testInverter() *mock.Inverter {
	return mock.NewInverter(map[uint16]uint16{
		register.AddrBatterySOC:            85,
		register.AddrBatteryVoltage:        532,
		register.AddrBatteryCurrent:        100,
		register.AddrTemps:                 0x1E19,
		register.AddrSystemVoltage:         48,
		register.AddrOutputPriority:        1,
		register.AddrMainsChargeCurrentLim: 0,
	})
}

func TestSessionReadRegisters(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	vals, err := s.ReadRegisters(register.AddrBatterySOC, 3)
	require.NoError(t, err)
	assert.Len(t, vals, 3)
	assert.Equal(t, uint16(85), vals[0], "SOC")
	assert.Equal(t, uint16(532), vals[1], "voltage")
}

func TestSessionLookupCaches(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	v1, err := s.Lookup(register.AddrBatterySOC)
	require.NoError(t, err)
	assert.Equal(t, uint16(85), v1)

	reads := inv.ReadCount
	v2, err := s.Lookup(register.AddrBatterySOC)
	require.NoError(t, err)
	assert.Equal(t, uint16(85), v2)
	assert.Equal(t, reads, inv.ReadCount, "should use cache")
}

func TestSessionStorePreloadsCache(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	s.Store(register.AddrBatterySOC, []uint16{85, 532, 100})

	v, err := s.Lookup(register.AddrBatterySOC)
	require.NoError(t, err)
	assert.Equal(t, uint16(85), v)
	assert.Zero(t, inv.ReadCount, "should not hit device")
}

func TestSessionWriteInvalidatesCache(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	_, _ = s.Lookup(register.AddrBatterySOC)
	require.Equal(t, 1, inv.ReadCount)

	require.NoError(t, s.WriteSingleRegister(register.AddrMainsChargeCurrentLim, 42))

	_, _ = s.Lookup(register.AddrBatterySOC)
	assert.Equal(t, 2, inv.ReadCount, "cache should be invalidated after write")
}

func TestSessionWriteFailurePreservesCache(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	_, _ = s.Lookup(register.AddrBatterySOC)
	reads := inv.ReadCount

	err := s.WriteSingleRegister(0xFFFF, 1)
	require.Error(t, err, "write to non-existent register")

	_, _ = s.Lookup(register.AddrBatterySOC)
	assert.Equal(t, reads, inv.ReadCount, "cache preserved on write failure")
}

func TestSessionWriteMultipleInvalidatesCache(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	_, _ = s.Lookup(register.AddrOutputPriority)
	reads := inv.ReadCount

	require.NoError(t, s.WriteMultipleRegisters(register.AddrOutputPriority, []uint16{2, 1}))

	_, _ = s.Lookup(register.AddrOutputPriority)
	assert.Greater(t, inv.ReadCount, reads, "cache invalidated after write")
}

func TestSessionReadIllegalAddress(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	_, err := s.ReadRegisters(0xFFFF, 1)
	require.Error(t, err)

	var modbusErr *modbus.ModbusError
	require.ErrorAs(t, err, &modbusErr)
	assert.Equal(t, byte(0x02), modbusErr.ExceptionCode)
}

func TestSessionContextCancellation(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())
	s := modbus.NewSession(inv)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Ctx = ctx

	// May succeed or fail depending on timing — just verify no panic
	_, _ = s.ReadRegisters(register.AddrBatterySOC, 1)
}

func TestMockInverterNotConnected(t *testing.T) {
	inv := testInverter()
	_, err := inv.ReadRegisters(register.AddrBatterySOC, 1)
	assert.Error(t, err)
}

func TestMockInverterReadHook(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())

	hookErr := fmt.Errorf("simulated timeout")
	inv.ReadHook = func(start, count uint16) error {
		if start == register.AddrBatterySOC {
			return hookErr
		}
		return nil
	}

	_, err := inv.ReadRegisters(register.AddrBatterySOC, 1)
	assert.ErrorIs(t, err, hookErr)

	vals, err := inv.ReadRegisters(register.AddrSystemVoltage, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(48), vals[0])
}

func TestMockInverterWriteMultipleAtomicity(t *testing.T) {
	inv := testInverter()
	require.NoError(t, inv.Connect())

	err := inv.WriteMultipleRegisters(register.AddrOutputPriority, []uint16{99, 99, 99})
	require.Error(t, err, "partially invalid range")

	v, _ := inv.GetRegister(register.AddrOutputPriority)
	assert.Equal(t, uint16(1), v, "original value preserved")
}
