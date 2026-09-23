package codec_test

import (
	"encoding/json"
	"io"
	"testing"

	rg "rustygo"
	"rustygo/codec"
)

func TestJSONScanner(t *testing.T) {
	raw := []byte(`{
		"user": "Alice",
		"id": 1024,
		"active": true,
		"tags": ["go", "arena", "simd"]
	}`)

	arena := rg.NewArena(4096)
	defer arena.Close()

	scope := arena.EnterScope()
	defer scope.Exit()

	scanner := codec.NewJSONScanner(raw)

	tokens := make(map[string]string)
	for {
		tokType, val, err := scanner.Next(scope)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("scanner error: %v", err)
		}
		if tokType == codec.JSONTokenKey {
			_, nextVal, err := scanner.Next(scope)
			if err != nil {
				t.Fatal(err)
			}
			tokens[val] = nextVal
		}
	}

	if tokens["user"] != "Alice" {
		t.Fatalf("expected user=Alice, got %s", tokens["user"])
	}
	if tokens["id"] != "1024" {
		t.Fatalf("expected id=1024, got %s", tokens["id"])
	}
	if tokens["active"] != "true" {
		t.Fatalf("expected active=true, got %s", tokens["active"])
	}
}

func BenchmarkJSONStandard(b *testing.B) {
	raw := []byte(`{"user":"Alice","id":1024,"active":true,"city":"Tehran"}`)
	type Payload struct {
		User   string `json:"user"`
		ID     int    `json:"id"`
		Active bool   `json:"active"`
		City   string `json:"city"`
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var p Payload
		_ = json.Unmarshal(raw, &p)
	}
}

func BenchmarkJSONScannerArena(b *testing.B) {
	raw := []byte(`{"user":"Alice","id":1024,"active":true,"city":"Tehran"}`)
	arena := rg.NewArena(64 * 1024)
	defer arena.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scope := arena.EnterScope()
		scanner := codec.NewJSONScanner(raw)
		for {
			_, _, err := scanner.Next(scope)
			if err != nil {
				break
			}
		}
		scope.Exit()
		if i%1000 == 0 {
			arena.Reset()
		}
	}
}
