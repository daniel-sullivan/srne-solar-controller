package serve

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

func startConditioningHub(t *testing.T) (*Hub, context.CancelFunc) {
	t.Helper()
	hub := NewHub(newTestSystem(t), time.Hour, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	// Run performs its initial poll synchronously before accepting requests.
	deadline := time.Now().Add(time.Second)
	for hub.Latest() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hub.Latest() == nil {
		t.Fatal("hub did not complete initial poll")
	}
	t.Cleanup(cancel)
	return hub, cancel
}

func TestConditioningReservationRejectsQueuedAndNewWrites(t *testing.T) {
	hub, _ := startConditioningHub(t)
	release := make(chan struct{})
	started := make(chan struct{})
	requireNoError(t, hub.ReserveConditioning())

	go func() {
		_ = hub.WithConditioning(context.Background(), func(context.Context, *inverter.System) error {
			close(started)
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("conditioning callback did not start")
	}

	writeDone := make(chan error, 1)
	go func() { writeDone <- hub.WriteSetting(context.Background(), "mains_charge_current_lim", "1") }()
	select {
	case err := <-writeDone:
		t.Fatalf("queued write completed before callback: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-writeDone:
		if !errors.Is(err, ErrConditioningReserved) {
			t.Fatalf("queued write error = %v, want ErrConditioningReserved", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued write did not complete")
	}
	// A second ordinary write is rejected at the entrypoint while reserved.
	if err := hub.WriteSetting(context.Background(), "mains_charge_current_lim", "1"); !errors.Is(err, ErrConditioningReserved) {
		t.Fatalf("new write error = %v, want ErrConditioningReserved", err)
	}
}

func TestConditioningCallbackRunsWhileReservedAndReleaseAllowsWrites(t *testing.T) {
	hub, _ := startConditioningHub(t)
	requireNoError(t, hub.ReserveConditioning())
	var called atomic.Bool
	err := hub.WithConditioning(context.Background(), func(_ context.Context, sys *inverter.System) error {
		called.Store(sys != nil)
		return nil
	})
	if err != nil || !called.Load() {
		t.Fatalf("callback result = %v, called = %v", err, called.Load())
	}
	hub.ReleaseConditioning()
	if err := hub.WriteSetting(context.Background(), "mains_charge_current_lim", "1"); err != nil {
		t.Fatalf("write after release: %v", err)
	}
}

func TestConditioningCanceledCallbackIsDiscarded(t *testing.T) {
	hub, _ := startConditioningHub(t)
	requireNoError(t, hub.ReserveConditioning())
	block := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_ = hub.WithConditioning(context.Background(), func(context.Context, *inverter.System) error {
			close(started)
			<-block
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("blocking callback did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var late atomic.Bool
	result := make(chan error, 1)
	go func() {
		result <- hub.WithConditioning(ctx, func(context.Context, *inverter.System) error {
			late.Store(true)
			return nil
		})
	}()
	cancel()
	close(block)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled callback error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled callback did not complete")
	}
	if late.Load() {
		t.Fatal("canceled callback executed late")
	}
}

func TestConditioningCancellationWaitsForStartedCallback(t *testing.T) {
	hub, _ := startConditioningHub(t)
	requireNoError(t, hub.ReserveConditioning())
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- hub.WithConditioning(ctx, func(context.Context, *inverter.System) error {
			close(started)
			<-release
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("WithConditioning returned while callback was still active")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("completed callback returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WithConditioning did not return after callback completed")
	}
}

func TestConditioningNilSystemAndRunTimeout(t *testing.T) {
	hub := NewHub(nil, time.Hour, time.Hour)
	if err := hub.ReserveConditioning(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := hub.WithConditioning(ctx, func(context.Context, *inverter.System) error { t.Fatal("callback ran"); return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("without Run error = %v, want deadline", err)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
