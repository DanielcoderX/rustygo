package rustygo_test

import (
	"errors"
	rg "rustygo"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func TestArenaTryAllocAndAllocOrErr(t *testing.T) {
	arena := rg.NewArena(8)

	if _, ok := arena.TryAlloc(0); ok {
		t.Fatal("TryAlloc(0) should fail")
	}

	if _, err := arena.AllocOrErr(0); !errors.Is(err, rg.ErrInvalidAllocSize) {
		t.Fatalf("expected ErrInvalidAllocSize, got %v", err)
	}

	if buf, ok := arena.TryAlloc(4); !ok || len(buf) != 4 {
		t.Fatalf("expected successful TryAlloc(4), got ok=%v len=%d", ok, len(buf))
	}

	if _, ok := arena.TryAlloc(5); ok {
		t.Fatal("TryAlloc should fail when arena is out of memory")
	}

	if _, err := arena.AllocOrErr(5); !errors.Is(err, rg.ErrArenaOutOfMemory) {
		t.Fatalf("expected ErrArenaOutOfMemory, got %v", err)
	}
}

func TestScopeAllocRejectsNonPositive(t *testing.T) {
	scope := rg.NewArena(16).EnterScope()
	defer scope.Exit()

	assertPanicsWith(t, "Alloc size must be > 0", func() {
		_ = scope.Alloc(0)
	})
}

func TestScopeExitOutOfOrderDoesNotMoveOffsetBackward(t *testing.T) {
	arena := rg.NewArena(64)

	s1 := arena.EnterScope()
	if _, err := s1.AllocOrErr(16); err != nil {
		t.Fatal(err)
	}

	s2 := arena.EnterScope()
	if _, err := s2.AllocOrErr(16); err != nil {
		t.Fatal(err)
	}

	s2.Exit()
	used, _ := arena.Stats()
	if used != 32 {
		t.Fatalf("expected used=32 after s2 exit, got %d", used)
	}

	s1.Exit()
	used, _ = arena.Stats()
	if used != 32 {
		t.Fatalf("expected used=32 after out-of-order exit, got %d", used)
	}
}

func TestScopeConcurrentAllocNoOverlap(t *testing.T) {
	const (
		workers   = 16
		perWorker = 1024
		allocSize = 8
	)

	totalSlots := workers * perWorker
	arena := rg.NewArena(totalSlots * allocSize)
	var seen sync.Map
	errCh := make(chan string, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope := arena.EnterScope()
			defer scope.Exit()

			for i := 0; i < perWorker; i++ {
				buf := scope.Alloc(allocSize)
				if buf == nil {
					errCh <- "unexpected nil allocation"
					return
				}
				addr := uintptr(unsafe.Pointer(&buf[0]))
				if _, loaded := seen.LoadOrStore(addr, struct{}{}); loaded {
					errCh <- "duplicate address detected"
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Fatal(msg)
	}

	used, _ := arena.Stats()
	if used != totalSlots*allocSize {
		t.Fatalf("expected used=%d got %d", totalSlots*allocSize, used)
	}
}

func TestWithScopeEnforcesExit(t *testing.T) {
	arena := rg.NewArena(32)
	var scopeRef interface{ Active() bool }
	if err := arena.WithScope(func(s *rg.Scope) error {
		scopeRef = s
		_, err := s.AllocOrErr(8)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if scopeRef == nil {
		t.Fatal("expected scope reference to be set")
	}
	if scopeRef.Active() {
		t.Fatal("scope should be inactive after WithScope returns")
	}
}

func TestWithScopeAllowsManualExit(t *testing.T) {
	arena := rg.NewArena(16)
	if err := arena.WithScope(func(s *rg.Scope) error {
		s.Exit()
		if s.Active() {
			t.Fatal("scope should be inactive after manual exit")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestScopeUsedBytes(t *testing.T) {
	s := rg.NewArena(64).EnterScope()
	defer s.Exit()

	if s.UsedBytes() != 0 {
		t.Fatalf("expected initial UsedBytes=0, got %d", s.UsedBytes())
	}
	if _, err := s.AllocOrErr(12); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AllocOrErr(20); err != nil {
		t.Fatal(err)
	}
	if got := s.UsedBytes(); got != 32 {
		t.Fatalf("expected UsedBytes=32, got %d", got)
	}
}

func TestArenaMarkAndRewind(t *testing.T) {
	arena := rg.NewArena(64)
	if _, err := arena.AllocOrErr(16); err != nil {
		t.Fatal(err)
	}
	mark := arena.Mark()
	if _, err := arena.AllocOrErr(16); err != nil {
		t.Fatal(err)
	}
	if err := arena.Rewind(mark); err != nil {
		t.Fatalf("rewind failed: %v", err)
	}
	used, _ := arena.Stats()
	if used != 16 {
		t.Fatalf("expected used=16 after rewind, got %d", used)
	}
}

func TestArenaRewindRejectsForwardMark(t *testing.T) {
	arena := rg.NewArena(32)
	if _, err := arena.AllocOrErr(8); err != nil {
		t.Fatal(err)
	}
	if err := arena.Rewind(rg.ArenaMark(16)); !errors.Is(err, rg.ErrRewindForward) {
		t.Fatalf("expected ErrRewindForward, got %v", err)
	}
}

func TestArenaAllocAligned(t *testing.T) {
	arena := rg.NewArena(128)
	buf, err := arena.AllocAlignedOrErr(16, 16)
	if err != nil {
		t.Fatal(err)
	}
	addr := uintptr(unsafe.Pointer(&buf[0]))
	if addr%16 != 0 {
		t.Fatalf("expected 16-byte alignment, address=%d", addr)
	}
	if _, err := arena.AllocAlignedOrErr(8, 3); !errors.Is(err, rg.ErrInvalidAlignment) {
		t.Fatalf("expected ErrInvalidAlignment, got %v", err)
	}
}

func assertPanicsWith(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic %q, but function did not panic", want)
		}
		got, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T", r)
		}
		if !strings.Contains(got, want) {
			t.Fatalf("panic mismatch: want contains %q, got %q", want, got)
		}
	}()
	fn()
}
