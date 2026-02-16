package rustygo

import (
	"sync"
	"sync/atomic"
)

const (
	defaultSessionMaxRetain = 64 * 1024
)

type SessionOption func(*Session)

// WithSessionMaxRetain sets the maximum capacity (in bytes) that is kept in the session cache.
func WithSessionMaxRetain(max int) SessionOption {
	return func(s *Session) {
		if max > 0 {
			s.maxRetain = max
		}
	}
}

// WithSessionZeroOnRelease controls whether released buffers are zeroed before reuse.
func WithSessionZeroOnRelease(enabled bool) SessionOption {
	return func(s *Session) {
		s.zeroOnRelease = enabled
	}
}

// Session provides a high-level, zero-config memory reuse surface.
type Session struct {
	pool sync.Pool

	maxRetain     int
	zeroOnRelease bool

	hits   uint64
	misses uint64
}

// SessionStats provides basic diagnostics for a session.
type SessionStats struct {
	Hits   uint64
	Misses uint64
}

// NewSession creates a new high-level session with safe defaults.
func NewSession(opts ...SessionOption) *Session {
	s := &Session{
		maxRetain:     defaultSessionMaxRetain,
		zeroOnRelease: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

var defaultSession = NewSession()

// DefaultSession returns the process-wide default session.
func DefaultSession() *Session {
	return defaultSession
}

// Borrow returns a buffer of length size, reusing cached memory when possible.
func (s *Session) Borrow(size int) []byte {
	if size <= 0 {
		panic("Borrow size must be > 0")
	}

	v := s.pool.Get()
	if v == nil {
		atomic.AddUint64(&s.misses, 1)
		return make([]byte, size)
	}

	atomic.AddUint64(&s.hits, 1)
	buf := v.([]byte)
	if cap(buf) < size {
		atomic.AddUint64(&s.misses, 1)
		return make([]byte, size)
	}
	return buf[:size]
}

// Release returns a borrowed buffer to the session cache.
func (s *Session) Release(buf []byte) {
	if buf == nil {
		return
	}
	if cap(buf) > s.maxRetain {
		return
	}
	if s.zeroOnRelease {
		for i := range buf {
			buf[i] = 0
		}
	}
	s.pool.Put(buf[:0])
}

// WithBorrow borrows a buffer, executes fn, and always releases the buffer.
func (s *Session) WithBorrow(size int, fn func([]byte) error) error {
	if fn == nil {
		return nil
	}
	buf := s.Borrow(size)
	defer s.Release(buf)
	return fn(buf)
}

// Reset drops currently cached buffers for future borrows.
func (s *Session) Reset() {
	s.pool = sync.Pool{}
}

// Stats returns session hit/miss statistics.
func (s *Session) Stats() SessionStats {
	return SessionStats{
		Hits:   atomic.LoadUint64(&s.hits),
		Misses: atomic.LoadUint64(&s.misses),
	}
}

// Borrow returns a buffer from the default session.
func Borrow(size int) []byte {
	return defaultSession.Borrow(size)
}

// Release returns a buffer to the default session.
func Release(buf []byte) {
	defaultSession.Release(buf)
}

// WithBorrow borrows from the default session for the duration of fn.
func WithBorrow(size int, fn func([]byte) error) error {
	return defaultSession.WithBorrow(size, fn)
}

// Reset clears cached buffers in the default session.
func Reset() {
	defaultSession.Reset()
}
