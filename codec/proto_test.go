package codec_test

import (
	"bytes"
	"io"
	"testing"

	rg "rustygo"
	"rustygo/codec"
)

func TestProtoRoundtrip(t *testing.T) {
	enc := codec.NewProtoEncoder()

	// Field 1: Varint ID
	enc.EncodeVarint(1, 1024)
	// Field 2: String Name
	enc.EncodeString(2, "RustyGo Proto Engine")
	// Field 3: ZigZag negative integer
	enc.EncodeZigZag64(3, -42)
	// Field 4: Fixed32
	enc.EncodeFixed32(4, 0x12345678)
	// Field 5: Fixed64
	enc.EncodeFixed64(5, 0x0102030405060708)
	// Field 6: Raw Bytes
	enc.EncodeBytes(6, []byte("zero-copy-payload"))

	data := enc.Bytes()

	arena := rg.NewArena(4096)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	dec := codec.NewProtoDecoder(data)

	var (
		gotID      uint64
		gotName    string
		gotZigZag  int64
		gotFixed32 uint32
		gotFixed64 uint64
		gotBytes   []byte
	)

	for dec.HasMore() {
		fieldNum, wireType, err := dec.NextField()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error reading field: %v", err)
		}

		switch fieldNum {
		case 1:
			gotID, err = dec.DecodeVarint()
		case 2:
			gotName, err = dec.DecodeString(scope)
		case 3:
			gotZigZag, err = dec.DecodeZigZag64()
		case 4:
			gotFixed32, err = dec.DecodeFixed32()
		case 5:
			gotFixed64, err = dec.DecodeFixed64()
		case 6:
			gotBytes, err = dec.DecodeBytes(scope)
		default:
			err = dec.SkipField(wireType)
		}
		if err != nil {
			t.Fatalf("failed decoding field %d: %v", fieldNum, err)
		}
	}

	if gotID != 1024 {
		t.Fatalf("expected ID 1024, got %d", gotID)
	}
	if gotName != "RustyGo Proto Engine" {
		t.Fatalf("expected name %q, got %q", "RustyGo Proto Engine", gotName)
	}
	if gotZigZag != -42 {
		t.Fatalf("expected -42, got %d", gotZigZag)
	}
	if gotFixed32 != 0x12345678 {
		t.Fatalf("expected 0x12345678, got 0x%X", gotFixed32)
	}
	if gotFixed64 != 0x0102030405060708 {
		t.Fatalf("expected 0x0102030405060708, got 0x%X", gotFixed64)
	}
	if !bytes.Equal(gotBytes, []byte("zero-copy-payload")) {
		t.Fatalf("payload mismatch")
	}
}

func TestProtoSkipField(t *testing.T) {
	enc := codec.NewProtoEncoder()
	enc.EncodeFixed64(1, 999999)
	enc.EncodeString(2, "to-skip")
	enc.EncodeVarint(3, 777)

	dec := codec.NewProtoDecoder(enc.Bytes())

	fieldNum, wireType, err := dec.NextField()
	if err != nil || fieldNum != 1 {
		t.Fatal(err)
	}
	if err := dec.SkipField(wireType); err != nil {
		t.Fatal(err)
	}

	fieldNum, wireType, err = dec.NextField()
	if err != nil || fieldNum != 2 {
		t.Fatal(err)
	}
	if err := dec.SkipField(wireType); err != nil {
		t.Fatal(err)
	}

	fieldNum, _, err = dec.NextField()
	if err != nil || fieldNum != 3 {
		t.Fatal(err)
	}
	val, err := dec.DecodeVarint()
	if err != nil || val != 777 {
		t.Fatalf("expected 777, got %d (err: %v)", val, err)
	}
}
