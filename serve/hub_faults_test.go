package serve

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHubReadFaultsSerialized verifies fault reads are serviced through the run
// loop while the hub is actively polling — the path that previously read the
// shared per-unit session concurrently with the poll/settings loop and raced.
func TestHubReadFaultsSerialized(t *testing.T) {
	sys := newTestSystem(t)
	_ = sys.Units() // touch before the hub starts (parity_test.go pattern)
	hub := NewHub(sys, 20*time.Millisecond, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(80 * time.Millisecond) // let the poll/settings loop spin

	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	// The mock does not implement the fault-history register range, so this
	// returns an error rather than records — but it must return promptly through
	// the run loop, not time out.
	_, err := hub.ReadFaults(rctx)
	require.Error(t, err)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
}

// TestHubReadFaultsSystemNotReady covers the nil-system branch in handleReadFaults.
func TestHubReadFaultsSystemNotReady(t *testing.T) {
	hub := NewHub(nil, time.Hour, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)

	rctx, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rcancel()
	_, err := hub.ReadFaults(rctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system not ready")
}

// TestHubReadFaultsContextCancelled covers the ctx-cancel path while awaiting the
// result: the hub is not running, so the buffered request is never serviced.
func TestHubReadFaultsContextCancelled(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, time.Hour, time.Hour) // not started

	rctx, rcancel := context.WithCancel(context.Background())
	rcancel() // already cancelled
	_, err := hub.ReadFaults(rctx)
	require.ErrorIs(t, err, context.Canceled)
}
