package interpack

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestPollFrameMatchesKnownWireRequest(t *testing.T) {
	request := PollFrame(1)
	// The live bus uses this 10-byte CRC16/MODBUS request format.
	if got := hex.EncodeToString(request[:8]); got != "0145000000540000" {
		t.Fatalf("poll prefix = %s", got)
	}
	if got := CRC16(request[:8]); got != uint16(request[8])|uint16(request[9])<<8 {
		t.Fatalf("poll CRC = %04x, wire CRC = %02x%02x", got, request[9], request[8])
	}
}

func TestStoreKeepsMissingPacksVisibleAndAdmitsNewOnes(t *testing.T) {
	var store Store
	now := time.Now()
	first := Frame{Address: 2}
	store.Record(first, now)
	firstSnapshot := store.Snapshot(now.Add(time.Second), 30*time.Second)
	if len(firstSnapshot) != 1 || !firstSnapshot[0].Fresh || !firstSnapshot[0].Confirmed {
		t.Fatalf("first report = %+v", firstSnapshot)
	}
	if firstSnapshot[0].IdentityUncertain || firstSnapshot[0].Ambiguous() {
		t.Fatalf("address 2 should not have identity uncertainty")
	}
	if !firstSnapshot[0].Active() {
		t.Fatalf("address 2 should be active")
	}

	store.Record(Frame{Address: 3}, now.Add(35*time.Second))
	packs := store.Snapshot(now.Add(36*time.Second), 30*time.Second)
	if len(packs) != 2 || packs[0].Frame.Address != 2 || packs[0].Fresh || packs[1].Frame.Address != 3 || !packs[1].Fresh {
		t.Fatalf("dynamic/stale reports = %+v", packs)
	}
	if packs[0].Active() {
		t.Fatalf("stale address 2 must not report active")
	}
	if !packs[1].Active() {
		t.Fatalf("fresh confirmed address 3 must report active")
	}
}

func TestAddressZeroRequiresConsistentReports(t *testing.T) {
	var store Store
	now := time.Now()
	first := Frame{Address: 0, CycleCount: 197, FullCapacityCentiAh: 10000}
	store.Record(first, now)
	snap1 := store.Snapshot(now, time.Minute)
	if snap1[0].Confirmed {
		t.Fatal("one address-zero reply should not establish a stable responder")
	}
	if !snap1[0].IdentityUncertain || !snap1[0].Ambiguous() {
		t.Fatal("address 0 must expose identity uncertainty")
	}

	store.Record(first, now.Add(time.Second))
	snap2 := store.Snapshot(now.Add(time.Second), time.Minute)
	if !snap2[0].Confirmed {
		t.Fatal("two consistent address-zero replies should establish a responder")
	}
	if !snap2[0].IdentityUncertain || !snap2[0].Ambiguous() {
		t.Fatal("address 0 must still expose identity uncertainty even when confirmed")
	}

	second := first
	second.CycleCount = 203
	store.Record(second, now.Add(2*time.Second))
	if store.Snapshot(now.Add(2*time.Second), time.Minute)[0].Confirmed {
		t.Fatal("a changed address-zero identity should reset confirmation")
	}
}

func TestAddressZeroRejectsStaleConsecutiveGap(t *testing.T) {
	var store Store
	now := time.Now()
	first := Frame{Address: 0, CycleCount: 197, FullCapacityCentiAh: 10000}
	store.Record(first, now)

	// Second sample arrives after gap exceeding maxConsecutiveGap (2 minutes)
	store.Record(first, now.Add(3*time.Minute))
	snap := store.Snapshot(now.Add(3*time.Minute), 5*time.Minute)
	if snap[0].Confirmed {
		t.Fatal("address zero reports separated by a large gap must reset confirmation")
	}
}

type partialWriter struct {
	writes [][]byte
	limit  int
	err    error
}

func (p *partialWriter) Write(b []byte) (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	n := len(b)
	if n > p.limit {
		n = p.limit
	}
	p.writes = append(p.writes, append([]byte(nil), b[:n]...))
	return n, nil
}

func TestWriteFull(t *testing.T) {
	pw := &partialWriter{limit: 3}
	data := []byte("0123456789")
	if err := writeFull(pw, data); err != nil {
		t.Fatalf("writeFull failed: %v", err)
	}
	if len(pw.writes) != 4 { // 3 + 3 + 3 + 1
		t.Fatalf("expected 4 partial writes, got %d", len(pw.writes))
	}

	// Test error propagation
	expectedErr := errors.New("write failure")
	errWriter := &partialWriter{err: expectedErr}
	if err := writeFull(errWriter, data); !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}

	// Test zero bytes written
	zeroWriter := &partialWriter{limit: 0}
	if err := writeFull(zeroWriter, data); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUnexpectedEOF on 0-byte write, got %v", err)
	}
}

type mockPort struct {
	mu          sync.Mutex
	cond        *sync.Cond
	rxBuf       []byte
	txBuf       []byte
	closed      bool
	readTimeout time.Duration
}

func newMockPort() *mockPort {
	m := &mockPort{}
	m.cond = sync.NewCond(&m.mu)
	return m
}

func (m *mockPort) SetReadTimeout(d time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readTimeout = d
	return nil
}

func (m *mockPort) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	m.txBuf = append(m.txBuf, p...)
	m.cond.Broadcast()
	return len(p), nil
}

func (m *mockPort) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.EOF
	}
	if len(m.rxBuf) > 0 {
		n := copy(p, m.rxBuf)
		m.rxBuf = m.rxBuf[n:]
		return n, nil
	}
	timeout := m.readTimeout
	if timeout <= 0 {
		timeout = 10 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for len(m.rxBuf) == 0 && !m.closed && time.Now().Before(deadline) {
		m.mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		m.mu.Lock()
	}
	if m.closed {
		return 0, io.EOF
	}
	if len(m.rxBuf) > 0 {
		n := copy(p, m.rxBuf)
		m.rxBuf = m.rxBuf[n:]
		return n, nil
	}
	return 0, nil
}

func (m *mockPort) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.cond.Broadcast()
	return nil
}

func (m *mockPort) FeedRx(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rxBuf = append(m.rxBuf, data...)
	m.cond.Broadcast()
}

func (m *mockPort) WaitTx(minBytes int, timeout time.Duration) []byte {
	deadline := time.Now().Add(timeout)
	m.mu.Lock()
	defer m.mu.Unlock()
	for len(m.txBuf) < minBytes && !m.closed && time.Now().Before(deadline) {
		m.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		m.mu.Lock()
	}
	out := append([]byte(nil), m.txBuf...)
	m.txBuf = nil
	return out
}

func TestMonitorPortQuietTimingAndProbing(t *testing.T) {
	port := newMockPort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedMu sync.Mutex
	var received []Frame
	onFrame := func(f Frame, _ time.Time) {
		receivedMu.Lock()
		received = append(received, f)
		receivedMu.Unlock()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- monitorPort(ctx, port, onFrame)
	}()

	// Simulate background bus traffic: frame from address 2 arrives
	port.FeedRx(testFrame(2))

	// Wait for monitor to poll address 1 (after initialQuiet and quiet window)
	tx := port.WaitTx(10, 2*time.Second)
	if len(tx) < 10 {
		t.Fatalf("expected 10-byte poll for address 1, got %d bytes", len(tx))
	}
	wantPoll1 := PollFrame(1)
	if [10]byte(tx[:10]) != wantPoll1 {
		t.Fatalf("expected poll for addr 1, got %x", tx[:10])
	}

	// Reply to address 1
	port.FeedRx(testFrame(1))

	// Monitor should wait probeGap and then poll address 0
	tx0 := port.WaitTx(10, 2*time.Second)
	if len(tx0) < 10 {
		t.Fatalf("expected 10-byte poll for address 0, got %d bytes", len(tx0))
	}
	wantPoll0 := PollFrame(0)
	if [10]byte(tx0[:10]) != wantPoll0 {
		t.Fatalf("expected poll for addr 0, got %x", tx0[:10])
	}

	// Reply to address 0
	port.FeedRx(testFrame(0))

	// Give callback a moment to register
	time.Sleep(50 * time.Millisecond)

	receivedMu.Lock()
	count := len(received)
	addresses := make([]uint8, len(received))
	for i, f := range received {
		addresses[i] = f.Address
	}
	receivedMu.Unlock()

	if count != 3 {
		t.Fatalf("expected 3 frames (2, 1, 0), got %d: %v", count, addresses)
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("monitorPort did not exit after context cancel")
	}
}

func TestMonitorPortAddress0RetryOnMiss(t *testing.T) {
	port := newMockPort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedMu sync.Mutex
	var received []Frame
	onFrame := func(f Frame, _ time.Time) {
		receivedMu.Lock()
		received = append(received, f)
		receivedMu.Unlock()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- monitorPort(ctx, port, onFrame)
	}()

	// Wait for address 1 poll and answer it
	tx1 := port.WaitTx(10, 2*time.Second)
	if len(tx1) < 10 || tx1[0] != 1 {
		t.Fatalf("expected address 1 poll, got %x", tx1)
	}
	port.FeedRx(testFrame(1))

	// Wait for first address 0 poll
	tx0First := port.WaitTx(10, 2*time.Second)
	if len(tx0First) < 10 || tx0First[0] != 0 {
		t.Fatalf("expected first address 0 poll, got %x", tx0First)
	}

	// Deliberately do NOT reply to the first address 0 poll.
	// Monitor should timeout after probeTimeout (250ms) and retry!
	tx0Retry := port.WaitTx(10, 2*time.Second)
	if len(tx0Retry) < 10 || tx0Retry[0] != 0 {
		t.Fatalf("expected retried address 0 poll, got %x", tx0Retry)
	}

	// Reply to the retried address 0 poll
	port.FeedRx(testFrame(0))

	time.Sleep(50 * time.Millisecond)

	receivedMu.Lock()
	defer receivedMu.Unlock()
	if len(received) != 2 || received[0].Address != 1 || received[1].Address != 0 {
		t.Fatalf("expected [addr1, addr0], got: %+v", received)
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("monitorPort did not exit on cancel")
	}
}

func TestMonitorPortContextCancellation(t *testing.T) {
	port := newMockPort()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- monitorPort(ctx, port, func(Frame, time.Time) {})
	}()

	// Cancel after brief start
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("monitorPort did not return promptly upon cancellation")
	}
}

func TestRunCleanShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	// Overwrite reconnectDelay for fast test
	origDelay := reconnectDelay
	reconnectDelay = 10 * time.Millisecond
	defer func() { reconnectDelay = origDelay }()

	done := make(chan struct{})
	go func() {
		Run(ctx, "/dev/null", func(Frame, time.Time) {})
		close(done)
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(time.Second):
		t.Fatal("Run did not exit on canceled context")
	}
}

func TestMonitorPortSettingsQueryAndDelivery(t *testing.T) {
	port := newMockPort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var settingsReceivedMu sync.Mutex
	var settingsReceived []SettingsFrame
	onSettings := func(s SettingsFrame, _ time.Time) {
		settingsReceivedMu.Lock()
		settingsReceived = append(settingsReceived, s)
		settingsReceivedMu.Unlock()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- monitorPort(ctx, port, func(Frame, time.Time) {}, onSettings)
	}()

	// 1. Complete telemetry probing for addr 1 and addr 0
	tx1 := port.WaitTx(10, 2*time.Second)
	if len(tx1) < 10 || tx1[1] != 0x45 || tx1[0] != 1 {
		t.Fatalf("expected poll for addr 1, got %x", tx1)
	}
	port.FeedRx(testFrame(1))

	tx0 := port.WaitTx(10, 2*time.Second)
	if len(tx0) < 10 || tx0[1] != 0x45 || tx0[0] != 0 {
		t.Fatalf("expected poll for addr 0, got %x", tx0)
	}
	port.FeedRx(testFrame(0))

	// 2. Wait for first settings query (candidate addresses start at 0, balance block 0x1C00)
	txSettings := port.WaitTx(10, 2*time.Second)
	if len(txSettings) < 10 || txSettings[1] != FunctionCodeSettings {
		t.Fatalf("expected 0x78 settings request, got %x", txSettings)
	}

	reqAddr := txSettings[0]
	startReg := binary.BigEndian.Uint16(txSettings[2:4])
	endReg := binary.BigEndian.Uint16(txSettings[4:6])

	// Feed back matching settings reply
	var resp []byte
	if startReg == RegBalanceStart && endReg == RegBalanceEnd {
		resp = makeTestBalanceFrame(reqAddr, 3400, 30, 5680)
	} else {
		resp = makeTestProtectionFrame(reqAddr, 3600, 3650, 5760, 5840)
	}
	port.FeedRx(resp)

	// Allow callback to process
	time.Sleep(50 * time.Millisecond)

	settingsReceivedMu.Lock()
	count := len(settingsReceived)
	settingsReceivedMu.Unlock()

	if count < 1 {
		t.Fatalf("expected at least 1 settings frame received, got %d", count)
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("monitorPort did not exit on cancel")
	}
}

func TestMonitorPortSettingsBoundedRetryOnMissingPack(t *testing.T) {
	port := newMockPort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- monitorPort(ctx, port, func(Frame, time.Time) {})
	}()

	// 1. Telemetry replies
	_ = port.WaitTx(10, 2*time.Second)
	port.FeedRx(testFrame(1))
	_ = port.WaitTx(10, 2*time.Second)
	port.FeedRx(testFrame(0))

	// 2. Settings query 1 arrives: deliberately do NOT reply
	txFirst := port.WaitTx(10, 2*time.Second)
	if len(txFirst) < 10 || txFirst[1] != FunctionCodeSettings {
		t.Fatalf("expected settings query, got %x", txFirst)
	}

	// 3. Monitor should retry once (maxSettingsRetry = 1) after probeTimeout (250ms)
	txRetry := port.WaitTx(10, 2*time.Second)
	if len(txRetry) < 10 || txRetry[1] != FunctionCodeSettings {
		t.Fatalf("expected retried settings query, got %x", txRetry)
	}
	if txRetry[0] != txFirst[0] || txRetry[2] != txFirst[2] {
		t.Fatalf("retry did not match original request: got %x, want %x", txRetry, txFirst)
	}

	// 4. Deliberately do NOT reply to the retry either.
	// After probeTimeout (250ms), monitor must advance to the next task!
	txNext := port.WaitTx(10, 2*time.Second)
	if len(txNext) < 10 || txNext[1] != FunctionCodeSettings {
		t.Fatalf("expected monitor to advance to next settings task, got %x", txNext)
	}

	// Verify next task is indeed different (e.g. protection block or different address)
	if txNext[2] == txFirst[2] && txNext[3] == txFirst[3] && txNext[0] == txFirst[0] {
		t.Fatalf("monitor repeated identical exhausted task instead of advancing")
	}

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("monitorPort did not exit on cancel")
	}
}
