package serve

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// Subscriber receives snapshots from the hub.
type Subscriber struct {
	C    chan *inverter.Snapshot
	done chan struct{}
}

// writeRequest is sent to the hub's write channel to serialize register writes with polling.
type writeRequest struct {
	field  string
	value  string
	result chan error
}

// faultsRequest is sent to the hub's run loop to serialize fault-history reads
// with polling, so they never issue overlapping MODBUS requests to the dongle.
type faultsRequest struct {
	result chan faultsResult
}

type faultsResult struct {
	faults []register.FaultRecord
	err    error
}

// Hub polls the inverter system and fans out snapshots to subscribers.
// All MODBUS communication is serialized through the hub's run loop.
type Hub struct {
	system          *inverter.System
	pollInterval    time.Duration
	settingsRefresh time.Duration

	mu          sync.RWMutex
	latest      *inverter.Snapshot
	settings    *inverter.Settings
	subscribers map[*Subscriber]struct{}

	writeCh  chan writeRequest
	faultsCh chan faultsRequest

	// Virtual switch state: last non-zero AC charge current limit (0xE205) so the
	// charge_from_mains toggle can restore it when switched back ON.
	prevMainsChargeCurrent     string
	prevMainsChargeCurrentInit bool
}

// defaultMainsChargeCurrent is the V2.04 factory default for 0xE205 (60 A), used
// when charge_from_mains is switched ON without a previously-seen non-zero limit.
const defaultMainsChargeCurrent = "60"

// NewHub creates a hub that polls the system at the given interval.
// The system may be nil initially — call SetSystem before Run.
func NewHub(system *inverter.System, pollInterval, settingsRefresh time.Duration) *Hub {
	return &Hub{
		system:          system,
		pollInterval:    pollInterval,
		settingsRefresh: settingsRefresh,
		subscribers:     make(map[*Subscriber]struct{}),
		writeCh:         make(chan writeRequest, 8),
		faultsCh:        make(chan faultsRequest, 4),
	}
}

// SetSystem sets the inverter system after deferred initialization.
func (h *Hub) SetSystem(system *inverter.System) {
	h.mu.Lock()
	h.system = system
	h.mu.Unlock()
}

// Subscribe returns a new subscriber that receives snapshots.
func (h *Hub) Subscribe() *Subscriber {
	sub := &Subscriber{
		C:    make(chan *inverter.Snapshot, 2),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	h.subscribers[sub] = struct{}{}
	h.mu.Unlock()
	return sub
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *Hub) Unsubscribe(sub *Subscriber) {
	h.mu.Lock()
	if _, ok := h.subscribers[sub]; ok {
		delete(h.subscribers, sub)
		close(sub.done)
	}
	h.mu.Unlock()
}

// Latest returns the most recent snapshot, or nil if none yet.
func (h *Hub) Latest() *inverter.Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latest
}

// Settings returns the cached settings, or nil if not yet read.
func (h *Hub) Settings() *inverter.Settings {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.settings
}

// SaveMainsChargeCurrent remembers the current AC charge current limit for the
// charge_from_mains toggle. A zero value is not saved (restoring zero would be a
// no-op OFF), so the prior non-zero limit survives an OFF→ON cycle.
func (h *Hub) SaveMainsChargeCurrent(current float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current > 0 {
		h.prevMainsChargeCurrent = fmt.Sprintf("%.0f", current)
	}
	h.prevMainsChargeCurrentInit = true
}

// RestoreMainsChargeCurrent returns the saved AC charge current limit, falling
// back to the factory default if no non-zero value has been seen.
func (h *Hub) RestoreMainsChargeCurrent() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.prevMainsChargeCurrent == "" {
		return defaultMainsChargeCurrent
	}
	return h.prevMainsChargeCurrent
}

// InitMainsChargeCurrent seeds the saved limit from settings on first call, so a
// restart with grid charging already enabled remembers the configured limit.
func (h *Hub) InitMainsChargeCurrent(settings *inverter.Settings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.prevMainsChargeCurrentInit || settings == nil {
		return
	}
	h.prevMainsChargeCurrentInit = true
	if settings.Inverter.MainsChargeCurrentLim > 0 {
		h.prevMainsChargeCurrent = fmt.Sprintf("%.0f", settings.Inverter.MainsChargeCurrentLim)
	}
}

// WriteSetting sends a write request to the hub's run loop, ensuring it's
// serialized with polling. Blocks until the write completes or ctx expires.
func (h *Hub) WriteSetting(ctx context.Context, field, value string) error {
	req := writeRequest{
		field:  field,
		value:  value,
		result: make(chan error, 1),
	}
	select {
	case h.writeCh <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-req.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReadFaults reads fault history through the hub's run loop, serialized with
// polling so it never issues overlapping MODBUS requests to the dongle.
// Blocks until the read completes or ctx expires.
func (h *Hub) ReadFaults(ctx context.Context) ([]register.FaultRecord, error) {
	req := faultsRequest{result: make(chan faultsResult, 1)}
	select {
	case h.faultsCh <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case res := <-req.result:
		return res.faults, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	pollTicker := time.NewTicker(h.pollInterval)
	defer pollTicker.Stop()

	settingsTicker := time.NewTicker(h.settingsRefresh)
	defer settingsTicker.Stop()

	// Read settings and take initial snapshot
	h.refreshSettings(ctx)
	h.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			h.poll(ctx)
		case <-settingsTicker.C:
			h.refreshSettings(ctx)
		case req := <-h.writeCh:
			h.handleWrite(ctx, req)
		case req := <-h.faultsCh:
			h.handleReadFaults(ctx, req)
		}
	}
}

func (h *Hub) handleReadFaults(ctx context.Context, req faultsRequest) {
	h.mu.RLock()
	sys := h.system
	h.mu.RUnlock()

	if sys == nil {
		req.result <- faultsResult{err: fmt.Errorf("system not ready")}
		return
	}

	faults, err := sys.ReadFaults(ctx)
	req.result <- faultsResult{faults: faults, err: err}
}

func (h *Hub) handleWrite(ctx context.Context, req writeRequest) {
	h.mu.RLock()
	sys := h.system
	h.mu.RUnlock()

	if sys == nil {
		req.result <- fmt.Errorf("system not ready")
		return
	}

	slog.Info("writing setting", "field", req.field, "value", req.value)
	err := sys.WriteSetting(ctx, req.field, req.value)
	if err != nil {
		slog.Error("setting write failed", "field", req.field, "error", err)
	} else {
		slog.Info("setting written", "field", req.field, "value", req.value)
		// Refresh settings after successful write
		h.refreshSettings(ctx)
	}
	req.result <- err
}

func (h *Hub) refreshSettings(ctx context.Context) {
	h.mu.RLock()
	sys := h.system
	latest := h.latest
	h.mu.RUnlock()
	if sys == nil {
		return
	}

	// Skip settings refresh when all units are stale (disconnected).
	// The poll loop will recover connections and trigger a refresh then.
	if latest != nil && len(latest.Units) > 0 {
		allStale := true
		for _, u := range latest.Units {
			if !u.Stale {
				allStale = false
				break
			}
		}
		if allStale {
			return
		}
	}

	if err := sys.RefreshSettings(ctx); err != nil {
		slog.Warn("settings refresh failed", "error", err)
	}
	settings, err := sys.ReadSettings(ctx)
	if err != nil {
		slog.Warn("settings read failed", "error", err)
		return
	}
	slog.Debug("settings refreshed")
	h.mu.Lock()
	h.settings = settings
	h.mu.Unlock()
}

func (h *Hub) poll(ctx context.Context) {
	h.mu.RLock()
	sys := h.system
	h.mu.RUnlock()
	if sys == nil {
		return
	}

	snap := sys.Snapshot(ctx)
	staleCount := 0
	for _, u := range snap.Units {
		if u.Stale {
			staleCount++
		}
	}
	slog.Debug("poll complete",
		"battery_soc", snap.Battery.SOC,
		"pv_total", snap.PV.TotalPower,
		"load_total", snap.Load.TotalPower,
		"stale_units", staleCount,
	)

	h.mu.Lock()
	h.latest = snap
	subs := make([]*Subscriber, 0, len(h.subscribers))
	for sub := range h.subscribers {
		subs = append(subs, sub)
	}
	h.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub.C <- snap:
		default:
			select {
			case <-sub.C:
			default:
			}
			select {
			case sub.C <- snap:
			default:
			}
		}
	}
}
