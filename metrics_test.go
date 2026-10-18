package rustygo_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rg "rustygo"
)

func TestMetricsTrackingAndPrometheus(t *testing.T) {
	initialSnap := rg.GlobalMetrics.Snapshot()

	arena := rg.NewArena(64 * 1024)
	snapAfterNew := rg.GlobalMetrics.Snapshot()
	if snapAfterNew.ActiveArenas != initialSnap.ActiveArenas+1 {
		t.Fatalf("expected active arenas increment")
	}

	scope := arena.EnterScope()
	buf := rg.AllocSlice[byte](scope, 1024)
	buf[0] = 1

	snapInScope := rg.GlobalMetrics.Snapshot()
	if snapInScope.ActiveScopes != initialSnap.ActiveScopes+1 {
		t.Fatalf("expected active scopes increment")
	}

	scope.Exit()
	snapAfterExit := rg.GlobalMetrics.Snapshot()
	if snapAfterExit.ActiveScopes != initialSnap.ActiveScopes {
		t.Fatalf("expected active scopes back to baseline")
	}
	if snapAfterExit.GCBypassedBytesTotal <= initialSnap.GCBypassedBytesTotal {
		t.Fatalf("expected GC bypassed bytes to increase")
	}

	_ = arena.Close()
	snapAfterClose := rg.GlobalMetrics.Snapshot()
	if snapAfterClose.ActiveArenas != initialSnap.ActiveArenas {
		t.Fatalf("expected active arenas back to baseline")
	}

	// Test HTTP Handler
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	rg.MetricsHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, expectedMetric := range []string{
		"rustygo_arena_allocated_bytes",
		"rustygo_arena_active_count",
		"rustygo_scope_active_count",
		"rustygo_arena_high_water_bytes",
		"rustygo_arena_gc_bypassed_bytes_total",
	} {
		if !strings.Contains(body, expectedMetric) {
			t.Fatalf("prometheus body missing metric %q", expectedMetric)
		}
	}
}
