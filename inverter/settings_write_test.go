package inverter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

func TestWriteSettingEncodesSignedTemperatureLimits(t *testing.T) {
	client := mock.NewInverter(map[uint16]uint16{
		register.AddrSystemVoltage:    48,
		register.AddrChargeMinTemp:    0,
		register.AddrDischargeMinTemp: 0,
	})
	require.NoError(t, client.Connect())

	sys := NewSystem([]modbus.Client{client}, nil)

	require.NoError(t, sys.WriteSetting(context.Background(), "charge_min_temp", "-5"))
	require.NoError(t, sys.WriteSetting(context.Background(), "discharge_min_temp", "-10"))

	chargeMinTemp, ok := client.GetRegister(register.AddrChargeMinTemp)
	require.True(t, ok)
	assert.Equal(t, uint16(0xFFFB), chargeMinTemp)

	dischargeMinTemp, ok := client.GetRegister(register.AddrDischargeMinTemp)
	require.True(t, ok)
	assert.Equal(t, uint16(0xFFF6), dischargeMinTemp)
}

// rejectChargerPriority simulates a parallel unit that refuses writes to the
// master-owned charger-priority register (0xE20F) while accepting others.
func rejectChargerPriority(addr, _ uint16) error {
	if addr == register.AddrChargerPriority {
		return &modbus.ModbusError{FunctionCode: modbus.FuncWriteSingleRegister, ExceptionCode: 0x04}
	}
	return nil
}

func TestWriteChargerPriorityWritesMasterOnly(t *testing.T) {
	master := mock.NewInverter(map[uint16]uint16{
		register.AddrSystemVoltage:   48,
		register.AddrChargerPriority: 3, // PV Only
	})
	require.NoError(t, master.Connect())

	slave := mock.NewInverter(map[uint16]uint16{
		register.AddrSystemVoltage:   48,
		register.AddrChargerPriority: 3,
	})
	require.NoError(t, slave.Connect())
	slave.WriteHook = rejectChargerPriority // the slave would abort an all-units write

	sys := NewSystem([]modbus.Client{master, slave}, nil)

	// AC Priority. Must succeed despite the slave rejecting the register.
	require.NoError(t, sys.WriteSetting(context.Background(), "charger_priority", "1"))

	got, ok := master.GetRegister(register.AddrChargerPriority)
	require.True(t, ok)
	assert.Equal(t, uint16(1), got, "master charger priority should be updated")

	slaveVal, ok := slave.GetRegister(register.AddrChargerPriority)
	require.True(t, ok)
	assert.Equal(t, uint16(3), slaveVal, "slave charger priority should be left untouched")
}

func TestWriteChargerPriorityMasterRejectionReturnsError(t *testing.T) {
	master := mock.NewInverter(map[uint16]uint16{
		register.AddrSystemVoltage:   48,
		register.AddrChargerPriority: 3,
	})
	require.NoError(t, master.Connect())
	master.WriteHook = rejectChargerPriority // the inverter itself refuses the write

	sys := NewSystem([]modbus.Client{master}, nil)

	err := sys.WriteSetting(context.Background(), "charger_priority", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0xE20F")
}
