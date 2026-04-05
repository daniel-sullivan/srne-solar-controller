package serve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/mock"
	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
)

func newTestSystem(t *testing.T) *inverter.System {
	t.Helper()
	sim := mock.NewSim()
	sim.SetSOC(85)
	sim.SetPV(600, 440)
	sim.SetLoad(500)
	require.NoError(t, sim.Connect())
	t.Cleanup(func() { _ = sim.Close() })

	sys := inverter.NewSystem([]modbus.Client{sim})
	require.NoError(t, sys.Init(context.Background()))
	return sys
}

func TestHubPollAndLatest(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Wait for at least one poll
	time.Sleep(100 * time.Millisecond)

	snap := hub.Latest()
	require.NotNil(t, snap)
	assert.Equal(t, float64(85), snap.Battery.SOC)
}

func TestHubSubscribe(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, time.Hour)

	sub := hub.Subscribe()
	defer hub.Unsubscribe(sub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	select {
	case snap := <-sub.C:
		require.NotNil(t, snap)
		assert.Equal(t, float64(85), snap.Battery.SOC)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for snapshot")
	}
}

func TestHubMultipleSubscribers(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, time.Hour)

	sub1 := hub.Subscribe()
	sub2 := hub.Subscribe()
	defer hub.Unsubscribe(sub1)
	defer hub.Unsubscribe(sub2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Both subscribers should get data
	for _, sub := range []*Subscriber{sub1, sub2} {
		select {
		case snap := <-sub.C:
			require.NotNil(t, snap)
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestHubUnsubscribe(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, time.Hour, time.Hour) // long interval — manual poll

	sub := hub.Subscribe()
	hub.Unsubscribe(sub)

	// Channel done should be closed
	select {
	case <-sub.done:
	default:
		t.Fatal("expected done to be closed")
	}
}
