package serve

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

// TestSnapshotValuesNil verifies a nil snapshot yields a nil map.
func TestSnapshotValuesNil(t *testing.T) {
	assert.Nil(t, snapshotValues(nil))
}

// TestSnapshotValues verifies that snapshotValues flattens a snapshot into the
// expected data-v keyed strings, including per-unit keys.
func TestSnapshotValues(t *testing.T) {
	snap := &inverter.Snapshot{}
	snap.Battery.SOC = 85
	snap.Battery.Voltage = 53.6
	snap.Battery.Current = 12.3
	snap.Battery.ChargeStatusName = "charging"
	snap.PV.TotalPower = 1040
	snap.PV.PV1Power = 600
	snap.PV.PV1Voltage = 120.5
	snap.PV.PV1Current = 5.0
	snap.Load.TotalPower = 500
	snap.Grid.L1.GridVoltage = 120.1
	snap.Grid.Frequency = 60.0
	snap.Inverter.MachineStateName = "running"
	snap.Units = []inverter.UnitSnapshot{
		{Host: "10.0.0.1"},
	}
	snap.Units[0].Inverter.MachineStateName = "running"
	snap.Units[0].Inverter.BusVoltage = 380.5
	snap.Units[0].PV.TotalPower = 1040

	v := snapshotValues(snap)
	require.NotNil(t, v)

	assert.Equal(t, "85", v["bat-soc"])
	assert.Equal(t, "53.6", v["bat-v"])
	assert.Equal(t, "12.3", v["bat-i"])
	assert.Equal(t, "charging", v["bat-status"])
	assert.Equal(t, "1040", v["pv-total"])
	assert.Equal(t, "600", v["pv1-w"])
	assert.Equal(t, "120.5/5.0", v["pv1-vi"])
	assert.Equal(t, "500", v["load-total"])
	assert.Equal(t, "120.1", v["grid-l1-v"])
	assert.Equal(t, "60.00", v["grid-freq"])
	assert.Equal(t, "running", v["inv-state"])

	// Per-unit keys.
	assert.Equal(t, "running", v["unit-state-10.0.0.1"])
	assert.Equal(t, "380.5", v["unit-bus-10.0.0.1"])
	assert.Equal(t, "1040", v["unit-pv-10.0.0.1"])
}

// TestDiffValues verifies diffValues returns only the keys that changed.
func TestDiffValues(t *testing.T) {
	t.Run("nil_prev_is_empty", func(t *testing.T) {
		cur := map[string]string{"bat-soc": "85"}
		changed := diffValues(nil, cur)
		assert.Empty(t, changed)
	})

	t.Run("no_change_is_empty", func(t *testing.T) {
		prev := map[string]string{"bat-soc": "85", "bat-v": "53.6"}
		cur := map[string]string{"bat-soc": "85", "bat-v": "53.6"}
		changed := diffValues(prev, cur)
		assert.Empty(t, changed)
	})

	t.Run("only_changed_keys", func(t *testing.T) {
		prev := map[string]string{"bat-soc": "85", "bat-v": "53.6", "load-w": "500"}
		cur := map[string]string{"bat-soc": "86", "bat-v": "53.6", "load-w": "600"}
		changed := diffValues(prev, cur)
		assert.True(t, changed["bat-soc"], "bat-soc should be marked changed")
		assert.True(t, changed["load-w"], "load-w should be marked changed")
		assert.False(t, changed["bat-v"], "bat-v unchanged should not be marked")
		assert.Len(t, changed, 2)
	})

	t.Run("new_key_not_marked", func(t *testing.T) {
		// A key absent from prev is not "changed" (it's new) — only keys present
		// in prev with a different value count.
		prev := map[string]string{"bat-soc": "85"}
		cur := map[string]string{"bat-soc": "85", "new-key": "1"}
		changed := diffValues(prev, cur)
		assert.Empty(t, changed)
	})
}

// TestSnapshotDiffRoundTrip drives snapshotValues + diffValues end-to-end with
// two snapshots that differ in one field, asserting exactly that key diffs.
func TestSnapshotDiffRoundTrip(t *testing.T) {
	a := &inverter.Snapshot{}
	a.Battery.SOC = 85
	a.Load.TotalPower = 500

	b := &inverter.Snapshot{}
	b.Battery.SOC = 90 // changed
	b.Load.TotalPower = 500

	va := snapshotValues(a)
	vb := snapshotValues(b)

	changed := diffValues(va, vb)
	assert.True(t, changed["bat-soc"], "bat-soc changed 85→90")
	assert.False(t, changed["load-total"], "load unchanged")
}

// TestWriteSSEEvent verifies the exact SSE wire format produced by writeSSEEvent,
// including multi-line data splitting and the blank-line terminator.
func TestWriteSSEEvent(t *testing.T) {
	t.Run("single_line", func(t *testing.T) {
		var buf bytes.Buffer
		writeSSEEvent(&recorderWriter{&buf}, "snapshot", `{"soc":85}`)
		assert.Equal(t, "event: snapshot\ndata: {\"soc\":85}\n\n", buf.String())
	})

	t.Run("multi_line", func(t *testing.T) {
		var buf bytes.Buffer
		writeSSEEvent(&recorderWriter{&buf}, "battery", "line1\nline2")
		assert.Equal(t, "event: battery\ndata: line1\ndata: line2\n\n", buf.String())
	})
}

// recorderWriter adapts a bytes.Buffer to http.ResponseWriter for writeSSEEvent,
// which only uses the Write method.
type recorderWriter struct{ buf *bytes.Buffer }

func (r *recorderWriter) Header() http.Header        { return http.Header{} }
func (r *recorderWriter) WriteHeader(int)            {}
func (r *recorderWriter) Write(p []byte) (int, error) { return r.buf.Write(p) }

// TestVC covers the vc template helper.
func TestVC(t *testing.T) {
	changed := map[string]bool{"bat-soc": true}
	assert.Equal(t, "value-changed", vc(changed, "bat-soc"))
	assert.Equal(t, "", vc(changed, "bat-v"))
	assert.Equal(t, "", vc(nil, "anything"))
}

// TestMPPT covers the MPPT template helper, including override and fallback paths.
func TestMPPT(t *testing.T) {
	td := templateData{
		MPPTLabels: map[string][2]string{
			"10.0.0.1": {"Roof", ""},
		},
	}
	// Configured override.
	assert.Equal(t, "Roof", td.MPPT("10.0.0.1", 1))
	// Configured host but empty label → fallback.
	assert.Equal(t, "MPPT 2", td.MPPT("10.0.0.1", 2))
	// Unknown host → fallback.
	assert.Equal(t, "MPPT 1", td.MPPT("10.9.9.9", 1))
	// Out-of-range index → fallback.
	assert.Equal(t, "MPPT 3", td.MPPT("10.0.0.1", 3))
	assert.Equal(t, "MPPT 0", td.MPPT("10.0.0.1", 0))
}

// TestHandleSSE drives the streaming /api/snapshot/stream endpoint against a real
// httptest server, reads at least one full SSE event, then cancels the request
// context so the handler returns. Bounded by a context timeout so it can never
// hang the suite.
func TestHandleSSE(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, time.Hour)

	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go hub.Run(hubCtx)
	time.Sleep(150 * time.Millisecond) // wait for first poll

	ws := NewWebServer(hub, sys)
	srv := httptest.NewServer(ws.Handler())
	t.Cleanup(srv.Close)

	// Bound the request lifetime so it self-terminates no matter what.
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/snapshot/stream", nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

	// Read until we observe a complete event with a "data:" line, or time out.
	scanner := bufio.NewScanner(resp.Body)
	var sawEvent, sawData, sawSnapshot bool
	deadline := time.After(2 * time.Second)
	lines := make(chan string)
	go func() {
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-reqCtx.Done():
				return
			}
		}
		_ = scanner.Err() // body is closed on cancel; the resulting read error is expected
		close(lines)
	}()

readLoop:
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				break readLoop
			}
			if strings.HasPrefix(line, "event:") {
				sawEvent = true
				if strings.Contains(line, "snapshot") {
					sawSnapshot = true
				}
			}
			if strings.HasPrefix(line, "data:") {
				sawData = true
			}
			// Once we've seen a full event including the JSON snapshot, we're done.
			if sawSnapshot && sawData {
				reqCancel()
				break readLoop
			}
		case <-deadline:
			break readLoop
		}
	}

	assert.True(t, sawEvent, "expected at least one SSE 'event:' line")
	assert.True(t, sawData, "expected at least one SSE 'data:' line")
	assert.True(t, sawSnapshot, "expected a 'snapshot' SSE event")
}

// TestHandleSSEContextCancel verifies the handler returns promptly when the
// client context is cancelled before any data is read, via httptest.Recorder
// driving Handler() directly with a cancellable request context.
func TestHandleSSEContextCancel(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, time.Hour)

	hubCtx, hubCancel := context.WithCancel(context.Background())
	t.Cleanup(hubCancel)
	go hub.Run(hubCtx)
	time.Sleep(150 * time.Millisecond)

	ws := NewWebServer(hub, sys)

	reqCtx, reqCancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/snapshot/stream", nil).WithContext(reqCtx)
	w := newFlushRecorder()

	done := make(chan struct{})
	go func() {
		ws.Handler().ServeHTTP(w, req)
		close(done)
	}()

	// Let it emit at least one event, then cancel.
	time.Sleep(120 * time.Millisecond)
	reqCancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleSSE did not return after context cancel")
	}

	assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	assert.Contains(t, w.body.String(), "data:")
}

// flushRecorder is an httptest.ResponseRecorder that also implements http.Flusher
// so handleSSE's flusher type-assertion succeeds.
type flushRecorder struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{header: http.Header{}, body: new(bytes.Buffer), code: http.StatusOK}
}

func (f *flushRecorder) Header() http.Header         { return f.header }
func (f *flushRecorder) Write(p []byte) (int, error) { return f.body.Write(p) }
func (f *flushRecorder) WriteHeader(code int)        { f.code = code }
func (f *flushRecorder) Flush()                      {}
