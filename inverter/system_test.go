package inverter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

// newSim creates a connected Sim with deterministic state via poke methods.
func newSim() *mock.Sim {
	s := mock.NewSim()
	_ = s.Connect()
	return s
}

func TestSingleUnitSnapshot(t *testing.T) {
	sim := newSim()
	sim.SetSOC(85)
	sim.SetPV(600, 440)
	sim.SetLoad(760)

	session := modbus.NewSession(sim)
	snap, err := readUnitSnapshot(session)
	require.NoError(t, err)
	require.Empty(t, snap.Errors)

	// Battery
	assert.Equal(t, float64(85), snap.Battery.SOC)
	assert.InDelta(t, 52.84, snap.Battery.Voltage, 0.2) // LiFePO4 curve at 85%
	assert.Equal(t, "Quick Charge", snap.Battery.ChargeStatusName)

	// PV — set to exactly 600W and 440W
	assert.Equal(t, float64(600), snap.PV.PV1Power)
	assert.Equal(t, float64(440), snap.PV.PV2Power)
	assert.Equal(t, float64(1040), snap.PV.TotalPower)

	// Inverter
	assert.Equal(t, "Inverter Mode", snap.Inverter.MachineStateName)
	assert.InDelta(t, 60.0, snap.Inverter.Frequency, 0.01)

	// Load — 760W split evenly across L1/L2
	assert.Equal(t, float64(760), snap.Load.TotalPower)

	// Stats
	assert.InDelta(t, 15.0, snap.Stats.PVGenerationToday, 0.1)
}

func TestParallelAggregation(t *testing.T) {
	sim1 := newSim()
	sim1.SetSOC(85)
	sim1.SetPV(600, 440)
	sim1.SetLoad(760)
	sim1.SetParallelMode(1)

	sim2 := newSim()
	sim2.SetSOC(85)
	sim2.SetPV(800, 500)
	sim2.SetLoad(760)
	sim2.SetParallelMode(1)

	s1 := modbus.NewSession(sim1)
	s2 := modbus.NewSession(sim2)
	snap1, _ := readUnitSnapshot(s1)
	snap2, _ := readUnitSnapshot(s2)

	result := aggregate([]UnitSnapshot{snap1, snap2})

	// Battery SOC: averaged (both 85)
	assert.InDelta(t, 85.0, result.Battery.SOC, 0.1)

	// Battery current: summed
	assert.InDelta(t, snap1.Battery.Current+snap2.Battery.Current, result.Battery.Current, 0.1)

	// PV power: summed (600+440) + (800+500) = 2340
	assert.Equal(t, float64(2340), result.PV.TotalPower)

	// Load power: summed (760 + 760 = 1520)
	assert.Equal(t, float64(1520), result.Load.TotalPower)

	// Stats: summed (15.0 + 15.0 = 30.0 kWh)
	assert.InDelta(t, 30.0, result.Stats.PVGenerationToday, 0.1)
}

func TestAggregationTempUsesMax(t *testing.T) {
	// Light load on sim1, heavy load on sim2 → sim2 runs hotter
	sim1 := newSim()
	sim1.SetLoad(200)
	sim1.SetPV(600, 400)

	sim2 := newSim()
	sim2.SetLoad(4000) // heavy load → hotter heatsinks
	sim2.SetPV(600, 400)

	s1 := modbus.NewSession(sim1)
	s2 := modbus.NewSession(sim2)
	snap1, _ := readUnitSnapshot(s1)
	snap2, _ := readUnitSnapshot(s2)

	result := aggregate([]UnitSnapshot{snap1, snap2})

	// Heatsink temps should be the max (from the heavier-loaded unit)
	assert.Greater(t, snap2.Inverter.HeatsinkTempB, snap1.Inverter.HeatsinkTempB)
	assert.Equal(t, snap2.Inverter.HeatsinkTempB, result.Inverter.HeatsinkTempB)
}

func TestStaleFallback(t *testing.T) {
	sim := newSim()
	sim.SetSOC(85)
	sim.SetPV(600, 440)
	sim.SetLoad(500)

	session := modbus.NewSession(sim)
	u := &unit{
		info:    UnitInfo{Host: "test"},
		session: session,
	}

	// Take a good snapshot
	snap, _ := readUnitSnapshot(session)
	snap.Host = "test"
	u.lastSnapshot = snap

	// Make reads fail
	sim.ReadHook = func(_, _ uint16) error {
		return &modbus.ModbusError{FunctionCode: 0x03, ExceptionCode: 0x02}
	}

	sys := &System{units: []*unit{u}}

	// First failure: fallback data, not yet stale
	result := sys.Snapshot(context.Background())
	assert.False(t, result.Units[0].Stale)
	assert.Equal(t, float64(85), result.Battery.SOC)

	// Accumulate failures past threshold
	for i := 0; i < MaxStaleSnapshots; i++ {
		sys.Snapshot(context.Background())
	}
	result = sys.Snapshot(context.Background())
	assert.True(t, result.Units[0].Stale)
}

func TestNewSystemWithClients(t *testing.T) {
	sim1 := newSim()
	sim1.SetPV(600, 440)
	sim1.SetParallelMode(1)

	sim2 := newSim()
	sim2.SetPV(900, 300)
	sim2.SetParallelMode(1)

	sys := NewSystem([]modbus.Client{sim1, sim2})
	require.NoError(t, sys.Init(context.Background()))
	assert.True(t, sys.IsParallel())

	snap := sys.Snapshot(context.Background())
	require.Len(t, snap.Units, 2)

	// PV1 power summed: 600 + 900 = 1500
	assert.Equal(t, float64(1500), snap.PV.PV1Power)
}

func TestPVChange(t *testing.T) {
	sim := newSim()
	sim.SetPV(1000, 500)
	sim.SetLoad(400)

	session := modbus.NewSession(sim)
	snap1, _ := readUnitSnapshot(session)
	assert.Equal(t, float64(1500), snap1.PV.TotalPower)

	// "Cloud passes over" — PV drops
	sim.SetPV(200, 100)

	session2 := modbus.NewSession(sim) // fresh session for uncached read
	snap2, _ := readUnitSnapshot(session2)
	assert.Equal(t, float64(300), snap2.PV.TotalPower)

	// Battery current should reflect the reduced PV
	assert.Less(t, snap2.Battery.Current, snap1.Battery.Current)
}

func TestGridVoltageSwitch(t *testing.T) {
	sim := newSim()
	sim.SetPV(500, 500)
	sim.SetLoad(1000)

	session := modbus.NewSession(sim)
	snap, _ := readUnitSnapshot(session)
	assert.Equal(t, "Inverter Mode", snap.Inverter.MachineStateName)
	assert.Equal(t, float64(0), snap.Grid.L1.GridVoltage)

	// Grid comes online
	sim.SetGridVoltage(120.0)

	session2 := modbus.NewSession(sim)
	snap2, _ := readUnitSnapshot(session2)
	assert.Equal(t, "Mains Mode", snap2.Inverter.MachineStateName)
	assert.InDelta(t, 120.0, snap2.Grid.L1.GridVoltage, 0.1)
}
