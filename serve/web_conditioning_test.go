package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/daniel-sullivan/srne-solar-controller/conditioning"
)

type conditioningWebFake struct {
	starts int
	stops  int
	err    error
	status ConditioningServiceStatus
}

func (f *conditioningWebFake) Start(context.Context) error { f.starts++; return f.err }
func (f *conditioningWebFake) Stop(context.Context) error  { f.stops++; return f.err }
func (f *conditioningWebFake) Status() ConditioningServiceStatus {
	if f.status.Enabled || f.status.Reason != "" {
		return f.status
	}
	return ConditioningServiceStatus{
		Engine: conditioning.Status{State: conditioning.StateIdle, Stage: conditioning.StageNone},
		Reason: "ready",
	}
}

func TestConditioningAPIUsesControllerEnablement(t *testing.T) {
	ws := NewWebServer(nil, nil)
	ws.SetConditioning(&conditioningWebFake{status: ConditioningServiceStatus{Reason: "not configured"}})
	rec := httptest.NewRecorder()
	ws.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/conditioning", nil))
	var got ConditioningAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("controller-reported disabled state was changed to enabled")
	}
}

func TestConditioningAPIReportsDisabledWithoutController(t *testing.T) {
	ws := NewWebServer(nil, nil)
	rec := httptest.NewRecorder()
	ws.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/conditioning", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got ConditioningAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("disabled server reported enabled conditioning")
	}
}

func TestConditioningGETDoesNotInvokeController(t *testing.T) {
	fake := &conditioningWebFake{}
	ws := NewWebServer(nil, nil)
	ws.SetConditioning(fake)
	rec := httptest.NewRecorder()
	ws.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/conditioning", nil))

	if rec.Code != http.StatusOK || fake.starts != 0 || fake.stops != 0 {
		t.Fatalf("GET status=%d starts=%d stops=%d", rec.Code, fake.starts, fake.stops)
	}
}

func TestConditioningPageOffersRecoveryWhenMonitorDisabled(t *testing.T) {
	fake := &conditioningWebFake{status: ConditioningServiceStatus{RestorePending: true, Reason: "restoration pending"}}
	ws := NewWebServer(nil, nil)
	ws.SetConditioning(fake)
	rec := httptest.NewRecorder()
	ws.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/conditioning", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "Stop and restore") || !strings.Contains(page, "Restoration pending") {
		t.Fatal("recovery control or pending warning missing from conditioning page")
	}
	if strings.Contains(page, "Start conditioning</button>") {
		t.Fatal("start control shown while restoration is pending")
	}
}

func TestConditioningStartAndStopAreExplicit(t *testing.T) {
	fake := &conditioningWebFake{}
	ws := NewWebServer(nil, nil)
	ws.SetConditioning(fake)
	h := ws.Handler()

	start := httptest.NewRecorder()
	h.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/conditioning/start", nil))
	stop := httptest.NewRecorder()
	h.ServeHTTP(stop, httptest.NewRequest(http.MethodPost, "/api/conditioning/stop", nil))

	if start.Code != http.StatusOK || stop.Code != http.StatusOK {
		t.Fatalf("start/stop statuses = %d/%d", start.Code, stop.Code)
	}
	if fake.starts != 1 || fake.stops != 1 {
		t.Fatalf("start/stop calls = %d/%d", fake.starts, fake.stops)
	}
}

func TestConditioningStartErrorUsesServerError(t *testing.T) {
	fake := &conditioningWebFake{err: errors.New("preflight failed")}
	ws := NewWebServer(nil, nil)
	ws.SetConditioning(fake)
	rec := httptest.NewRecorder()
	ws.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/conditioning/start", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
