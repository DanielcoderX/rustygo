package rustygo

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// MetricsTracker records real-time allocation and GC bypass telemetry.
type MetricsTracker struct {
	allocatedBytes       atomic.Int64
	usedBytes            atomic.Int64
	activeArenas         atomic.Int64
	activeScopes         atomic.Int64
	highWaterMarkBytes   atomic.Int64
	gcBypassedBytesTotal atomic.Uint64
}

// GlobalMetrics is the singleton metrics tracker for the process.
var GlobalMetrics = &MetricsTracker{}

// RecordArenaCreated tracks a new arena allocation.
func (m *MetricsTracker) RecordArenaCreated(slabBytes int) {
	m.activeArenas.Add(1)
	alloc := m.allocatedBytes.Add(int64(slabBytes))
	m.updateHighWater(alloc)
}

// RecordArenaClosed tracks arena destruction.
func (m *MetricsTracker) RecordArenaClosed(slabBytes int) {
	m.activeArenas.Add(-1)
	m.allocatedBytes.Add(-int64(slabBytes))
}

// RecordScopeEnter tracks scope creation.
func (m *MetricsTracker) RecordScopeEnter() {
	m.activeScopes.Add(1)
}

// RecordScopeExit tracks scope destruction and used memory offset.
func (m *MetricsTracker) RecordScopeExit(usedBytes int) {
	m.activeScopes.Add(-1)
	if usedBytes > 0 {
		m.gcBypassedBytesTotal.Add(uint64(usedBytes))
	}
}

func (m *MetricsTracker) updateHighWater(current int64) {
	for {
		old := m.highWaterMarkBytes.Load()
		if current <= old {
			break
		}
		if m.highWaterMarkBytes.CompareAndSwap(old, current) {
			break
		}
	}
}

// Stats returns a snapshot of current memory metrics.
type MetricsSnapshot struct {
	AllocatedBytes       int64  `json:"allocated_bytes"`
	ActiveArenas         int64  `json:"active_arenas"`
	ActiveScopes         int64  `json:"active_scopes"`
	HighWaterMarkBytes   int64  `json:"high_water_mark_bytes"`
	GCBypassedBytesTotal uint64 `json:"gc_bypassed_bytes_total"`
}

// Snapshot returns current metrics copy.
func (m *MetricsTracker) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		AllocatedBytes:       m.allocatedBytes.Load(),
		ActiveArenas:         m.activeArenas.Load(),
		ActiveScopes:         m.activeScopes.Load(),
		HighWaterMarkBytes:   m.highWaterMarkBytes.Load(),
		GCBypassedBytesTotal: m.gcBypassedBytesTotal.Load(),
	}
}

// PrometheusText renders metrics in Prometheus text exposition format.
func (m *MetricsTracker) PrometheusText() string {
	snap := m.Snapshot()
	return fmt.Sprintf(`# HELP rustygo_arena_allocated_bytes Current memory in bytes allocated across all active arenas.
# TYPE rustygo_arena_allocated_bytes gauge
rustygo_arena_allocated_bytes %d

# HELP rustygo_arena_active_count Number of currently live arena instances.
# TYPE rustygo_arena_active_count gauge
rustygo_arena_active_count %d

# HELP rustygo_scope_active_count Number of currently active lexical scopes.
# TYPE rustygo_scope_active_count gauge
rustygo_scope_active_count %d

# HELP rustygo_arena_high_water_bytes Highest allocated bytes observed in the process.
# TYPE rustygo_arena_high_water_bytes gauge
rustygo_arena_high_water_bytes %d

# HELP rustygo_arena_gc_bypassed_bytes_total Cumulative bytes allocated in arenas bypassing Go runtime GC.
# TYPE rustygo_arena_gc_bypassed_bytes_total counter
rustygo_arena_gc_bypassed_bytes_total %d
`, snap.AllocatedBytes, snap.ActiveArenas, snap.ActiveScopes, snap.HighWaterMarkBytes, snap.GCBypassedBytesTotal)
}

// MetricsHandler returns an HTTP handler serving Prometheus metrics.
func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(GlobalMetrics.PrometheusText()))
	})
}
