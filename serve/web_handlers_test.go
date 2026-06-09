package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

// newRunningHubWebServer starts a hub against the mock system and returns a
// WebServer wired to both. It mirrors the setup pattern in parity_test.go:
// capture sys.Units() before the hub starts to avoid races, run the hub with a
// cancelable context cleaned up via t.Cleanup, and sleep for the first poll +
// settings refresh.
func newRunningHubWebServer(t *testing.T) (*WebServer, *Hub, *inverter.System) {
	t.Helper()
	sys := newTestSystem(t)
	_ = sys.Units() // touch before hub starts (parity_test.go pattern)
	hub := NewHub(sys, 50*time.Millisecond, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(150 * time.Millisecond) // first poll + settings refresh

	return NewWebServer(hub, sys), hub, sys
}

func TestHandleSettingsPage(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	body := w.Body.String()
	assert.Contains(t, body, "SRNE Solar Dashboard")
	// settings page should render a settings form / Apply control
	assert.Contains(t, strings.ToLower(body), "settings")
}

func TestHandleSettingsPageLoading(t *testing.T) {
	// Hub never refreshes settings → Settings() is nil → loading page.
	sys := newTestSystem(t)
	hub := NewHub(sys, time.Hour, time.Hour)
	ws := NewWebServer(hub, sys)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "SRNE Solar Dashboard")
}

func TestHandleFaultsPage(t *testing.T) {
	// The fault read is serialized through the hub run loop, so it stays race-clean
	// even against an actively-polling hub. A running hub is required for the read
	// to be serviced.
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/faults", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	// The mock does not implement the fault-history range, so ReadFaults errors;
	// handleFaults logs and renders the page with no faults (still 200 HTML).
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "SRNE Solar Dashboard")
}

func TestHandleFaultsPageSystemNotReady(t *testing.T) {
	hub := NewHub(nil, time.Hour, time.Hour)
	ws := NewWebServer(hub, nil) // system nil → loading page

	req := httptest.NewRequest(http.MethodGet, "/faults", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestHandleDashboardLoading(t *testing.T) {
	// No poll yet → Latest() nil → loading page (covers the nil-snapshot branch).
	sys := newTestSystem(t)
	hub := NewHub(sys, time.Hour, time.Hour)
	ws := NewWebServer(hub, sys)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "SRNE Solar Dashboard")
}

func TestHandleAPISettings(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var settings inverter.Settings
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &settings))
}

func TestHandleAPISettingsNoData(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, time.Hour, time.Hour) // settings never refreshed
	ws := NewWebServer(hub, sys)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "no settings yet", resp["error"])
}

func TestHandleAPIFaults(t *testing.T) {
	// ReadFaults is serialized through the hub run loop (race-clean against polling);
	// a running hub is required to service the read.
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/faults", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	// The mock simulator does not implement the fault-history register range, so
	// ReadFaults returns an "illegal address" error and the handler responds 500
	// with a JSON {"error":...} body. (System-not-ready 503 is covered separately
	// in web_faults_startup_test.go.)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["error"])
}

func TestHandleWriteSettings(t *testing.T) {
	ws, hub, _ := newRunningHubWebServer(t)

	body := `{"changes":[{"field":"mains_charge_current_lim","value":"20"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var resp settingsWriteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	require.Len(t, resp.Applied, 1)
	assert.Equal(t, "mains_charge_current_lim", resp.Applied[0].Field)
	assert.Empty(t, resp.Errors)

	// Value should round-trip into the hub's settings after a refresh.
	time.Sleep(150 * time.Millisecond)
	settings := hub.Settings()
	require.NotNil(t, settings)
	assert.Equal(t, 20.0, settings.Inverter.MainsChargeCurrentLim)
}

func TestHandleWriteSettingsInvalidField(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	body := `{"changes":[{"field":"not_a_real_field","value":"1"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	// All changes failed → 500 with ok:false and a populated errors list.
	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp settingsWriteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.OK)
	require.Len(t, resp.Errors, 1)
	assert.Equal(t, "not_a_real_field", resp.Errors[0].Field)
	assert.NotEmpty(t, resp.Errors[0].Message)
	assert.Empty(t, resp.Applied)
}

func TestHandleWriteSettingsMalformedJSON(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings/write", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleWriteSettingsNoChanges(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/settings/write", strings.NewReader(`{"changes":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAPIControlWriteNumber(t *testing.T) {
	ws, hub, _ := newRunningHubWebServer(t)

	body := strings.NewReader(`{"value":"20"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/controls/mains_charge_current", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["ok"])

	// The write should land in settings the way the dashboard reads it back.
	time.Sleep(150 * time.Millisecond)
	settings := hub.Settings()
	require.NotNil(t, settings)
	assert.Equal(t, 20.0, settings.Inverter.MainsChargeCurrentLim)
}

func TestHandleAPIControlWriteUnknownKey(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	body := strings.NewReader(`{"value":"1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/controls/not_a_control", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["ok"])
	assert.NotEmpty(t, resp["error"])
}

func TestHandleAPIControlWriteMalformedJSON(t *testing.T) {
	ws, _, _ := newRunningHubWebServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/controls/mains_charge_current", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFaultCodeName(t *testing.T) {
	tests := []struct {
		code uint16
		want string
	}{
		{1, "BatVoltLow"},
		{6, "BatOverVolt"},
		{60000, "Unknown(60000)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, faultCodeName(tt.code))
	}
}

func TestWalkJSON(t *testing.T) {
	m := map[string]any{
		"battery": map[string]any{"soc": 85.0},
	}
	assert.Equal(t, 85.0, walkJSON(m, "battery.soc"))
	assert.Nil(t, walkJSON(m, "battery.missing"))
	assert.Nil(t, walkJSON(m, "battery.soc.deeper")) // descend into non-map
	assert.Nil(t, walkJSON(nil, "battery.soc"))
}

func TestSetSystemAndMPPTLabels(t *testing.T) {
	hub := NewHub(nil, time.Hour, time.Hour)
	ws := NewWebServer(hub, nil)
	assert.Nil(t, ws.system)

	sys := newTestSystem(t)
	ws.SetSystem(sys)
	assert.Same(t, sys, ws.system)

	labels := map[string][2]string{"10.0.0.1": {"East", "West"}}
	ws.SetMPPTLabels(labels)
	assert.Equal(t, labels, ws.mpptLabels)
}
