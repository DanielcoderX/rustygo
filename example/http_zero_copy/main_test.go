package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	rg "rustygo"
)

func TestUserHandler(t *testing.T) {
	arena := rg.NewArena(64 * 1024)
	defer arena.Close()

	server := SetupServer(arena)

	payload := []byte(`{"user":"DanielCoder","role":"engineer"}`)
	req := httptest.NewRequest(http.MethodPost, "/user", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	expected := `{"status":"ok","welcome":"DanielCoder"}`
	if rec.Body.String() != expected {
		t.Fatalf("expected response %s, got %s", expected, rec.Body.String())
	}
}

func BenchmarkUserHandler(b *testing.B) {
	arena := rg.NewArena(1024 * 1024)
	defer arena.Close()

	server := SetupServer(arena)
	payload := []byte(`{"user":"DanielCoder","role":"engineer"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/user", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
	}
}
