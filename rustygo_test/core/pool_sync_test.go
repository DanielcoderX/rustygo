package rustygo_test

import (
	"testing"

	rg "rustygo"
)

func TestSyncPoolWrapper(t *testing.T) {
	pool := rg.NewSyncPoolWrapper(func() *int {
		v := new(int)
		*v = 42
		return v
	})

	v1 := pool.Get()
	if v1 == nil || *v1 != 42 {
		t.Fatalf("expected 42, got %v", v1)
	}

	*v1 = 100
	pool.Put(v1)

	v2 := pool.Get()
	if v2 == nil {
		t.Fatalf("expected non-nil")
	}
	// It could be the same object or a new one depending on sync.Pool internals,
	// but since we just put it, it's highly likely to be 100 or 42.
	if *v2 != 100 && *v2 != 42 {
		t.Fatalf("expected 100 or 42, got %d", *v2)
	}
}
