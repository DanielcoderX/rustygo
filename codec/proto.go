package codec

import (
	"encoding/binary"
	"errors"
	"io"
	"unsafe"

	rg "rustygo"
)

// WireType represents a Protobuf wire encoding type.
type WireType int

const (
	WireVarint  WireType = 0
	WireFixed64 WireType = 1
	WireBytes   WireType = 2
	WireFixed32 WireType = 5
)

var (
	ErrMalformedProto = errors.New("codec/proto: malformed protobuf wire data")
	ErrInvalidWire    = errors.New("codec/proto: invalid wire type")
)

// ProtoDecoder provides high-throughput zero-copy Protobuf wire parsing with arena allocation.
type ProtoDecoder struct {
	data []byte
	pos  int
}

// NewProtoDecoder initializes a decoder over wire data.
func NewProtoDecoder(data []byte) *ProtoDecoder {
	return &ProtoDecoder{data: data, pos: 0}
}

// Reset re-initializes decoder over new data.
func (d *ProtoDecoder) Reset(data []byte) {
	d.data = data
	d.pos = 0
}

// HasMore returns true if unparsed bytes remain.
func (d *ProtoDecoder) HasMore() bool {
	return d.pos < len(d.data)
}

// NextField decodes the next tag, returning field number and wire type.
func (d *ProtoDecoder) NextField() (int, WireType, error) {
	if d.pos >= len(d.data) {
		return 0, 0, io.EOF
	}

	tag, err := d.DecodeVarint()
	if err != nil {
		return 0, 0, err
	}

	wireType := WireType(tag & 0x07)
	fieldNum := int(tag >> 3)
	if fieldNum <= 0 {
		return 0, 0, ErrMalformedProto
	}

	return fieldNum, wireType, nil
}

// DecodeVarint reads an unsigned 64-bit varint.
func (d *ProtoDecoder) DecodeVarint() (uint64, error) {
	var val uint64
	var shift uint

	for shift < 64 {
		if d.pos >= len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		b := d.data[d.pos]
		d.pos++

		val |= uint64(b&0x7F) << shift
		if (b & 0x80) == 0 {
			return val, nil
		}
		shift += 7
	}
	return 0, ErrMalformedProto
}

// DecodeZigZag64 decodes a signed 64-bit integer encoded with ZigZag.
func (d *ProtoDecoder) DecodeZigZag64() (int64, error) {
	v, err := d.DecodeVarint()
	if err != nil {
		return 0, err
	}
	return int64((v >> 1) ^ -(v & 1)), nil
}

// DecodeFixed32 reads a 32-bit fixed integer or float.
func (d *ProtoDecoder) DecodeFixed32() (uint32, error) {
	if d.pos+4 > len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint32(d.data[d.pos:])
	d.pos += 4
	return v, nil
}

// DecodeFixed64 reads a 64-bit fixed integer or double.
func (d *ProtoDecoder) DecodeFixed64() (uint64, error) {
	if d.pos+8 > len(d.data) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint64(d.data[d.pos:])
	d.pos += 8
	return v, nil
}

// DecodeString decodes a length-delimited string directly into the arena Scope.
func (d *ProtoDecoder) DecodeString(scope *rg.Scope) (string, error) {
	length, err := d.DecodeVarint()
	if err != nil {
		return "", err
	}
	l := int(length)
	if d.pos+l > len(d.data) {
		return "", io.ErrUnexpectedEOF
	}

	raw := d.data[d.pos : d.pos+l]
	d.pos += l

	if scope != nil && l > 0 {
		buf := rg.AllocSlice[byte](scope, l)
		copy(buf, raw)
		return unsafe.String(&buf[0], l), nil
	}
	return string(raw), nil
}

// DecodeBytes decodes length-delimited bytes directly into the arena Scope.
func (d *ProtoDecoder) DecodeBytes(scope *rg.Scope) ([]byte, error) {
	length, err := d.DecodeVarint()
	if err != nil {
		return nil, err
	}
	l := int(length)
	if d.pos+l > len(d.data) {
		return nil, io.ErrUnexpectedEOF
	}

	raw := d.data[d.pos : d.pos+l]
	d.pos += l

	if scope != nil && l > 0 {
		buf := rg.AllocSlice[byte](scope, l)
		copy(buf, raw)
		return buf, nil
	}
	out := make([]byte, l)
	copy(out, raw)
	return out, nil
}

// SkipField skips the payload of an unneeded field based on its wire type.
func (d *ProtoDecoder) SkipField(wireType WireType) error {
	switch wireType {
	case WireVarint:
		_, err := d.DecodeVarint()
		return err
	case WireFixed64:
		if d.pos+8 > len(d.data) {
			return io.ErrUnexpectedEOF
		}
		d.pos += 8
		return nil
	case WireBytes:
		length, err := d.DecodeVarint()
		if err != nil {
			return err
		}
		if d.pos+int(length) > len(d.data) {
			return io.ErrUnexpectedEOF
		}
		d.pos += int(length)
		return nil
	case WireFixed32:
		if d.pos+4 > len(d.data) {
			return io.ErrUnexpectedEOF
		}
		d.pos += 4
		return nil
	default:
		return ErrInvalidWire
	}
}

// ProtoEncoder provides low-allocation serialization of Protobuf wire fields.
type ProtoEncoder struct {
	buf []byte
}

// NewProtoEncoder creates a new encoder.
func NewProtoEncoder() *ProtoEncoder {
	return &ProtoEncoder{buf: make([]byte, 0, 128)}
}

// Bytes returns the encoded bytes.
func (e *ProtoEncoder) Bytes() []byte {
	return e.buf
}

// Reset clears the buffer.
func (e *ProtoEncoder) Reset() {
	e.buf = e.buf[:0]
}

func (e *ProtoEncoder) encodeTag(fieldNum int, wireType WireType) {
	tag := uint64((fieldNum << 3) | int(wireType))
	e.appendVarint(tag)
}

func (e *ProtoEncoder) appendVarint(v uint64) {
	for v >= 0x80 {
		e.buf = append(e.buf, byte(v)|0x80)
		v >>= 7
	}
	e.buf = append(e.buf, byte(v))
}

// EncodeVarint writes a varint field.
func (e *ProtoEncoder) EncodeVarint(fieldNum int, val uint64) {
	e.encodeTag(fieldNum, WireVarint)
	e.appendVarint(val)
}

// EncodeZigZag64 writes a signed integer with ZigZag encoding.
func (e *ProtoEncoder) EncodeZigZag64(fieldNum int, val int64) {
	v := uint64((val << 1) ^ (val >> 63))
	e.EncodeVarint(fieldNum, v)
}

// EncodeFixed32 writes a 32-bit fixed integer.
func (e *ProtoEncoder) EncodeFixed32(fieldNum int, val uint32) {
	e.encodeTag(fieldNum, WireFixed32)
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], val)
	e.buf = append(e.buf, b[:]...)
}

// EncodeFixed64 writes a 64-bit fixed integer.
func (e *ProtoEncoder) EncodeFixed64(fieldNum int, val uint64) {
	e.encodeTag(fieldNum, WireFixed64)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], val)
	e.buf = append(e.buf, b[:]...)
}

// EncodeString writes a string field.
func (e *ProtoEncoder) EncodeString(fieldNum int, val string) {
	e.encodeTag(fieldNum, WireBytes)
	e.appendVarint(uint64(len(val)))
	e.buf = append(e.buf, val...)
}

// EncodeBytes writes a raw byte array field.
func (e *ProtoEncoder) EncodeBytes(fieldNum int, val []byte) {
	e.encodeTag(fieldNum, WireBytes)
	e.appendVarint(uint64(len(val)))
	e.buf = append(e.buf, val...)
}
