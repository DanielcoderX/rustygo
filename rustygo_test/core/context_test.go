package rustygo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	rg "rustygo"
)

func TestContextArenaAndScope(t *testing.T) {
	arena := rg.NewArena(1024)
	defer arena.Close()

	ctx := context.Background()
	ctx = rg.WithContextArena(ctx, arena)

	retrieved, ok := rg.FromContext(ctx)
	if !ok || retrieved != arena {
		t.Fatal("failed to retrieve arena from context")
	}

	scope := arena.EnterScope()
	ctxWithScope, cleanup := rg.WithContextScope(ctx, scope)

	s, ok := rg.ScopeFromContext(ctxWithScope)
	if !ok || s != scope {
		t.Fatal("failed to retrieve scope from context")
	}

	_ = s.Alloc(32)
	if s.UsedBytes() != 32 {
		t.Fatalf("expected used 32, got %d", s.UsedBytes())
	}

	cleanup()
	if s.Active() {
		t.Fatal("scope should be inactive after cleanup")
	}
}

func TestHTTPMiddlewareScopeLifecycle(t *testing.T) {
	arena := rg.NewArena(4096)
	defer arena.Close()

	var handled bool
	var allocatedAddr uintptr

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := rg.ScopeFromContext(r.Context())
		if !ok || !s.Active() {
			t.Fatal("expected active scope in request context")
		}

		buf := s.Alloc(64)
		if len(buf) != 64 {
			t.Fatal("failed allocation inside HTTP handler")
		}
		allocatedAddr = uintptr(len(buf))
		handled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := rg.HTTPMiddleware(arena)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !handled || rec.Code != http.StatusOK {
		t.Fatalf("handler failed: handled=%v code=%d", handled, rec.Code)
	}
	if allocatedAddr == 0 {
		t.Fatal("allocation did not happen")
	}
}
