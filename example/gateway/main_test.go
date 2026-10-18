package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rg "rustygo"
)

func TestGatewayPipeline(t *testing.T) {
	gw := NewGatewayServer()
	arena := rg.NewArena(64 * 1024)
	defer arena.Close()

	handler := rg.HTTPMiddleware(arena)(http.HandlerFunc(gw.ProxyHandler))

	// 1. Initial Request (Cache Miss)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(`{"action":"list","limit":50}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Cache") != "MISS" {
		t.Fatalf("expected MISS, got %s", rec.Header().Get("X-Cache"))
	}
	if !strings.Contains(rec.Body.String(), "RustyGo v1.0.0") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}

	// 2. Second Request (Cache Hit - Zero GC)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Cache") != "HIT-ZERO-GC" {
		t.Fatalf("expected HIT-ZERO-GC, got %s", rec2.Header().Get("X-Cache"))
	}
}
