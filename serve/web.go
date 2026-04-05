package serve

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// WebServer handles HTTP routes for the dashboard and API.
type WebServer struct {
	hub    *Hub
	system *inverter.System
	tmpl   *template.Template
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

	// REST API
	mux.HandleFunc("GET /api/snapshot", ws.handleAPISnapshot)
	mux.HandleFunc("GET /api/settings", ws.handleAPISettings)
	mux.HandleFunc("GET /api/faults", ws.handleAPIFaults)

	// SSE stream
	mux.HandleFunc("GET /api/snapshot/stream", ws.handleSSE)

	// Static files
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	return mux
}

func (ws *WebServer) renderPage(w http.ResponseWriter, data pageData) {
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

	faults, err := ws.system.ReadFaults(ctx)
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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	faults, err := ws.system.ReadFaults(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(faults)
}

// faultCodeName wraps register.FaultCodeName for templates.
func faultCodeName(code uint16) string {
	return register.FaultCodeName(code)
}

func init() {
	templateFuncs["faultCodeName"] = faultCodeName
}
