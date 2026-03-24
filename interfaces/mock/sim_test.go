package mock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/register"
)

func TestSimDefaultRegisters(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())
	defer func() { _ = s.Close() }()

	vals, err := s.ReadRegisters(register.AddrBatterySOC, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(85), vals[0])

	vals, err = s.ReadRegisters(register.AddrSystemVoltage, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(48), vals[0])

	vals, err = s.ReadRegisters(register.AddrMachineState, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(5), vals[0], "should be in inverter mode")
}

func TestSimTick(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())
	defer func() { _ = s.Close() }()

	socBefore, _ := s.GetRegister(register.AddrBatterySOC)

	// Tick several times — SOC should change (PV charging during daytime)
	for i := 0; i < 100; i++ {
		s.Tick()
	}

	socAfter, _ := s.GetRegister(register.AddrBatterySOC)
	// SOC may or may not change depending on simulated time of day and load,
	// but registers should remain valid
	assert.LessOrEqual(t, socAfter, uint16(100))
	_ = socBefore // used for documentation
}

func TestSimSetSOC(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())

	s.SetSOC(50)
	vals, err := s.ReadRegisters(register.AddrBatterySOC, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(50), vals[0])

	// Battery voltage should correspond to 50% SOC
	vals, err = s.ReadRegisters(register.AddrBatteryVoltage, 1)
	require.NoError(t, err)
	battV := float64(vals[0]) * 0.1
	assert.InDelta(t, 49.2, battV, 0.2, "~49.2V at 50% SOC")
}

func TestSimSetSOC_Clamped(t *testing.T) {
	s := NewSim()
	s.SetSOC(150) // should clamp to 100
	// Need to connect first
	require.NoError(t, s.Connect())
	vals, err := s.ReadRegisters(register.AddrBatterySOC, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(100), vals[0])
}

func TestSimTriggerFault(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())

	s.TriggerFault(14) // OverloadInverter
	s.Tick()

	vals, err := s.ReadRegisters(register.AddrFaultCode1, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(14), vals[0])

	vals, err = s.ReadRegisters(register.AddrMachineState, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(10), vals[0], "machine state = fault")
}

func TestSimClearFaults(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())

	s.TriggerFault(14)
	s.Tick()
	s.ClearFaults()

	vals, err := s.ReadRegisters(register.AddrFaultCode1, 1)
	require.NoError(t, err)
	assert.Zero(t, vals[0])

	vals, err = s.ReadRegisters(register.AddrMachineState, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(5), vals[0], "back to inverter mode")
}

func TestSimSetLoad(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())

	s.SetLoad(2000) // heavy load
	s.Tick()

	vals, err := s.ReadRegisters(register.AddrLoadPowerL1, 1)
	require.NoError(t, err)
	assert.Greater(t, vals[0], uint16(0), "should have load power")
}

func TestSimStartStop(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())

	s.Start(10 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	// Verify registers are still readable
	_, err := s.ReadRegisters(register.AddrBatterySOC, 1)
	assert.NoError(t, err)
}

func TestSimProductInfo(t *testing.T) {
	s := NewSim()
	require.NoError(t, s.Connect())

	// Read model ASCII
	vals, err := s.ReadRegisters(register.AddrProductModel, 8)
	require.NoError(t, err)

	model := register.FormatValue(register.Register{Type: register.ASCII, Count: 8}, vals, nil)
	assert.Contains(t, model, "ASP48100U200")
}

func TestSimPVCurve(t *testing.T) {
	assert.Zero(t, pvCurve(3), "no PV at 3am")
	assert.Zero(t, pvCurve(21), "no PV at 9pm")
	assert.Greater(t, pvCurve(13), 3000.0, "peak PV around noon")
	assert.Greater(t, pvCurve(10), pvCurve(7), "more PV at 10am than 7am")
}
