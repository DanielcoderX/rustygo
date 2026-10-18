package codec_test

import (
	"bytes"
	"strings"
	"testing"

	"rustygo/codec"
)

func TestFindQuoteOrEscape(t *testing.T) {
	data := []byte("this is a test string without delimiters until here \" and more")
	idx := codec.FindQuoteOrEscape(data)
	if idx != 52 {
		t.Fatalf("expected quote at 52, got %d", idx)
	}

	escapedData := []byte("path\\to\\file")
	idx = codec.FindQuoteOrEscape(escapedData)
	if idx != 4 {
		t.Fatalf("expected slash at 4, got %d", idx)
	}

	cleanData := []byte("no delimiters here at all")
	if idx = codec.FindQuoteOrEscape(cleanData); idx != -1 {
		t.Fatalf("expected -1, got %d", idx)
	}
}

func TestFindByte(t *testing.T) {
	data := []byte("12345678abcdefghXYZ")
	if idx := codec.FindByte(data, 'X'); idx != 16 {
		t.Fatalf("expected X at 16, got %d", idx)
	}
	if idx := codec.FindByte(data, '9'); idx != -1 {
		t.Fatalf("expected -1, got %d", idx)
	}
}

func BenchmarkFindQuoteOrEscapeVector(b *testing.B) {
	buf := []byte(strings.Repeat("a", 1024) + "\"")

	b.ResetTimer()
	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = codec.FindQuoteOrEscape(buf)
	}
}

func BenchmarkFindQuoteOrEscapeScalar(b *testing.B) {
	buf := []byte(strings.Repeat("a", 1024) + "\"")

	b.ResetTimer()
	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = bytes.IndexByte(buf, '"')
	}
}
