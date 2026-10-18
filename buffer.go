package rustygo

import (
	"strconv"
	"unicode/utf8"
	"unsafe"
)

// ArenaBuffer is an arena-allocated growable byte buffer and string builder.
// All backing memory is allocated from the provided Scope, bypassing the Go runtime GC.
type ArenaBuffer struct {
	scope *Scope
	buf   []byte
	len   int
}

// NewArenaBuffer initializes a new buffer within the specified Scope.
func NewArenaBuffer(scope *Scope, initialCap int) *ArenaBuffer {
	if initialCap <= 0 {
		initialCap = 64
	}
	var b []byte
	if scope != nil {
		b = AllocSlice[byte](scope, initialCap)
	} else {
		b = make([]byte, initialCap)
	}
	return &ArenaBuffer{
		scope: scope,
		buf:   b,
		len:   0,
	}
}

func (b *ArenaBuffer) grow(extra int) {
	required := b.len + extra
	if required <= len(b.buf) {
		return
	}

	newCap := len(b.buf) * 2
	if newCap < required {
		newCap = required + 64
	}

	var newBuf []byte
	if b.scope != nil {
		newBuf = AllocSlice[byte](b.scope, newCap)
	} else {
		newBuf = make([]byte, newCap)
	}

	copy(newBuf, b.buf[:b.len])
	b.buf = newBuf
}

// Write appends a slice of bytes to the buffer.
func (b *ArenaBuffer) Write(p []byte) (int, error) {
	b.grow(len(p))
	copy(b.buf[b.len:], p)
	b.len += len(p)
	return len(p), nil
}

// WriteString appends string contents to the buffer.
func (b *ArenaBuffer) WriteString(s string) (int, error) {
	b.grow(len(s))
	copy(b.buf[b.len:], s)
	b.len += len(s)
	return len(s), nil
}

// WriteByte appends a single byte.
func (b *ArenaBuffer) WriteByte(c byte) error {
	b.grow(1)
	b.buf[b.len] = c
	b.len++
	return nil
}

// WriteRune appends a UTF-8 encoded rune.
func (b *ArenaBuffer) WriteRune(r rune) (int, error) {
	if r < utf8.RuneSelf {
		_ = b.WriteByte(byte(r))
		return 1, nil
	}
	b.grow(utf8.UTFMax)
	n := utf8.EncodeRune(b.buf[b.len:], r)
	b.len += n
	return n, nil
}

// WriteInt appends a decimal string representation of integer.
func (b *ArenaBuffer) WriteInt(v int64) {
	var tmp [32]byte
	res := strconv.AppendInt(tmp[:0], v, 10)
	_, _ = b.Write(res)
}

// WriteUint appends an unsigned integer in decimal.
func (b *ArenaBuffer) WriteUint(v uint64) {
	var tmp [32]byte
	res := strconv.AppendUint(tmp[:0], v, 10)
	_, _ = b.Write(res)
}

// Bytes returns the slice of accumulated bytes.
func (b *ArenaBuffer) Bytes() []byte {
	return b.buf[:b.len]
}

// String returns a zero-copy string representation pointing to arena memory.
func (b *ArenaBuffer) String() string {
	if b.len == 0 {
		return ""
	}
	return unsafe.String(&b.buf[0], b.len)
}

// Len returns the current number of written bytes.
func (b *ArenaBuffer) Len() int {
	return b.len
}

// Cap returns current capacity before another slab growth.
func (b *ArenaBuffer) Cap() int {
	return len(b.buf)
}

// Reset clears the buffer content without freeing the allocated slab.
func (b *ArenaBuffer) Reset() {
	b.len = 0
}

// Truncate discards all but the first n unread bytes from the buffer.
func (b *ArenaBuffer) Truncate(n int) {
	if n < 0 || n > b.len {
		return
	}
	b.len = n
}
