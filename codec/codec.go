package codec

import (
	"encoding/binary"
	"errors"
	"io"
	"unsafe"

	rg "rustygo"
)

var (
	ErrUnexpectedEOF = io.ErrUnexpectedEOF
	ErrNegativeLength = errors.New("codec: negative length")
)

// Reader is an arena-backed zero-copy stream deserializer.
type Reader struct {
	r io.Reader
}

// NewReader wraps an io.Reader for arena-backed reading.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

// ReadBytes allocates a slice of length n from the scope and fills it from the underlying reader.
func (cr *Reader) ReadBytes(s *rg.Scope, n int) ([]byte, error) {
	if n < 0 {
		return nil, ErrNegativeLength
	}
	if n == 0 {
		return nil, nil
	}
	buf := rg.AllocSlice[byte](s, n)
	if _, err := io.ReadFull(cr.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// ReadString allocates n bytes in the arena scope, reads data, and returns a zero-copy string.
func (cr *Reader) ReadString(s *rg.Scope, n int) (string, error) {
	buf, err := cr.ReadBytes(s, n)
	if err != nil {
		return "", err
	}
	if len(buf) == 0 {
		return "", nil
	}
	return unsafe.String(&buf[0], len(buf)), nil
}

// ReadUint32 reads an unsigned 32-bit big-endian integer.
func (cr *Reader) ReadUint32() (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(cr.r, b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b[:]), nil
}

// ReadUint64 reads an unsigned 64-bit big-endian integer.
func (cr *Reader) ReadUint64() (uint64, error) {
	var b [8]byte
	if _, err := io.ReadFull(cr.r, b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

// ReadVarint reads a variable-length integer with zero allocations.
func (cr *Reader) ReadVarint() (uint64, error) {
	var x uint64
	var s uint
	var b [1]byte
	for i := 0; i < binary.MaxVarintLen64; i++ {
		if _, err := io.ReadFull(cr.r, b[:]); err != nil {
			return 0, err
		}
		byteVal := b[0]
		if byteVal < 0x80 {
			if i == binary.MaxVarintLen64-1 && byteVal > 1 {
				return 0, errors.New("codec: varint overflow")
			}
			return x | uint64(byteVal)<<s, nil
		}
		x |= uint64(byteVal&0x7f) << s
		s += 7
	}
	return 0, errors.New("codec: varint overflow")
}

// CloneString copies a Go string into arena memory and returns a string referencing the arena.
func CloneString(s *rg.Scope, str string) string {
	if len(str) == 0 {
		return ""
	}
	buf := rg.AllocSlice[byte](s, len(str))
	copy(buf, str)
	return unsafe.String(&buf[0], len(buf))
}
