package rustygo_test

import (
	rg "rustygo"
	"testing"
)

func TestLocalArenaBasicAndGeometricGrowth(t *testing.T) {
	la := rg.NewLocalArena(64)
	defer la.Close()

	if la.Capacity() != 64 {
		t.Fatalf("expected initial cap 64, got %d", la.Capacity())
	}

	buf1 := la.Alloc(48)
	if len(buf1) != 48 {
		t.Fatalf("expected len 48, got %d", len(buf1))
	}

	// This allocation exceeds remaining 16 bytes in chunk 0, triggering geometric doubling
	buf2 := la.Alloc(48)
	if len(buf2) != 48 {
		t.Fatalf("expected len 48, got %d", len(buf2))
	}

	if la.Capacity() < 112 {
		t.Fatalf("expected capacity >= 112, got %d", la.Capacity())
	}

	used, cap := la.Stats()
	if used != 96 {
		t.Fatalf("expected used 96, got %d", used)
	}
	if cap < 112 {
		t.Fatalf("expected capacity >= 112, got %d", cap)
	}

	la.Reset()
	usedAfter, capAfter := la.Stats()
	if usedAfter != 0 {
		t.Fatalf("expected used 0 after Reset, got %d", usedAfter)
	}
	if capAfter != cap {
		t.Fatalf("expected cap retained after Reset, got %d vs %d", capAfter, cap)
	}

	// Allocating after Reset reuses existing slabs
	buf3 := la.Alloc(48)
	buf4 := la.Alloc(48)
	if len(buf3) != 48 || len(buf4) != 48 {
		t.Fatal("unexpected allocation failure after Reset")
	}
	if la.Capacity() != cap {
		t.Fatalf("expected no new chunk allocation, got cap %d vs %d", la.Capacity(), cap)
	}
}

func TestLocalArenaAllocAligned(t *testing.T) {
	la := rg.NewLocalArena(128)
	defer la.Close()

	_ = la.Alloc(3)
	aligned := la.AllocAligned(16, 16)
	if len(aligned) != 16 {
		t.Fatalf("expected len 16, got %d", len(aligned))
	}
	addr := uintptr(len(aligned))
	if addr%16 != 0 {
		t.Fatalf("expected 16-byte alignment, got %d", addr)
	}
}

func TestScopeMemoryPoisoningOnExit(t *testing.T) {
	arena := rg.NewArena(128, rg.WithPoisonOnScopeExit(0xDE))
	defer arena.Close()

	scope := arena.EnterScope()
	buf := scope.Alloc(32)
	for i := range buf {
		buf[i] = 0x42
	}

	// Exit scope: all allocated bytes must be poisoned with 0xDE
	scope.Exit()

	for i, b := range buf {
		if b != 0xDE {
			t.Fatalf("byte at %d not poisoned: expected 0xDE, got 0x%02X", i, b)
		}
	}
}

func BenchmarkLocalArenaAlloc(b *testing.B) {
	la := rg.NewLocalArena(1024 * 1024)
	defer la.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = la.Alloc(64)
		if i%10000 == 0 {
			la.Reset()
		}
	}
}

func BenchmarkArenaAllocCAS(b *testing.B) {
	arena := rg.NewArena(1024 * 1024)
	defer arena.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = arena.Alloc(64)
		if i%10000 == 0 {
			arena.Reset()
		}
	}
}
