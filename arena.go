package rustygo

import (
	"errors"
	"sync/atomic"
	"unsafe"
)

// Arena is a fixed-size memory arena
type Arena struct {
	buf  []byte
	off  uint64
	base uintptr
}

var (
	// ErrInvalidAllocSize is returned when allocation size is <= 0.
	ErrInvalidAllocSize = errors.New("alloc size must be > 0")
	// ErrArenaOutOfMemory is returned when arena cannot satisfy an allocation request.
	ErrArenaOutOfMemory = errors.New("arena out of memory")
	// ErrInvalidAlignment is returned when alignment is not a positive power of 2.
	ErrInvalidAlignment = errors.New("alignment must be a positive power of 2")
	// ErrInvalidArenaMark is returned when a mark is outside valid arena bounds.
	ErrInvalidArenaMark = errors.New("invalid arena mark")
	// ErrRewindForward is returned when rewinding to a mark ahead of current offset.
	ErrRewindForward = errors.New("cannot rewind forward")
	// ErrScopeInactive is returned when allocation is attempted on an exited scope.
	ErrScopeInactive = errors.New("scope is not active")
)

type ArenaMark uint64

func NewArena(size int) *Arena {
	if size <= 0 {
		panic("arena size must be > 0")
	}
	buf := make([]byte, size)
	return &Arena{
		buf:  buf,
		base: uintptr(unsafe.Pointer(&buf[0])),
	}
}

// TryAlloc allocates memory globally and reports whether it succeeded.
func (a *Arena) TryAlloc(n int) ([]byte, bool) {
	if n <= 0 {
		return nil, false
	}
	req := uint64(n)
	capacity := uint64(len(a.buf))
	for {
		off := atomic.LoadUint64(&a.off)
		end := off + req
		if end < off || end > capacity {
			return nil, false
		}
		if atomic.CompareAndSwapUint64(&a.off, off, end) {
			return a.buf[off:end], true
		}
	}
}

// AllocOrErr allocates memory globally and returns a descriptive error on failure.
func (a *Arena) AllocOrErr(n int) ([]byte, error) {
	if n <= 0 {
		return nil, ErrInvalidAllocSize
	}
	buf, ok := a.TryAlloc(n)
	if !ok {
		return nil, ErrArenaOutOfMemory
	}
	return buf, nil
}

// TryAllocAligned allocates n bytes with the given alignment and reports whether it succeeded.
func (a *Arena) TryAllocAligned(n, align int) ([]byte, bool) {
	if n <= 0 || !isPowerOfTwo(align) {
		return nil, false
	}
	req := uint64(n)
	mask := uintptr(align - 1)
	capacity := uint64(len(a.buf))
	for {
		off := atomic.LoadUint64(&a.off)
		curAddr := a.base + uintptr(off)
		alignedAddr := (curAddr + mask) & ^mask
		aligned := uint64(alignedAddr - a.base)
		end := aligned + req
		if aligned < off || end < aligned || end > capacity {
			return nil, false
		}
		if atomic.CompareAndSwapUint64(&a.off, off, end) {
			return a.buf[aligned:end], true
		}
	}
}

// AllocAlignedOrErr allocates aligned memory and returns a descriptive error on failure.
func (a *Arena) AllocAlignedOrErr(n, align int) ([]byte, error) {
	if n <= 0 {
		return nil, ErrInvalidAllocSize
	}
	if !isPowerOfTwo(align) {
		return nil, ErrInvalidAlignment
	}
	buf, ok := a.TryAllocAligned(n, align)
	if !ok {
		return nil, ErrArenaOutOfMemory
	}
	return buf, nil
}

// AllocAligned allocates aligned memory and panics on invalid input or OOM.
func (a *Arena) AllocAligned(n, align int) []byte {
	buf, err := a.AllocAlignedOrErr(n, align)
	if err != nil {
		panic(err.Error())
	}
	return buf
}

// Alloc allocates memory globally and panics on invalid size or OOM.
func (a *Arena) Alloc(n int) []byte {
	buf, err := a.AllocOrErr(n)
	if err != nil {
		panic(err.Error())
	}
	return buf
}

func (a *Arena) Reset() {
	atomic.StoreUint64(&a.off, 0)
}

func (a *Arena) Capacity() int {
	return len(a.buf)
}

func (a *Arena) Stats() (used, capacity int) {
	used = int(atomic.LoadUint64(&a.off))
	capacity = len(a.buf)
	return
}

// Mark captures the current arena allocation offset.
func (a *Arena) Mark() ArenaMark {
	return ArenaMark(atomic.LoadUint64(&a.off))
}

// Rewind moves the arena offset back to a previous mark.
func (a *Arena) Rewind(mark ArenaMark) error {
	target := uint64(mark)
	if target > uint64(len(a.buf)) {
		return ErrInvalidArenaMark
	}
	for {
		cur := atomic.LoadUint64(&a.off)
		if target > cur {
			return ErrRewindForward
		}
		if atomic.CompareAndSwapUint64(&a.off, cur, target) {
			return nil
		}
	}
}

// WithScope creates a scope, executes fn, and always exits the scope.
func (a *Arena) WithScope(fn func(*Scope) error) (err error) {
	if fn == nil {
		return nil
	}
	s := a.EnterScope()
	defer func() {
		if s.Active() {
			s.Exit()
		}
	}()
	return fn(s)
}

// ----------------- ScopedArena -----------------

type Scope struct {
	arena  *Arena
	used   uint64
	active uint32
}

// EnterScope returns a new scope. Scope allocations are concurrency-safe.
func (a *Arena) EnterScope() *Scope {
	return &Scope{
		arena:  a,
		active: 1,
	}
}

// TryAlloc allocates memory through the scope and reports whether it succeeded.
func (s *Scope) TryAlloc(n int) ([]byte, bool) {
	if !s.Active() || n <= 0 {
		return nil, false
	}
	buf, ok := s.arena.TryAlloc(n)
	if ok {
		atomic.AddUint64(&s.used, uint64(n))
	}
	return buf, ok
}

// AllocOrErr allocates memory through the scope and returns a descriptive error.
func (s *Scope) AllocOrErr(n int) ([]byte, error) {
	if !s.Active() {
		return nil, ErrScopeInactive
	}
	if n <= 0 {
		return nil, ErrInvalidAllocSize
	}
	buf, ok := s.TryAlloc(n)
	if !ok {
		return nil, ErrArenaOutOfMemory
	}
	return buf, nil
}

// Alloc allocates memory through the scope and panics on invalid size or inactive scope.
func (s *Scope) Alloc(n int) []byte {
	if !s.Active() {
		panic("scope is not active")
	}
	if n <= 0 {
		panic("Alloc size must be > 0")
	}
	buf, ok := s.TryAlloc(n)
	if !ok {
		return nil // prevent panic
	}
	return buf
}

// Active reports whether the scope is still active.
func (s *Scope) Active() bool {
	return atomic.LoadUint32(&s.active) == 1
}

// UsedBytes returns the total bytes successfully allocated through this scope.
func (s *Scope) UsedBytes() int {
	return int(atomic.LoadUint64(&s.used))
}

// Exit marks the scope as inactive.
func (s *Scope) Exit() {
	if !atomic.CompareAndSwapUint32(&s.active, 1, 0) {
		panic("scope already exited")
	}
}

func isPowerOfTwo(v int) bool {
	return v > 0 && (v&(v-1)) == 0
}
