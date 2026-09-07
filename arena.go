package rustygo

import (
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

type chunk struct {
	buf     []byte
	base    uintptr
	off     uint64
	release func([]byte) error
}

func (c *chunk) tryAlloc(n int) ([]byte, bool) {
	if n <= 0 {
		return nil, false
	}
	req := uint64(n)
	capacity := uint64(len(c.buf))
	for {
		off := atomic.LoadUint64(&c.off)
		end := off + req
		if end < off || end > capacity {
			return nil, false
		}
		if atomic.CompareAndSwapUint64(&c.off, off, end) {
			return c.buf[off:end], true
		}
	}
}

func (c *chunk) tryAllocAligned(n, align int) ([]byte, bool) {
	if n <= 0 || !isPowerOfTwo(align) {
		return nil, false
	}
	req := uint64(n)
	mask := uintptr(align - 1)
	capacity := uint64(len(c.buf))
	for {
		off := atomic.LoadUint64(&c.off)
		curAddr := c.base + uintptr(off)
		alignedAddr := (curAddr + mask) & ^mask
		aligned := uint64(alignedAddr - c.base)
		end := aligned + req
		if aligned < off || end < aligned || end > capacity {
			return nil, false
		}
		if atomic.CompareAndSwapUint64(&c.off, off, end) {
			return c.buf[aligned:end], true
		}
	}
}

// Arena is a memory arena supporting dynamic slab/chunk growth.
type Arena struct {
	mu        sync.Mutex
	chunks    []*chunk
	active    atomic.Pointer[chunk]
	chunkSize int
	maxCap    int
	closed    uint32
}

type ArenaOption func(*Arena)

func WithChunkSize(size int) ArenaOption {
	return func(a *Arena) {
		if size > 0 {
			a.chunkSize = size
		}
	}
}

func WithMaxCapacity(maxCap int) ArenaOption {
	return func(a *Arena) {
		if maxCap > 0 {
			a.maxCap = maxCap
		}
	}
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

func makeMark(chunkIdx int, off uint64) ArenaMark {
	return ArenaMark((uint64(chunkIdx) << 32) | (off & 0xFFFFFFFF))
}

func parseMark(mark ArenaMark) (chunkIdx int, off uint64) {
	u := uint64(mark)
	return int(u >> 32), u & 0xFFFFFFFF
}

func newChunk(size int) (*chunk, error) {
	buf, release, err := allocArenaBuffer(size)
	if err != nil {
		return nil, err
	}
	return &chunk{
		buf:     buf,
		base:    uintptr(unsafe.Pointer(&buf[0])),
		release: release,
	}, nil
}

func NewArena(size int, opts ...ArenaOption) *Arena {
	if size <= 0 {
		panic("arena size must be > 0")
	}
	a := &Arena{
		chunkSize: size,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}

	chk, err := newChunk(a.chunkSize)
	if err != nil {
		panic(err.Error())
	}
	a.chunks = []*chunk{chk}
	a.active.Store(chk)

	runtime.SetFinalizer(a, func(arena *Arena) {
		_ = arena.Close()
	})
	return a
}

func NewFixedArena(size int) *Arena {
	return NewArena(size, WithMaxCapacity(size))
}

// TryAlloc allocates memory globally and reports whether it succeeded.
func (a *Arena) TryAlloc(n int) ([]byte, bool) {
	if n <= 0 {
		return nil, false
	}
	act := a.active.Load()
	if act != nil {
		if buf, ok := act.tryAlloc(n); ok {
			return buf, true
		}
	}
	return a.allocSlow(n, 1)
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
	act := a.active.Load()
	if act != nil {
		if buf, ok := act.tryAllocAligned(n, align); ok {
			return buf, true
		}
	}
	return a.allocSlow(n, align)
}

func (a *Arena) allocSlow(n, align int) ([]byte, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	act := a.active.Load()
	if act != nil {
		var buf []byte
		var ok bool
		if align > 1 {
			buf, ok = act.tryAllocAligned(n, align)
		} else {
			buf, ok = act.tryAlloc(n)
		}
		if ok {
			return buf, true
		}
	}

	for i, chk := range a.chunks {
		if chk == act {
			for j := i + 1; j < len(a.chunks); j++ {
				nextChk := a.chunks[j]
				var buf []byte
				var ok bool
				if align > 1 {
					buf, ok = nextChk.tryAllocAligned(n, align)
				} else {
					buf, ok = nextChk.tryAlloc(n)
				}
				if ok {
					a.active.Store(nextChk)
					return buf, true
				}
			}
			break
		}
	}

	newSize := a.chunkSize
	if n > newSize {
		newSize = n
	}

	if a.maxCap > 0 {
		curCap := 0
		for _, chk := range a.chunks {
			curCap += len(chk.buf)
		}
		if curCap+newSize > a.maxCap {
			return nil, false
		}
	}

	newChk, err := newChunk(newSize)
	if err != nil {
		return nil, false
	}

	a.chunks = append(a.chunks, newChk)
	a.active.Store(newChk)

	if align > 1 {
		return newChk.tryAllocAligned(n, align)
	}
	return newChk.tryAlloc(n)
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
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.chunks {
		atomic.StoreUint64(&c.off, 0)
	}
	if len(a.chunks) > 0 {
		a.active.Store(a.chunks[0])
	}
}

// Close releases the arena backing memory to the operating system.
func (a *Arena) Close() error {
	if a == nil {
		return nil
	}
	if !atomic.CompareAndSwapUint32(&a.closed, 0, 1) {
		return nil
	}
	runtime.SetFinalizer(a, nil)

	a.mu.Lock()
	chunks := a.chunks
	a.chunks = nil
	a.active.Store(nil)
	a.mu.Unlock()

	var firstErr error
	for _, c := range chunks {
		if c.release != nil && len(c.buf) > 0 {
			if err := c.release(c.buf); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		c.buf = nil
		c.base = 0
		atomic.StoreUint64(&c.off, 0)
	}
	return firstErr
}

func (a *Arena) Capacity() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	tot := 0
	for _, c := range a.chunks {
		tot += len(c.buf)
	}
	return tot
}

func (a *Arena) Stats() (used, capacity int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.chunks {
		used += int(atomic.LoadUint64(&c.off))
		capacity += len(c.buf)
	}
	return
}

// Mark captures the current arena allocation offset.
func (a *Arena) Mark() ArenaMark {
	a.mu.Lock()
	defer a.mu.Unlock()
	act := a.active.Load()
	for i, c := range a.chunks {
		if c == act {
			return makeMark(i, atomic.LoadUint64(&c.off))
		}
	}
	if len(a.chunks) > 0 {
		return makeMark(0, atomic.LoadUint64(&a.chunks[0].off))
	}
	return makeMark(0, 0)
}

// Rewind moves the arena offset back to a previous mark.
func (a *Arena) Rewind(mark ArenaMark) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	targetIdx, targetOff := parseMark(mark)
	if targetIdx < 0 || targetIdx >= len(a.chunks) {
		return ErrInvalidArenaMark
	}
	targetChunk := a.chunks[targetIdx]
	if targetOff > uint64(len(targetChunk.buf)) {
		return ErrInvalidArenaMark
	}

	act := a.active.Load()
	actIdx := 0
	for i, c := range a.chunks {
		if c == act {
			actIdx = i
			break
		}
	}

	if targetIdx > actIdx {
		return ErrRewindForward
	}
	if targetIdx == actIdx && targetOff > atomic.LoadUint64(&act.off) {
		return ErrRewindForward
	}

	for i := targetIdx + 1; i < len(a.chunks); i++ {
		atomic.StoreUint64(&a.chunks[i].off, 0)
	}

	atomic.StoreUint64(&targetChunk.off, targetOff)
	a.active.Store(targetChunk)

	return nil
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
	arena    *Arena
	used     uint64
	active   uint32
	cleanups []func()
}

// EnterScope returns a new scope. Scope allocations are concurrency-safe.
func (a *Arena) EnterScope() *Scope {
	return &Scope{
		arena:  a,
		active: 1,
	}
}

// OnExit registers a function to run when the scope exits.
func (s *Scope) OnExit(fn func()) {
	s.cleanups = append(s.cleanups, fn)
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

// Exit marks the scope as inactive and runs all registered cleanups in reverse order.
func (s *Scope) Exit() {
	if !atomic.CompareAndSwapUint32(&s.active, 1, 0) {
		panic("scope already exited")
	}
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
	s.cleanups = nil
}

var (
	mapPools  sync.Map
	chanPools sync.Map
)

func getMapPool(mapType reflect.Type) *sync.Pool {
	if p, ok := mapPools.Load(mapType); ok {
		return p.(*sync.Pool)
	}
	p := &sync.Pool{
		New: func() any {
			return reflect.MakeMap(mapType).Interface()
		},
	}
	actual, _ := mapPools.LoadOrStore(mapType, p)
	return actual.(*sync.Pool)
}

func getChanPool(chanType reflect.Type, cap int) *sync.Pool {
	type key struct {
		t   reflect.Type
		cap int
	}
	k := key{t: chanType, cap: cap}
	if p, ok := chanPools.Load(k); ok {
		return p.(*sync.Pool)
	}
	p := &sync.Pool{
		New: func() any {
			return reflect.MakeChan(chanType, cap).Interface()
		},
	}
	actual, _ := chanPools.LoadOrStore(k, p)
	return actual.(*sync.Pool)
}

func AllocMap[K comparable, V any](s *Scope) map[K]V {
	if !s.Active() {
		panic("scope is not active")
	}
	var zero map[K]V
	t := reflect.TypeOf(zero)
	if t == nil {
		return make(map[K]V)
	}
	pool := getMapPool(t)
	m := pool.Get().(map[K]V)
	s.OnExit(func() {
		clear(m)
		pool.Put(m)
	})
	return m
}

func AllocChan[T any](s *Scope, cap int) chan T {
	if !s.Active() {
		panic("scope is not active")
	}
	var zero chan T
	t := reflect.TypeOf(zero)
	if t == nil {
		return make(chan T, cap)
	}
	pool := getChanPool(t, cap)
	ch := pool.Get().(chan T)
	s.OnExit(func() {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			default:
				pool.Put(ch)
				return
			}
		}
	})
	return ch
}

func isPowerOfTwo(v int) bool {
	return v > 0 && (v&(v-1)) == 0
}

// Region is the zero-config arena+scope wrapper for the common single-lifetime case.
type Region struct {
	arena *Arena
	scope *Scope
}

// NewRegion creates a region backed by a fresh arena and entered scope.
func NewRegion(size int) *Region {
	arena := NewArena(size)
	return &Region{
		arena: arena,
		scope: arena.EnterScope(),
	}
}

// Scope returns the region's active scope.
func (r *Region) Scope() *Scope {
	if r == nil {
		return nil
	}
	return r.scope
}

// Reset clears the region's arena and enters a fresh scope for reuse.
func (r *Region) Reset() {
	if r == nil || r.arena == nil {
		return
	}
	if r.scope != nil && r.scope.Active() {
		r.scope.Exit()
	}
	r.arena.Reset()
	r.scope = r.arena.EnterScope()
}

// Done closes the active scope and releases the arena backing memory.
func (r *Region) Done() error {
	if r == nil {
		return nil
	}
	if r.scope != nil && r.scope.Active() {
		r.scope.Exit()
	}
	r.scope = nil
	if r.arena == nil {
		return nil
	}
	err := r.arena.Close()
	r.arena = nil
	return err
}

// New allocates storage for a single T from the region.
func New[T any](r *Region) *T {
	if r == nil || r.scope == nil {
		panic("region is not active")
	}
	return AllocValue[T](r.scope)
}

// Slice allocates a slice with len==cap==n from the region.
func Slice[T any](r *Region, n int) []T {
	if r == nil || r.scope == nil {
		panic("region is not active")
	}
	return AllocSlice[T](r.scope, n)
}

// SliceCap allocates a slice with the requested length and capacity from the region.
func SliceCap[T any](r *Region, length, capacity int) []T {
	if r == nil || r.scope == nil {
		panic("region is not active")
	}
	return AllocSliceCap[T](r.scope, length, capacity)
}

// HasPointersReflect reports whether type t contains pointers that require GC scanning.
func HasPointersReflect(t reflect.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Chan, reflect.Interface, reflect.Func, reflect.String, reflect.UnsafePointer:
		return true
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if HasPointersReflect(t.Field(i).Type) {
				return true
			}
		}
	case reflect.Array:
		return HasPointersReflect(t.Elem())
	}
	return false
}

// AllocValue allocates storage for a single T through the scope.
func AllocValue[T any](s *Scope) *T {
	if !s.Active() {
		panic("scope is not active")
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t != nil && HasPointersReflect(t) {
		return new(T)
	}
	size := int(unsafe.Sizeof(zero))
	if size == 0 {
		return new(T)
	}
	align := int(unsafe.Alignof(zero))
	buf := s.arena.AllocAligned(size, align)
	atomic.AddUint64(&s.used, uint64(len(buf)))
	return (*T)(unsafe.Pointer(&buf[0]))
}

// AllocSlice allocates a slice with length and capacity n through the scope.
func AllocSlice[T any](s *Scope, n int) []T {
	return AllocSliceCap[T](s, n, n)
}

// AllocSliceCap allocates a slice with the requested length and capacity through the scope.
func AllocSliceCap[T any](s *Scope, length, capacity int) []T {
	if !s.Active() {
		panic("scope is not active")
	}
	if length < 0 || capacity < length {
		panic("invalid slice bounds")
	}
	var zero T
	t := reflect.TypeOf(zero)
	if t != nil && HasPointersReflect(t) {
		return make([]T, length, capacity)
	}
	size := int(unsafe.Sizeof(zero))
	if size == 0 {
		return make([]T, length, capacity)
	}
	align := int(unsafe.Alignof(zero))
	buf := s.arena.AllocAligned(size*capacity, align)
	atomic.AddUint64(&s.used, uint64(len(buf)))
	return unsafe.Slice((*T)(unsafe.Pointer(&buf[0])), capacity)[:length:capacity]
}
