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
