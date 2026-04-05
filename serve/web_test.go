package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
)

func newTestWebServer(t *testing.T) *WebServer {
	t.Helper()
	sys := newTestSystem(t)
	hub := NewHub(sys, 50*time.Millisecond, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go hub.Run(ctx)
	time.Sleep(100 * time.Millisecond) // wait for first poll

	return NewWebServer(hub, sys)
}

func TestAPIsnapshot(t *testing.T) {
	ws := newTestWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var snap inverter.Snapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snap))
	assert.Equal(t, float64(85), snap.Battery.SOC)
}

func TestAPISnapshotNoData(t *testing.T) {
	sys := newTestSystem(t)
	hub := NewHub(sys, time.Hour, time.Hour) // no poll yet
	ws := NewWebServer(hub, sys)

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestDashboardPage(t *testing.T) {
	ws := newTestWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "SRNE Solar Dashboard")
	assert.Contains(t, body, "beer.min.css")
	assert.Contains(t, body, "htmx.org")
	assert.Contains(t, body, "sse-connect")
	assert.Contains(t, body, ">85<") // SOC value
}

func TestStaticCSS(t *testing.T) {
	ws := newTestWebServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Dashboard layout")
}
