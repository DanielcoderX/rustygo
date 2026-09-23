package codec_test

import (
	"bytes"
	"testing"

	rg "rustygo"
	"rustygo/codec"
)

func TestMsgPackRoundtrip(t *testing.T) {
	enc := codec.NewMsgPackEncoder()

	// Encode Map with 4 pairs
	enc.EncodeMapHeader(4)
	enc.EncodeString("name")
	enc.EncodeString("RustyGo")
	enc.EncodeString("version")
	enc.EncodeInt(5)
	enc.EncodeString("active")
	enc.EncodeBool(true)
	enc.EncodeString("payload")
	enc.EncodeBytes([]byte("zero-copy-binary-bytes"))

	data := enc.Bytes()

	arena := rg.NewArena(4096)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	dec := codec.NewMsgPackDecoder(data)
	mapLen, err := dec.DecodeMapHeader()
	if err != nil || mapLen != 4 {
		t.Fatalf("expected map of 4, got len=%d err=%v", mapLen, err)
	}

	results := make(map[string]any)
	for i := 0; i < mapLen; i++ {
		key, err := dec.DecodeString(scope)
		if err != nil {
			t.Fatalf("failed decoding key: %v", err)
		}
		switch key {
		case "name":
			val, err := dec.DecodeString(scope)
			if err != nil {
				t.Fatal(err)
			}
			results[key] = val
		case "version":
			val, err := dec.DecodeInt()
			if err != nil {
				t.Fatal(err)
			}
			results[key] = val
		case "active":
			val, err := dec.DecodeBool()
			if err != nil {
				t.Fatal(err)
			}
			results[key] = val
		case "payload":
			val, err := dec.DecodeBytes(scope)
			if err != nil {
				t.Fatal(err)
			}
			results[key] = val
		}
	}

	if results["name"] != "RustyGo" {
		t.Fatalf("expected name=RustyGo, got %v", results["name"])
	}
	if results["version"] != int64(5) {
		t.Fatalf("expected version=5, got %v", results["version"])
	}
	if results["active"] != true {
		t.Fatalf("expected active=true, got %v", results["active"])
	}
	if !bytes.Equal(results["payload"].([]byte), []byte("zero-copy-binary-bytes")) {
		t.Fatalf("payload mismatch")
	}
}

func TestMsgPackArrayAndNumbers(t *testing.T) {
	enc := codec.NewMsgPackEncoder()
	enc.EncodeArrayHeader(3)
	enc.EncodeInt(-10)
	enc.EncodeInt(1000000)
	enc.EncodeNil()

	dec := codec.NewMsgPackDecoder(enc.Bytes())
	l, err := dec.DecodeArrayHeader()
	if err != nil || l != 3 {
		t.Fatalf("expected array len=3, got %d err=%v", l, err)
	}

	v1, err := dec.DecodeInt()
	if err != nil || v1 != -10 {
		t.Fatalf("expected -10, got %d", v1)
	}
	v2, err := dec.DecodeInt()
	if err != nil || v2 != 1000000 {
		t.Fatalf("expected 1000000, got %d", v2)
	}
	if err := dec.DecodeNil(); err != nil {
		t.Fatalf("expected nil, got error: %v", err)
	}
}
