package codec

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"unsafe"

	rg "rustygo"
)

var (
	ErrMalformedMsgPack = errors.New("codec/msgpack: malformed MessagePack payload")
	ErrTypeMismatch     = errors.New("codec/msgpack: type mismatch")
)

// MsgPackDecoder provides zero-copy arena-backed decoding for MessagePack payloads.
type MsgPackDecoder struct {
	data []byte
	pos  int
}

// NewMsgPackDecoder initializes a decoder over binary data.
func NewMsgPackDecoder(data []byte) *MsgPackDecoder {
	return &MsgPackDecoder{data: data, pos: 0}
}

// Reset re-arms the decoder on new data.
func (d *MsgPackDecoder) Reset(data []byte) {
	d.data = data
	d.pos = 0
}

func (d *MsgPackDecoder) Remaining() int {
	return len(d.data) - d.pos
}

// DecodeNil consumes a nil token.
func (d *MsgPackDecoder) DecodeNil() error {
	if d.pos >= len(d.data) {
		return io.EOF
	}
	if d.data[d.pos] != 0xc0 {
		return ErrTypeMismatch
	}
	d.pos++
	return nil
}

// DecodeBool decodes a boolean value.
func (d *MsgPackDecoder) DecodeBool() (bool, error) {
	if d.pos >= len(d.data) {
		return false, io.EOF
	}
	b := d.data[d.pos]
	if b == 0xc2 {
		d.pos++
		return false, nil
	}
	if b == 0xc3 {
		d.pos++
		return true, nil
	}
	return false, ErrTypeMismatch
}

// DecodeInt decodes signed or unsigned integer as int64.
func (d *MsgPackDecoder) DecodeInt() (int64, error) {
	if d.pos >= len(d.data) {
		return 0, io.EOF
	}
	b := d.data[d.pos]

	// Positive fixint: 0x00 - 0x7f
	if b <= 0x7f {
		d.pos++
		return int64(b), nil
	}
	// Negative fixint: 0xe0 - 0xff
	if b >= 0xe0 {
		d.pos++
		return int64(int8(b)), nil
	}

	switch b {
	case 0xcc: // uint 8
		if d.pos+2 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := d.data[d.pos+1]
		d.pos += 2
		return int64(v), nil
	case 0xcd: // uint 16
		if d.pos+3 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := binary.BigEndian.Uint16(d.data[d.pos+1:])
		d.pos += 3
		return int64(v), nil
	case 0xce: // uint 32
		if d.pos+5 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := binary.BigEndian.Uint32(d.data[d.pos+1:])
		d.pos += 5
		return int64(v), nil
	case 0xcf: // uint 64
		if d.pos+9 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := binary.BigEndian.Uint64(d.data[d.pos+1:])
		d.pos += 9
		return int64(v), nil
	case 0xd0: // int 8
		if d.pos+2 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int8(d.data[d.pos+1])
		d.pos += 2
		return int64(v), nil
	case 0xd1: // int 16
		if d.pos+3 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int16(binary.BigEndian.Uint16(d.data[d.pos+1:]))
		d.pos += 3
		return int64(v), nil
	case 0xd2: // int 32
		if d.pos+5 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int32(binary.BigEndian.Uint32(d.data[d.pos+1:]))
		d.pos += 5
		return int64(v), nil
	case 0xd3: // int 64
		if d.pos+9 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int64(binary.BigEndian.Uint64(d.data[d.pos+1:]))
		d.pos += 9
		return v, nil
	default:
		return 0, ErrTypeMismatch
	}
}

// DecodeFloat64 decodes float32 or float64.
func (d *MsgPackDecoder) DecodeFloat64() (float64, error) {
	if d.pos >= len(d.data) {
		return 0, io.EOF
	}
	b := d.data[d.pos]
	if b == 0xca { // float 32
		if d.pos+5 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		bits := binary.BigEndian.Uint32(d.data[d.pos+1:])
		d.pos += 5
		return float64(math.Float32frombits(bits)), nil
	}
	if b == 0xcb { // float 64
		if d.pos+9 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		bits := binary.BigEndian.Uint64(d.data[d.pos+1:])
		d.pos += 9
		return math.Float64frombits(bits), nil
	}
	return 0, ErrTypeMismatch
}

// DecodeString decodes string content directly into the provided Scope without heap escape.
func (d *MsgPackDecoder) DecodeString(scope *rg.Scope) (string, error) {
	if d.pos >= len(d.data) {
		return "", io.EOF
	}
	b := d.data[d.pos]
	var length int

	if b >= 0xa0 && b <= 0xbf {
		// fixstr
		length = int(b & 0x1f)
		d.pos++
	} else if b == 0xd9 {
		// str 8
		if d.pos+2 > len(d.data) {
			return "", io.ErrUnexpectedEOF
		}
		length = int(d.data[d.pos+1])
		d.pos += 2
	} else if b == 0xda {
		// str 16
		if d.pos+3 > len(d.data) {
			return "", io.ErrUnexpectedEOF
		}
		length = int(binary.BigEndian.Uint16(d.data[d.pos+1:]))
		d.pos += 3
	} else if b == 0xdb {
		// str 32
		if d.pos+5 > len(d.data) {
			return "", io.ErrUnexpectedEOF
		}
		length = int(binary.BigEndian.Uint32(d.data[d.pos+1:]))
		d.pos += 5
	} else {
		return "", ErrTypeMismatch
	}

	if d.pos+length > len(d.data) {
		return "", io.ErrUnexpectedEOF
	}

	raw := d.data[d.pos : d.pos+length]
	d.pos += length

	if scope != nil {
		buf := rg.AllocSlice[byte](scope, length)
		copy(buf, raw)
		return unsafe.String(&buf[0], length), nil
	}
	return string(raw), nil
}

// DecodeBytes decodes raw binary directly into the provided Scope without heap allocation.
func (d *MsgPackDecoder) DecodeBytes(scope *rg.Scope) ([]byte, error) {
	if d.pos >= len(d.data) {
		return nil, io.EOF
	}
	b := d.data[d.pos]
	var length int

	if b == 0xc4 { // bin 8
		if d.pos+2 > len(d.data) {
			return nil, io.ErrUnexpectedEOF
		}
		length = int(d.data[d.pos+1])
		d.pos += 2
	} else if b == 0xc5 { // bin 16
		if d.pos+3 > len(d.data) {
			return nil, io.ErrUnexpectedEOF
		}
		length = int(binary.BigEndian.Uint16(d.data[d.pos+1:]))
		d.pos += 3
	} else if b == 0xc6 { // bin 32
		if d.pos+5 > len(d.data) {
			return nil, io.ErrUnexpectedEOF
		}
		length = int(binary.BigEndian.Uint32(d.data[d.pos+1:]))
		d.pos += 5
	} else {
		return nil, ErrTypeMismatch
	}

	if d.pos+length > len(d.data) {
		return nil, io.ErrUnexpectedEOF
	}

	raw := d.data[d.pos : d.pos+length]
	d.pos += length

	if scope != nil {
		buf := rg.AllocSlice[byte](scope, length)
		copy(buf, raw)
		return buf, nil
	}
	out := make([]byte, length)
	copy(out, raw)
	return out, nil
}

// DecodeArrayHeader decodes the number of elements in an array.
func (d *MsgPackDecoder) DecodeArrayHeader() (int, error) {
	if d.pos >= len(d.data) {
		return 0, io.EOF
	}
	b := d.data[d.pos]
	if b >= 0x90 && b <= 0x9f {
		d.pos++
		return int(b & 0x0f), nil
	}
	if b == 0xdc { // array 16
		if d.pos+3 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int(binary.BigEndian.Uint16(d.data[d.pos+1:]))
		d.pos += 3
		return v, nil
	}
	if b == 0xdd { // array 32
		if d.pos+5 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int(binary.BigEndian.Uint32(d.data[d.pos+1:]))
		d.pos += 5
		return v, nil
	}
	return 0, ErrTypeMismatch
}

// DecodeMapHeader decodes the number of key-value pairs in a map.
func (d *MsgPackDecoder) DecodeMapHeader() (int, error) {
	if d.pos >= len(d.data) {
		return 0, io.EOF
	}
	b := d.data[d.pos]
	if b >= 0x80 && b <= 0x8f {
		d.pos++
		return int(b & 0x0f), nil
	}
	if b == 0xde { // map 16
		if d.pos+3 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int(binary.BigEndian.Uint16(d.data[d.pos+1:]))
		d.pos += 3
		return v, nil
	}
	if b == 0xdf { // map 32
		if d.pos+5 > len(d.data) {
			return 0, io.ErrUnexpectedEOF
		}
		v := int(binary.BigEndian.Uint32(d.data[d.pos+1:]))
		d.pos += 5
		return v, nil
	}
	return 0, ErrTypeMismatch
}

// MsgPackEncoder is a lightweight fast encoder for serializing MessagePack.
type MsgPackEncoder struct {
	buf []byte
}

func NewMsgPackEncoder() *MsgPackEncoder {
	return &MsgPackEncoder{buf: make([]byte, 0, 128)}
}

func (e *MsgPackEncoder) Bytes() []byte {
	return e.buf
}

func (e *MsgPackEncoder) EncodeNil() {
	e.buf = append(e.buf, 0xc0)
}

func (e *MsgPackEncoder) EncodeBool(v bool) {
	if v {
		e.buf = append(e.buf, 0xc3)
	} else {
		e.buf = append(e.buf, 0xc2)
	}
}

func (e *MsgPackEncoder) EncodeInt(v int64) {
	if v >= 0 && v <= 127 {
		e.buf = append(e.buf, byte(v))
		return
	}
	if v >= -32 && v < 0 {
		e.buf = append(e.buf, byte(v))
		return
	}
	if v >= math.MinInt8 && v <= math.MaxInt8 {
		e.buf = append(e.buf, 0xd0, byte(v))
		return
	}
	if v >= math.MinInt16 && v <= math.MaxInt16 {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(v))
		e.buf = append(append(e.buf, 0xd1), b[:]...)
		return
	}
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(v))
		e.buf = append(append(e.buf, 0xd2), b[:]...)
		return
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	e.buf = append(append(e.buf, 0xd3), b[:]...)
}

func (e *MsgPackEncoder) EncodeString(s string) {
	l := len(s)
	if l <= 31 {
		e.buf = append(e.buf, 0xa0|byte(l))
	} else if l <= math.MaxUint8 {
		e.buf = append(e.buf, 0xd9, byte(l))
	} else if l <= math.MaxUint16 {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(l))
		e.buf = append(append(e.buf, 0xda), b[:]...)
	} else {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(l))
		e.buf = append(append(e.buf, 0xdb), b[:]...)
	}
	e.buf = append(e.buf, s...)
}

func (e *MsgPackEncoder) EncodeBytes(b []byte) {
	l := len(b)
	if l <= math.MaxUint8 {
		e.buf = append(e.buf, 0xc4, byte(l))
	} else if l <= math.MaxUint16 {
		var hdr [2]byte
		binary.BigEndian.PutUint16(hdr[:], uint16(l))
		e.buf = append(append(e.buf, 0xc5), hdr[:]...)
	} else {
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(l))
		e.buf = append(append(e.buf, 0xc6), hdr[:]...)
	}
	e.buf = append(e.buf, b...)
}

func (e *MsgPackEncoder) EncodeArrayHeader(size int) {
	if size <= 15 {
		e.buf = append(e.buf, 0x90|byte(size))
	} else if size <= math.MaxUint16 {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(size))
		e.buf = append(append(e.buf, 0xdc), b[:]...)
	} else {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(size))
		e.buf = append(append(e.buf, 0xdd), b[:]...)
	}
}

func (e *MsgPackEncoder) EncodeMapHeader(size int) {
	if size <= 15 {
		e.buf = append(e.buf, 0x80|byte(size))
	} else if size <= math.MaxUint16 {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(size))
		e.buf = append(append(e.buf, 0xde), b[:]...)
	} else {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(size))
		e.buf = append(append(e.buf, 0xdf), b[:]...)
	}
}
