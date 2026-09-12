package serve

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/bms/interpack"
	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

type conditioningPageData struct {
	Page         string
	Conditioning *ConditioningAPIResponse
}

// ConditioningAPIResponse keeps the enabled state explicit while exposing the
// controller status as stable, machine-readable JSON.
type ConditioningAPIResponse struct {
	ConditioningServiceStatus
}

func (ws *WebServer) conditioningStatus() *ConditioningAPIResponse {
	ws.conditioningMu.RLock()
	controller := ws.conditioning
	ws.conditioningMu.RUnlock()
	if controller == nil {
		return &ConditioningAPIResponse{}
	}
	return &ConditioningAPIResponse{ConditioningServiceStatus: controller.Status()}
}

func (ws *WebServer) handleConditioning(w http.ResponseWriter, _ *http.Request) {
	data := conditioningPageData{Page: "conditioning", Conditioning: ws.conditioningStatus()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ws.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (ws *WebServer) handleAPIConditioning(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ws.conditioningStatus())
}

func (ws *WebServer) handleConditioningStart(w http.ResponseWriter, r *http.Request) {
	ws.conditioningMu.RLock()
	controller := ws.conditioning
	ws.conditioningMu.RUnlock()
	if controller == nil {
		writeConditioningError(w, http.StatusServiceUnavailable, "battery conditioning is disabled")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := controller.Start(ctx); err != nil {
		writeConditioningError(w, conditioningErrorStatus(err), err.Error())
		return
	}
	ws.writeConditioningJSON(w, http.StatusOK)
}

func (ws *WebServer) handleConditioningStop(w http.ResponseWriter, r *http.Request) {
	ws.conditioningMu.RLock()
	controller := ws.conditioning
	ws.conditioningMu.RUnlock()
	if controller == nil {
		writeConditioningError(w, http.StatusServiceUnavailable, "battery conditioning is disabled")
		return
	}
	// Recovery must outlive a browser disconnect: the controller owns the
	// detached recovery context and will leave RestorePending visible on error.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := controller.Stop(ctx); err != nil {
		writeConditioningError(w, conditioningErrorStatus(err), err.Error())
		return
	}
	ws.writeConditioningJSON(w, http.StatusOK)
}

func (ws *WebServer) writeConditioningJSON(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ws.conditioningStatus())
}

func writeConditioningError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": message})
}

func conditioningErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrConditioningActive), errors.Is(err, ErrConditioningRecovery), errors.Is(err, ErrConditioningReserved):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// BMSFreshnessDuration is the default maximum age for a BMS telemetry snapshot to be considered fresh.
// Four 15-second poll cycles (60s) allows transient retransmissions while identifying disappeared packs.
const BMSFreshnessDuration = 60 * time.Second

// WebServer handles HTTP routes for the dashboard and API.
type WebServer struct {
	hub            *Hub
	system         *inverter.System
	tmpl           *template.Template
	mpptLabels     map[string][2]string // host -> [mppt1, mppt2] display labels
	bmsMu          sync.RWMutex
	bmsStore       *interpack.Store
	bmsConfig      *BMSConfig
	conditioningMu sync.RWMutex
	conditioning   ConditioningController
}

// ConditioningController is the small lifecycle surface exposed to the web
// layer. Implementations own the inverter transaction and recovery lifecycle.
type ConditioningController interface {
	Start(context.Context) error
	Stop(context.Context) error
	Status() ConditioningServiceStatus
}

// selectOption is a dropdown option for templates.
type selectOption struct {
	Value uint16
	Label string
}

// Template functions available in all templates.
var templateFuncs = template.FuncMap{
	"vc":  vc,
	"add": func(a, b int) int { return a + b },
	"outputPriorityOptions": func() []selectOption {
		return []selectOption{{0, "SOL (Solar First)"}, {1, "UTI (Mains First)"}, {2, "SBU (Solar→Battery→Utility)"}}
	},
	"chargerPriorityOptions": func() []selectOption {
		return []selectOption{{0, "CSO (PV Preferred)"}, {1, "CUB (Mains Preferred)"}, {2, "SNU (Hybrid)"}, {3, "OSO (PV Only)"}}
	},
	"boolToUint16": func(b bool) uint16 {
		if b {
			return 1
		}
		return 0
	},
	"dict": func(pairs ...any) map[string]any {
		m := make(map[string]any, len(pairs)/2)
		for i := 0; i+1 < len(pairs); i += 2 {
			m[pairs[i].(string)] = pairs[i+1]
		}
		return m
	},
}

// NewWebServer creates a web server backed by the given hub and system.
// The system may be nil initially — call SetSystem once available.
func NewWebServer(hub *Hub, system *inverter.System) *WebServer {
	tmpl := template.Must(
		template.New("").Funcs(templateFuncs).ParseFS(templateFS,
			"templates/*.html",
			"templates/partials/*.html",
		),
	)
	return &WebServer{hub: hub, system: system, tmpl: tmpl}
}

// SetSystem sets the inverter system after deferred initialization.
func (ws *WebServer) SetSystem(system *inverter.System) {
	ws.system = system
}

// SetMPPTLabels registers per-inverter labels for MPPT 1 and MPPT 2, keyed by host.
// An empty string in either slot falls back to the default "MPPT 1" / "MPPT 2".
func (ws *WebServer) SetMPPTLabels(labels map[string][2]string) {
	ws.mpptLabels = labels
}

// SetBMS configures the BMS store and optional configuration for the web server.
func (ws *WebServer) SetBMS(store *interpack.Store, cfg *BMSConfig) {
	ws.bmsMu.Lock()
	defer ws.bmsMu.Unlock()
	ws.bmsStore = store
	ws.bmsConfig = cfg
}

// SetConditioning attaches the manual battery-conditioning controller.
func (ws *WebServer) SetConditioning(controller ConditioningController) {
	ws.conditioningMu.Lock()
	defer ws.conditioningMu.Unlock()
	ws.conditioning = controller
}

// Handler returns the HTTP handler with all routes registered.
func (ws *WebServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// Dashboard
	mux.HandleFunc("GET /", ws.handleDashboard)

	// Settings
	mux.HandleFunc("GET /settings", ws.handleSettings)
	mux.HandleFunc("POST /api/settings/write", ws.handleWriteSettings)

	// Faults
	mux.HandleFunc("GET /faults", ws.handleFaults)
	mux.HandleFunc("GET /bank", ws.handleBank)
	mux.HandleFunc("GET /conditioning", ws.handleConditioning)

	// REST API
	mux.HandleFunc("GET /api/snapshot", ws.handleAPISnapshot)
	mux.HandleFunc("GET /api/settings", ws.handleAPISettings)
	mux.HandleFunc("GET /api/faults", ws.handleAPIFaults)
	mux.HandleFunc("GET /api/entities", ws.handleAPIEntities)
	mux.HandleFunc("POST /api/controls/{key}", ws.handleAPIControlWrite)
	mux.HandleFunc("GET /api/bms", ws.handleAPIBMS)
	mux.HandleFunc("GET /api/conditioning", ws.handleAPIConditioning)
	mux.HandleFunc("POST /api/conditioning/start", ws.handleConditioningStart)
	mux.HandleFunc("POST /api/conditioning/stop", ws.handleConditioningStop)

	// SSE stream
	mux.HandleFunc("GET /api/snapshot/stream", ws.handleSSE)

	// Static files
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	return mux
}

func (ws *WebServer) renderPage(w http.ResponseWriter, data pageData) {
	if data.MPPTLabels == nil {
		data.MPPTLabels = ws.mpptLabels
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ws.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (ws *WebServer) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	snap := ws.hub.Latest()
	if snap == nil {
		ws.renderPage(w, pageData{Page: "loading"})
		return
	}
	ws.renderPage(w, pageData{Page: "dashboard", Snapshot: snap})
}

func (ws *WebServer) handleBank(w http.ResponseWriter, _ *http.Request) {
	ws.renderPage(w, pageData{Page: "bank"})
}

func (ws *WebServer) handleSettings(w http.ResponseWriter, _ *http.Request) {
	settings := ws.hub.Settings()
	if settings == nil {
		ws.renderPage(w, pageData{Page: "loading"})
		return
	}
	ws.renderPage(w, pageData{Page: "settings", Settings: settings})
}

func (ws *WebServer) handleFaults(w http.ResponseWriter, r *http.Request) {
	if ws.system == nil {
		ws.renderPage(w, pageData{Page: "loading"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	faults, err := ws.hub.ReadFaults(ctx)
	if err != nil {
		slog.Warn("fault history read failed", "error", err)
		faults = nil
	}

	data := pageData{
		Page:   "faults",
		Faults: faults,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ws.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// settingsWriteRequest is the JSON body for batch settings writes.
type settingsWriteRequest struct {
	Changes []settingsChange `json:"changes"`
}

type settingsChange struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type settingsWriteResponse struct {
	OK      bool             `json:"ok"`
	Applied []settingsChange `json:"applied,omitempty"`
	Errors  []settingsError  `json:"errors,omitempty"`
}

type settingsError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (ws *WebServer) handleWriteSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Changes) == 0 {
		http.Error(w, "no changes", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	slog.Info("settings write request", "changes", len(req.Changes))

	// Route all writes through the hub to serialize with polling
	var resp settingsWriteResponse
	for _, c := range req.Changes {
		if err := ws.hub.WriteSetting(ctx, c.Field, c.Value); err != nil {
			resp.Errors = append(resp.Errors, settingsError{Field: c.Field, Message: err.Error()})
		} else {
			resp.Applied = append(resp.Applied, c)
		}
	}
	resp.OK = len(resp.Errors) == 0

	w.Header().Set("Content-Type", "application/json")
	if !resp.OK {
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (ws *WebServer) handleAPISnapshot(w http.ResponseWriter, _ *http.Request) {
	snap := ws.hub.Latest()
	w.Header().Set("Content-Type", "application/json")
	if snap == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no data yet"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(snap)
}

func (ws *WebServer) handleAPISettings(w http.ResponseWriter, _ *http.Request) {
	settings := ws.hub.Settings()
	w.Header().Set("Content-Type", "application/json")
	if settings == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"no settings yet"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(settings)
}

func (ws *WebServer) handleAPIFaults(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if ws.system == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "fault history not available yet"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	faults, err := ws.hub.ReadFaults(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(faults)
}

func (ws *WebServer) handleAPIEntities(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	snap := ws.hub.Latest()
	settings := ws.hub.Settings()

	if settings != nil {
		ws.hub.InitMainsChargeCurrent(settings)
	}

	// Build sensor list with current values
	var snapMap map[string]any
	if snap != nil {
		data, _ := json.Marshal(snap)
		_ = json.Unmarshal(data, &snapMap)
	}

	type sensorEntity struct {
		Key         string `json:"key"`
		Name        string `json:"name"`
		Unit        string `json:"unit,omitempty"`
		DeviceClass string `json:"device_class,omitempty"`
		StateClass  string `json:"state_class,omitempty"`
		Icon        string `json:"icon,omitempty"`
		ValuePath   string `json:"value_path"`
		Value       any    `json:"value"`
	}
	type switchEntity struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Icon  string `json:"icon,omitempty"`
		State string `json:"state"`
	}
	type numberEntity struct {
		Key   string  `json:"key"`
		Name  string  `json:"name"`
		Icon  string  `json:"icon,omitempty"`
		Unit  string  `json:"unit,omitempty"`
		Field string  `json:"field"`
		Min   float64 `json:"min"`
		Max   float64 `json:"max"`
		Step  float64 `json:"step"`
		Value string  `json:"value"`
	}
	type selectEntity struct {
		Key     string   `json:"key"`
		Name    string   `json:"name"`
		Icon    string   `json:"icon,omitempty"`
		Field   string   `json:"field"`
		Options []string `json:"options"`
		Value   string   `json:"value"`
	}
	type textEntity struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Icon  string `json:"icon,omitempty"`
		Field string `json:"field"`
		Value string `json:"value"`
	}

	sensors := make([]sensorEntity, 0, len(systemSensors))
	for _, s := range systemSensors {
		se := sensorEntity{
			Key:         s.Key,
			Name:        s.Name,
			Unit:        s.Unit,
			DeviceClass: s.DeviceClass,
			StateClass:  s.StateClass,
			Icon:        s.Icon,
			ValuePath:   s.ValuePath,
			Value:       walkJSON(snapMap, s.ValuePath),
		}
		sensors = append(sensors, se)
	}

	switches := make([]switchEntity, 0, len(controlSwitches))
	for _, sw := range controlSwitches {
		state := ""
		if settings != nil {
			state = sw.StateFunc(settings)
		}
		switches = append(switches, switchEntity{Key: sw.Key, Name: sw.Name, Icon: sw.Icon, State: state})
	}

	numbers := make([]numberEntity, 0, len(controlNumbers))
	for _, num := range controlNumbers {
		value := ""
		if settings != nil {
			value = num.StateFunc(settings)
		}
		numbers = append(numbers, numberEntity{
			Key: num.Key, Name: num.Name, Icon: num.Icon, Unit: num.Unit,
			Field: num.Field, Min: num.Min, Max: num.Max, Step: num.Step, Value: value,
		})
	}

	selects := make([]selectEntity, 0, len(controlSelects))
	for _, sel := range controlSelects {
		value := ""
		if settings != nil {
			value = sel.StateFunc(settings)
		}
		selects = append(selects, selectEntity{
			Key: sel.Key, Name: sel.Name, Icon: sel.Icon,
			Field: sel.Field, Options: sel.Options, Value: value,
		})
	}

	texts := make([]textEntity, 0, len(controlTexts))
	for _, txt := range controlTexts {
		value := ""
		if settings != nil {
			value = txt.StateFunc(settings)
		}
		texts = append(texts, textEntity{Key: txt.Key, Name: txt.Name, Icon: txt.Icon, Field: txt.Field, Value: value})
	}

	resp := map[string]any{
		"sensors":  sensors,
		"switches": switches,
		"numbers":  numbers,
		"selects":  selects,
		"texts":    texts,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (ws *WebServer) handleAPIControlWrite(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), controlWriteTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")
	if err := executeControl(ctx, ws.hub, key, body.Value); err != nil {
		slog.Error("control write failed", "key", key, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	slog.Info("control written", "key", key, "value", body.Value)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// BMSResponse is the JSON response structure for GET /api/bms.
// It reports whether the BMS monitor is configured, whether live telemetry has been
// observed, telemetry freshness summary counts, settings and alarm coverage counts,
// and individual pack states with verified settings. It provides read-only factual telemetry
// and does not certify battery health or safety.
type BMSResponse struct {
	Configured               bool             `json:"configured"`
	SerialDevice             string           `json:"serial_device,omitempty"`
	Observed                 bool             `json:"observed"`
	Status                   string           `json:"status"`
	AllObservedFresh         bool             `json:"all_observed_fresh"`
	FreshnessSeconds         int              `json:"freshness_seconds"`
	FreshnessDuration        string           `json:"freshness_duration"`
	TotalPacks               int              `json:"total_packs"`
	FreshPacks               int              `json:"fresh_packs"`
	StalePacks               int              `json:"stale_packs"`
	UnconfirmedPacks         int              `json:"unconfirmed_packs"`
	UncertainPacks           int              `json:"uncertain_packs"`
	AllObservedSettingsFresh bool             `json:"all_observed_settings_fresh"`
	AllObservedAlarmsClear   bool             `json:"all_observed_alarms_clear"`
	SettingsFreshPacks       int              `json:"settings_fresh_packs"`
	SettingsStalePacks       int              `json:"settings_stale_packs"`
	Packs                    []interpack.Pack `json:"packs"`
}

func (ws *WebServer) handleAPIBMS(w http.ResponseWriter, _ *http.Request) {
	ws.bmsMu.RLock()
	store := ws.bmsStore
	cfg := ws.bmsConfig
	ws.bmsMu.RUnlock()

	configured := (cfg != nil && cfg.SerialDevice != "") || store != nil
	var device string
	if cfg != nil {
		device = cfg.SerialDevice
	}

	freshnessSec := int(BMSFreshnessDuration.Seconds())
	freshnessStr := fmt.Sprintf("%ds", freshnessSec)

	w.Header().Set("Content-Type", "application/json")

	if !configured || store == nil {
		status := "disabled"
		if configured {
			status = "unseen"
		}
		_ = json.NewEncoder(w).Encode(BMSResponse{
			Configured:               configured,
			SerialDevice:             device,
			Observed:                 false,
			Status:                   status,
			AllObservedFresh:         false,
			FreshnessSeconds:         freshnessSec,
			FreshnessDuration:        freshnessStr,
			TotalPacks:               0,
			FreshPacks:               0,
			StalePacks:               0,
			UnconfirmedPacks:         0,
			UncertainPacks:           0,
			AllObservedSettingsFresh: false,
			AllObservedAlarmsClear:   false,
			SettingsFreshPacks:       0,
			SettingsStalePacks:       0,
			Packs:                    make([]interpack.Pack, 0),
		})
		return
	}

	packs := store.Snapshot(time.Now(), BMSFreshnessDuration)
	if packs == nil {
		packs = make([]interpack.Pack, 0)
	}

	totalPacks := len(packs)
	freshPacks := 0
	stalePacks := 0
	unconfirmedPacks := 0
	uncertainPacks := 0
	settingsFreshPacks := 0
	settingsStalePacks := 0
	allAlarmsClear := true

	for _, p := range packs {
		if p.Fresh {
			freshPacks++
		} else {
			stalePacks++
		}
		if !p.Confirmed {
			unconfirmedPacks++
		}
		if p.IdentityUncertain {
			uncertainPacks++
		}

		// Evaluate alarm status from decoded protection info
		if !p.Frame.Protection.AlarmsClear {
			allAlarmsClear = false
		}

		// Evaluate settings status
		if p.Settings != nil {
			if p.Settings.Fresh && p.Settings.Status == interpack.SettingsFresh {
				settingsFreshPacks++
			} else if p.Settings.Status == interpack.SettingsStale {
				settingsStalePacks++
			}
		}
	}

	observed := totalPacks > 0
	status := "disabled"
	if configured {
		switch {
		case !observed:
			status = "unseen"
		case stalePacks > 0:
			status = "stale"
		case unconfirmedPacks > 0:
			status = "unconfirmed"
		default:
			status = "fresh"
		}
	}

	// Never claim battery health or safety from raw unverified status/alarm bytes or address counts.
	// all_observed_fresh only reports that live telemetry frames have been recently observed
	// and confirmed across all seen pack addresses.
	allObservedFresh := status == "fresh"
	allObservedSettingsFresh := totalPacks > 0 && settingsFreshPacks == totalPacks
	allObservedAlarmsClear := totalPacks > 0 && allAlarmsClear

	resp := BMSResponse{
		Configured:               true,
		SerialDevice:             device,
		Observed:                 observed,
		Status:                   status,
		AllObservedFresh:         allObservedFresh,
		FreshnessSeconds:         freshnessSec,
		FreshnessDuration:        freshnessStr,
		TotalPacks:               totalPacks,
		FreshPacks:               freshPacks,
		StalePacks:               stalePacks,
		UnconfirmedPacks:         unconfirmedPacks,
		UncertainPacks:           uncertainPacks,
		AllObservedSettingsFresh: allObservedSettingsFresh,
		AllObservedAlarmsClear:   allObservedAlarmsClear,
		SettingsFreshPacks:       settingsFreshPacks,
		SettingsStalePacks:       settingsStalePacks,
		Packs:                    packs,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// walkJSON traverses a dot-separated path in a JSON map.
func walkJSON(m map[string]any, path string) any {
	if m == nil {
		return nil
	}
	parts := strings.Split(path, ".")
	var current any = m
	for _, p := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = cm[p]
		if !ok {
			return nil
		}
	}
	return current
}

// faultCodeName wraps register.FaultCodeName for templates.
func faultCodeName(code uint16) string {
	return register.FaultCodeName(code)
}

func init() {
	templateFuncs["faultCodeName"] = faultCodeName
}
