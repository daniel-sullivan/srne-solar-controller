package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/daniel-sullivan/srne-solar-controller/inverter"
	"github.com/daniel-sullivan/srne-solar-controller/register"
)

// partialNames lists the template names emitted as SSE events.
var partialNames = []string{"battery", "pv", "load", "grid", "inverter", "units"}

// pageData is the top-level data for full page renders (layout + content).
type pageData struct {
	Page     string
	Snapshot *inverter.Snapshot
	Settings *inverter.Settings
	Faults   []register.FaultRecord
	Changed  map[string]bool
}

// TemplateData returns a templateData for use by dashboard partials.
func (p pageData) TemplateData() templateData {
	return templateData{Snapshot: p.Snapshot, Changed: p.Changed}
}

// templateData is passed to SSE partial templates.
type templateData struct {
	*inverter.Snapshot
	Changed map[string]bool
}

// VC returns "value-changed" if the named key changed, empty string otherwise.
// Used in templates as: class="{{vc .Changed "bat-soc"}}"
func vc(changed map[string]bool, key string) string {
	if changed[key] {
		return "value-changed"
	}
	return ""
}

func (ws *WebServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	remoteAddr := r.RemoteAddr
	slog.Info("sse client connected", "remote", remoteAddr)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx
	flusher.Flush()

	sub := ws.hub.Subscribe()
	defer func() {
		ws.hub.Unsubscribe(sub)
		slog.Info("sse client disconnected", "remote", remoteAddr)
	}()

	var prev map[string]string // previous rendered values per key
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-sub.C:
			if !ok {
				return
			}

			cur := snapshotValues(snap)
			changed := diffValues(prev, cur)
			prev = cur

			data := templateData{Snapshot: snap, Changed: changed}

			// Emit HTML partial events for HTMX sse-swap
			for _, name := range partialNames {
				var buf bytes.Buffer
				if err := ws.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
					continue
				}
				writeSSEEvent(w, name, buf.String())
			}

			// Emit raw JSON event for API consumers
			jsonBytes, err := json.Marshal(snap)
			if err == nil {
				writeSSEEvent(w, "snapshot", string(jsonBytes))
			}

			flusher.Flush()
		}
	}
}

// snapshotValues extracts all trackable values as formatted strings, keyed by data-v name.
func snapshotValues(s *inverter.Snapshot) map[string]string {
	if s == nil {
		return nil
	}
	v := map[string]string{
		// Battery
		"bat-soc":    fmt.Sprintf("%.0f", s.Battery.SOC),
		"bat-v":      fmt.Sprintf("%.1f", s.Battery.Voltage),
		"bat-i":      fmt.Sprintf("%.1f", s.Battery.Current),
		"bat-w":      fmt.Sprintf("%.0f", s.Battery.TotalChargePower),
		"bat-status": s.Battery.ChargeStatusName,
		"bat-temp":   fmt.Sprintf("%.0f", s.Battery.BatteryTemp),
		"ctrl-temp":  fmt.Sprintf("%.0f", s.Battery.ControllerTemp),
		// PV
		"pv-total": fmt.Sprintf("%.0f", s.PV.TotalPower),
		"pv1-w":    fmt.Sprintf("%.0f", s.PV.PV1Power),
		"pv1-vi":   fmt.Sprintf("%.1f/%.1f", s.PV.PV1Voltage, s.PV.PV1Current),
		"pv2-w":    fmt.Sprintf("%.0f", s.PV.PV2Power),
		"pv2-vi":   fmt.Sprintf("%.1f/%.1f", s.PV.PV2Voltage, s.PV.PV2Current),
		// Load
		"load-total": fmt.Sprintf("%.0f", s.Load.TotalPower),
		"load-w":     fmt.Sprintf("%.0f", s.Load.TotalPower),
		"load-va":    fmt.Sprintf("%.0f", s.Load.TotalApparentPower),
		"load-pf":    fmt.Sprintf("%.2f", s.Load.PowerFactor),
		"load-dc":    fmt.Sprintf("%.0f", s.Load.DCPower),
		// Grid / AC output
		"grid-l1-v":   fmt.Sprintf("%.1f", s.Grid.L1.GridVoltage),
		"grid-l1-w":   fmt.Sprintf("%.0f", s.Load.L1.Power),
		"grid-l1-va":  fmt.Sprintf("%.0f/%.1f", s.Load.L1.ApparentPower, s.Load.L1.Current),
		"grid-l1-pct": fmt.Sprintf("%.0f", s.Load.L1.Ratio),
		"grid-l2-v":   fmt.Sprintf("%.1f", s.Grid.L2.GridVoltage),
		"grid-l2-w":   fmt.Sprintf("%.0f", s.Load.L2.Power),
		"grid-l2-va":  fmt.Sprintf("%.0f/%.1f", s.Load.L2.ApparentPower, s.Load.L2.Current),
		"grid-l2-pct": fmt.Sprintf("%.0f", s.Load.L2.Ratio),
		"grid-l3-i":   fmt.Sprintf("%.1f", s.Load.L3.Current),
		"grid-freq":   fmt.Sprintf("%.2f", s.Grid.Frequency),
		"grid-charge": fmt.Sprintf("%.1f", s.Grid.MainsChargeCurr),
		"inv-l1-v":    fmt.Sprintf("%.1f", s.Grid.L1.InverterVoltage),
		"inv-l2-v":    fmt.Sprintf("%.1f", s.Grid.L2.InverterVoltage),
		"inv-l3-v":    fmt.Sprintf("%.1f", s.Grid.L3.InverterVoltage),
		// Inverter
		"inv-state": s.Inverter.MachineStateName,
		"inv-bus":   fmt.Sprintf("%.1f", s.Inverter.BusVoltage),
		"inv-freq":  fmt.Sprintf("%.2f", s.Inverter.Frequency),
		"inv-ta":    fmt.Sprintf("%.1f", s.Inverter.HeatsinkTempA),
		"inv-tb":    fmt.Sprintf("%.1f", s.Inverter.HeatsinkTempB),
	}
	// Per-unit values
	for _, u := range s.Units {
		v[fmt.Sprintf("unit-pv-%s", u.Host)] = fmt.Sprintf("%.0f", u.PV.TotalPower)
		v[fmt.Sprintf("unit-load-%s", u.Host)] = fmt.Sprintf("%.0f", u.Load.TotalPower)
		v[fmt.Sprintf("unit-pv1-w-%s", u.Host)] = fmt.Sprintf("%.0f", u.PV.PV1Power)
		v[fmt.Sprintf("unit-pv1-vi-%s", u.Host)] = fmt.Sprintf("%.1f/%.1f", u.PV.PV1Voltage, u.PV.PV1Current)
		v[fmt.Sprintf("unit-pv2-w-%s", u.Host)] = fmt.Sprintf("%.0f", u.PV.PV2Power)
		v[fmt.Sprintf("unit-pv2-vi-%s", u.Host)] = fmt.Sprintf("%.1f/%.1f", u.PV.PV2Voltage, u.PV.PV2Current)
	}
	return v
}

// diffValues returns keys that changed between prev and cur.
func diffValues(prev, cur map[string]string) map[string]bool {
	changed := make(map[string]bool)
	if prev == nil {
		return changed // first snapshot — nothing to highlight
	}
	for k, newVal := range cur {
		if oldVal, ok := prev[k]; ok && oldVal != newVal {
			changed[k] = true
		}
	}
	return changed
}

// writeSSEEvent writes a single SSE event. Multi-line data is split per spec.
// Write errors mean the client disconnected; nothing to handle.
func writeSSEEvent(w http.ResponseWriter, event, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	for _, line := range strings.Split(data, "\n") {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
	_, _ = fmt.Fprint(w, "\n")
}
