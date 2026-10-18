package codec_test

import (
	"bytes"
	"strings"
	"testing"

	rg "rustygo"
	"rustygo/codec"
)

func TestWSFrameMaskedTextRoundtrip(t *testing.T) {
	msg := "Hello WebSocket Arena Streaming Engine!"
	mask := [4]byte{0x1A, 0x2B, 0x3C, 0x4D}

	var buf bytes.Buffer
	err := codec.WriteWSFrame(&buf, codec.WSOpText, true, []byte(msg), &mask)
	if err != nil {
		t.Fatalf("failed writing frame: %v", err)
	}

	arena := rg.NewArena(4096)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	frame, err := codec.ReadWSFrame(&buf, scope)
	if err != nil {
		t.Fatalf("failed reading frame: %v", err)
	}

	if frame.Opcode != codec.WSOpText || !frame.Fin {
		t.Fatalf("unexpected opcode/fin: %v/%v", frame.Opcode, frame.Fin)
	}

	if string(frame.Payload) != msg {
		t.Fatalf("payload mismatch: got %q, want %q", string(frame.Payload), msg)
	}
}

func TestWSFrameLargeBinaryVectorized(t *testing.T) {
	largeMsg := strings.Repeat("VECTORIZED-UNMASK-FAST-PATH-CHUNK-", 300) // ~10KB
	mask := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}

	var buf bytes.Buffer
	err := codec.WriteWSFrame(&buf, codec.WSOpBinary, true, []byte(largeMsg), &mask)
	if err != nil {
		t.Fatal(err)
	}

	arena := rg.NewArena(64 * 1024)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	frame, err := codec.ReadWSFrame(&buf, scope)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(frame.Payload, []byte(largeMsg)) {
		t.Fatalf("large binary unmasking mismatch")
	}
}

func TestWSFramePingPong(t *testing.T) {
	pingPayload := []byte("heartbeat-1234")

	var buf bytes.Buffer
	err := codec.WriteWSFrame(&buf, codec.WSOpPing, true, pingPayload, nil)
	if err != nil {
		t.Fatal(err)
	}

	arena := rg.NewArena(1024)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	frame, err := codec.ReadWSFrame(&buf, scope)
	if err != nil {
		t.Fatal(err)
	}

	if frame.Opcode != codec.WSOpPing || !bytes.Equal(frame.Payload, pingPayload) {
		t.Fatalf("ping frame mismatch")
	}
}
