package rustygo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DebugDashboardData structures the snapshot for debugging inspection.
type DebugDashboardData struct {
	Metrics   MetricsSnapshot           `json:"metrics"`
	AutoTuner map[string]int            `json:"autotuner_calibrations"`
}

// DebugHandler renders an interactive HTML inspection dashboard or JSON snapshot.
func DebugHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := GlobalMetrics.Snapshot()

		// Read auto-tuner recommendations
		GlobalAutoTuner.mu.RLock()
		calibrations := make(map[string]int, len(GlobalAutoTuner.sites))
		for k, v := range GlobalAutoTuner.sites {
			calibrations[k] = v.estimatedBytes
		}
		GlobalAutoTuner.mu.RUnlock()

		data := DebugDashboardData{
			Metrics:   snap,
			AutoTuner: calibrations,
		}

		if r.URL.Query().Get("format") == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(data)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>RustyGo Runtime Arena Inspector</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; margin: 0; padding: 2rem; }
		.container { max-width: 900px; margin: 0 auto; }
		h1 { color: #38bdf8; display: flex; align-items: center; gap: 0.5rem; }
		.card-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin: 1.5rem 0; }
		.card { background: #1e293b; padding: 1.25rem; border-radius: 8px; border: 1px solid #334155; }
		.card-val { font-size: 1.75rem; font-weight: bold; color: #f1f5f9; margin-top: 0.25rem; }
		.card-lbl { color: #94a3b8; font-size: 0.85rem; text-transform: uppercase; letter-spacing: 0.05em; }
		table { width: 100%%; border-collapse: collapse; margin-top: 1rem; background: #1e293b; border-radius: 8px; overflow: hidden; }
		th, td { text-align: left; padding: 0.75rem 1rem; border-bottom: 1px solid #334155; }
		th { background: #0f172a; color: #94a3b8; font-size: 0.85rem; text-transform: uppercase; }
		.badge { background: #0284c7; color: white; padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.75rem; }
	</style>
</head>
<body>
	<div class="container">
		<h1>⚡ RustyGo Runtime Arena Inspector <span class="badge">v0.6.0</span></h1>
		
		<div class="card-grid">
			<div class="card">
				<div class="card-lbl">Allocated Slab Memory</div>
				<div class="card-val">%s</div>
			</div>
			<div class="card">
				<div class="card-lbl">Active Arenas</div>
				<div class="card-val">%d</div>
			</div>
			<div class="card">
				<div class="card-lbl">Active Scopes</div>
				<div class="card-val">%d</div>
			</div>
			<div class="card">
				<div class="card-lbl">GC Bypassed Total</div>
				<div class="card-val">%s</div>
			</div>
		</div>

		<h2>🎯 Calibrated Auto-Tuner Call Sites (%d)</h2>
		<table>
			<thead>
				<tr>
					<th>Call Site Identifier</th>
					<th>Learned Memory Demand</th>
					<th>Optimal Initial Slab</th>
				</tr>
			</thead>
			<tbody>
`,
			formatBytes(snap.AllocatedBytes),
			snap.ActiveArenas,
			snap.ActiveScopes,
			formatBytes(int64(snap.GCBypassedBytesTotal)),
			len(calibrations),
		)

		if len(calibrations) == 0 {
			html += `<tr><td colspan="3" style="text-align: center; color: #94a3b8;">No call sites calibrated yet. Run workloads with WithAutoTunedScope.</td></tr>`
		} else {
			for site, size := range calibrations {
				rec := GlobalAutoTuner.RecommendedSize(site)
				html += fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td><strong>%s</strong></td></tr>`, site, formatBytes(int64(size)), formatBytes(int64(rec)))
			}
		}

		html += `
			</tbody>
		</table>
	</div>
</body>
</html>`
		_, _ = w.Write([]byte(html))
	})
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// RegisterDebugHandler registers the /debug/rustygo/arenas endpoint on mux.
func RegisterDebugHandler(mux *http.ServeMux) {
	mux.Handle("/debug/rustygo/arenas", DebugHandler())
}
