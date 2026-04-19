package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIFaultsSystemNotReady(t *testing.T) {
	hub := NewHub(nil, time.Hour, time.Hour)
	ws := NewWebServer(hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/faults", nil)
	w := httptest.NewRecorder()
	ws.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "fault history not available yet", resp["error"])
}
