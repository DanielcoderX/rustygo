package rustygo_test

import (
	rg "rustygo"
	"testing"
)

func TestPoolWithBorrow(t *testing.T) {
	p := rg.NewPool(func() *int {
		v := 0
		return &v
	})
	err := p.WithBorrow(func(v *int) error {
		*v = 42
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	stats := p.Stats()
	if stats.InUse != 0 {
		t.Fatalf("expected InUse=0, got %d", stats.InUse)
	}
}

func TestWithGCDisabled(t *testing.T) {
	called := false
	err := rg.WithGCDisabled(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("callback was not called")
	}
}

func TestWithGCPercent(t *testing.T) {
	called := false
	err := rg.WithGCPercent(100, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("callback was not called")
	}
}
