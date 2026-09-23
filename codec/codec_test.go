package codec_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	rg "rustygo"
	"rustygo/codec"
)

func TestCodecReaderBytesAndString(t *testing.T) {
	data := []byte("Hello, RustyGo Arena Zero-Copy World!")
	r := bytes.NewReader(data)
	cr := codec.NewReader(r)

	arena := rg.NewArena(1024)
	defer arena.Close()

	scope := arena.EnterScope()
	defer scope.Exit()

	s, err := cr.ReadString(scope, 5)
	if err != nil {
		t.Fatal(err)
	}
	if s != "Hello" {
		t.Fatalf("expected 'Hello', got '%s'", s)
	}

	b, err := cr.ReadBytes(scope, 2)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != ", " {
		t.Fatalf("expected ', ', got '%s'", string(b))
	}

	rest, err := cr.ReadString(scope, len(data)-7)
	if err != nil {
		t.Fatal(err)
	}
	if rest != "RustyGo Arena Zero-Copy World!" {
		t.Fatalf("expected rest string, got '%s'", rest)
	}
}

func TestCodecReaderNumbers(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(0xDEADBEEF))
	_ = binary.Write(&buf, binary.BigEndian, uint64(0x0123456789ABCDEF))

	cr := codec.NewReader(&buf)
	u32, err := cr.ReadUint32()
	if err != nil {
		t.Fatal(err)
	}
	if u32 != 0xDEADBEEF {
		t.Fatalf("expected 0xDEADBEEF, got 0x%X", u32)
	}

	u64, err := cr.ReadUint64()
	if err != nil {
		t.Fatal(err)
	}
	if u64 != 0x0123456789ABCDEF {
		t.Fatalf("expected 0x0123456789ABCDEF, got 0x%X", u64)
	}
}

func TestCloneString(t *testing.T) {
	arena := rg.NewArena(1024)
	defer arena.Close()

	scope := arena.EnterScope()
	defer scope.Exit()

	orig := "persistent string data"
	cloned := codec.CloneString(scope, orig)
	if cloned != orig {
		t.Fatalf("expected '%s', got '%s'", orig, cloned)
	}
	if scope.UsedBytes() < len(orig) {
		t.Fatalf("expected used bytes >= %d, got %d", len(orig), scope.UsedBytes())
	}
}

func BenchmarkCodecZeroCopyString(b *testing.B) {
	payload := []byte("The quick brown fox jumps over the lazy dog.")
	arena := rg.NewArena(4096)
	defer arena.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scope := arena.EnterScope()
		r := bytes.NewReader(payload)
		cr := codec.NewReader(r)
		_, _ = cr.ReadString(scope, len(payload))
		scope.Exit()
		arena.Reset()
	}
}
