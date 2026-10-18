package rustygo_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rg "rustygo"
)

func TestReadResponseBodyZeroCopy(t *testing.T) {
	expectedPayload := strings.Repeat("zero-copy-payload-chunk-", 500) // ~12KB
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, expectedPayload)
	}))
	defer ts.Close()

	arena := rg.NewArena(64 * 1024)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	body, err := rg.ReadResponseBody(resp, scope, 1024*1024)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	if string(body) != expectedPayload {
		t.Fatalf("payload mismatch: len(got)=%d len(want)=%d", len(body), len(expectedPayload))
	}
}

func TestClientGetScope(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer ts.Close()

	arena := rg.NewArena(16 * 1024)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	body, resp, err := rg.DefaultClient.GetScope(context.Background(), ts.URL, scope)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("expected {\"status\":\"ok\"}, got %q", string(body))
	}
}
