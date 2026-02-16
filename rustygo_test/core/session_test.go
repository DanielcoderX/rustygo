package rustygo_test

import (
	rg "rustygo"
	"testing"
)

func TestSessionBorrowRelease(t *testing.T) {
	s := rg.NewSession()
	buf := s.Borrow(32)
	if len(buf) != 32 {
		t.Fatalf("expected len=32, got %d", len(buf))
	}
	buf[0] = 99
	s.Release(buf)

	buf2 := s.Borrow(32)
	if len(buf2) != 32 {
		t.Fatalf("expected len=32, got %d", len(buf2))
	}
	if buf2[0] != 0 {
		t.Fatalf("expected zeroed reused buffer, got %d", buf2[0])
	}
}

func TestSessionWithBorrow(t *testing.T) {
	s := rg.NewSession()
	if err := s.WithBorrow(16, func(buf []byte) error {
		if len(buf) != 16 {
			t.Fatalf("expected len=16, got %d", len(buf))
		}
		buf[15] = 7
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultSessionFunctions(t *testing.T) {
	rg.Reset()
	buf := rg.Borrow(8)
	if len(buf) != 8 {
		t.Fatalf("expected len=8, got %d", len(buf))
	}
	rg.Release(buf)
}

func TestSessionMaxRetain(t *testing.T) {
	s := rg.NewSession(
		rg.WithSessionMaxRetain(32),
	)
	large := make([]byte, 0, 128)
	s.Release(large)
	stats := s.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Fatalf("unexpected stats before borrow: %+v", stats)
	}
}
