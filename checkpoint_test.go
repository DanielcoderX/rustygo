package rustygo_test

import (
	"errors"
	"testing"

	rg "rustygo"
)

func TestArenaSavepointAndRollback(t *testing.T) {
	arena := rg.NewArena(4096)
	defer arena.Close()

	// Initial allocation
	b1 := arena.Alloc(256)
	b1[0] = 0x11

	sp := arena.Savepoint()

	// Subsequent allocations that we want to rollback
	b2 := arena.Alloc(512)
	b2[0] = 0x22
	b3 := arena.Alloc(1024)
	b3[0] = 0x33

	// Perform rollback
	if err := arena.Rollback(sp); err != nil {
		t.Fatalf("unexpected rollback error: %v", err)
	}

	// Next allocation should reuse the memory location of b2
	b4 := arena.Alloc(512)
	b4[0] = 0x44

	if &b2[0] != &b4[0] {
		t.Fatalf("expected b4 to reuse rolled-back address: b2=%p, b4=%p", &b2[0], &b4[0])
	}
}

func TestArenaTransactionRollbackOnError(t *testing.T) {
	arena := rg.NewArena(4096)
	defer arena.Close()

	_ = arena.Alloc(128)
	var bAborted []byte

	customErr := errors.New("transaction abort")
	err := arena.Transaction(func(a *rg.Arena) error {
		bAborted = a.Alloc(2048)
		return customErr
	})

	if !errors.Is(err, customErr) {
		t.Fatalf("expected customErr, got %v", err)
	}

	// New allocation should be at the rolled back offset
	bReused := arena.Alloc(2048)
	if &bAborted[0] != &bReused[0] {
		t.Fatalf("expected memory reuse after rolled-back transaction")
	}
}

func TestArenaTransactionCommit(t *testing.T) {
	arena := rg.NewArena(4096)
	defer arena.Close()

	var bInside []byte
	err := arena.Transaction(func(a *rg.Arena) error {
		bInside = a.Alloc(512)
		bInside[0] = 42
		return nil
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	if bInside[0] != 42 {
		t.Fatalf("data modified")
	}

	// Next alloc should be after bInside
	bNext := arena.Alloc(512)
	if &bNext[0] == &bInside[0] {
		t.Fatalf("committed allocation was overwritten")
	}
}
