package rustygo_test

import (
	"bytes"
	"strings"
	"testing"

	rg "rustygo"
)

func TestArenaBufferBasicAndGrowth(t *testing.T) {
	arena := rg.NewArena(16 * 1024)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	buf := rg.NewArenaBuffer(scope, 8)
	if buf.Len() != 0 || buf.Cap() != 8 {
		t.Fatalf("unexpected initial len/cap: %d/%d", buf.Len(), buf.Cap())
	}

	_, _ = buf.WriteString("Hello, ")
	_, _ = buf.WriteString("RustyGo ")
	buf.WriteInt(2026)
	_ = buf.WriteByte('!')

	expected := "Hello, RustyGo 2026!"
	if buf.String() != expected {
		t.Fatalf("expected %q, got %q", expected, buf.String())
	}
	if !bytes.Equal(buf.Bytes(), []byte(expected)) {
		t.Fatalf("expected bytes match")
	}

	// Test growth beyond initial capacity
	longSuffix := strings.Repeat("-vector-fast-path", 50)
	_, _ = buf.WriteString(longSuffix)
	if !strings.HasSuffix(buf.String(), longSuffix) {
		t.Fatalf("failed large buffer growth")
	}

	// Test Reset
	buf.Reset()
	if buf.Len() != 0 {
		t.Fatalf("expected len 0 after reset, got %d", buf.Len())
	}
	_, _ = buf.WriteString("Reused")
	if buf.String() != "Reused" {
		t.Fatalf("expected Reused, got %q", buf.String())
	}
}

func TestArenaBufferRuneAndTruncate(t *testing.T) {
	arena := rg.NewArena(4096)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	buf := rg.NewArenaBuffer(scope, 16)
	_, _ = buf.WriteRune('🚀')
	_, _ = buf.WriteRune('⚡')

	if buf.Len() != 7 { // 4 bytes for 🚀, 3 bytes for ⚡
		t.Fatalf("expected len 7, got %d", buf.Len())
	}

	buf.Truncate(4)
	if buf.String() != "🚀" {
		t.Fatalf("expected 🚀 after truncate, got %q", buf.String())
	}
}
