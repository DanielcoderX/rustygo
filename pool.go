package rustygo

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// -----------------------------
// Generic pool (Treiber lock-free or sync.Pool backend)
// -----------------------------

type poolNode[T any] struct {
	val  *T
	next *poolNode[T]
}

type PoolBackend uint8

const (
	PoolBackendTreiber PoolBackend = iota
	PoolBackendSync
)

type PoolOption[T any] func(*Pool[T])

func WithPoolBackend[T any](backend PoolBackend) PoolOption[T] {
	return func(p *Pool[T]) {
		p.backend = backend
	}
}

func WithResetFn[T any](resetFn func(*T)) PoolOption[T] {
	return func(p *Pool[T]) {
		p.resetFn = resetFn
	}
}

func WithZeroOnFree[T any]() PoolOption[T] {
	return func(p *Pool[T]) {
		p.zeroOnFree = true
	}
}

func WithPoisonOnFree[T any](pattern byte) PoolOption[T] {
	return func(p *Pool[T]) {
		p.poisonOnFree = true
		p.poisonPattern = pattern
	}
}

type Pool[T any] struct {
	head atomic.Pointer[poolNode[T]]
	pool sync.Pool

	backend PoolBackend
	newFn   func() *T
	resetFn func(*T)

	zeroOnFree    bool
	poisonOnFree  bool
	poisonPattern byte

	trackerMu sync.Mutex
	tracker   map[uintptr]struct{}

	inUse int64
	total int64
	peak  int64
}

func NewPool[T any](newFn func() *T, opts ...PoolOption[T]) *Pool[T] {
	if newFn == nil {
		panic("Pool requires a constructor")
	}
	p := &Pool[T]{
		newFn:   newFn,
		backend: PoolBackendTreiber,
	}
	if poolDebug {
		p.tracker = make(map[uintptr]struct{})
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

func NewSyncPool[T any](newFn func() *T, opts ...PoolOption[T]) *Pool[T] {
	opts = append([]PoolOption[T]{WithPoolBackend[T](PoolBackendSync)}, opts...)
	return NewPool(newFn, opts...)
}

func (p *Pool[T]) Alloc() *T {
	var obj *T
	switch p.backend {
	case PoolBackendSync:
		if v := p.pool.Get(); v != nil {
			obj = v.(*T)
		} else {
			atomic.AddInt64(&p.total, 1)
			obj = p.newFn()
		}
	default:
		obj = p.allocTreiber()
	}
	if poolDebug {
		p.debugMarkAlloc(obj)
	}
	in := atomic.AddInt64(&p.inUse, 1)
	updatePeak64(&p.peak, in)
	return obj
}

// Borrow is an intent-based alias for Alloc.
func (p *Pool[T]) Borrow() *T {
	return p.Alloc()
}

func (p *Pool[T]) allocTreiber() *T {
	for {
		h := p.head.Load()
		if h == nil {
			atomic.AddInt64(&p.total, 1)
			return p.newFn()
		}
		if p.head.CompareAndSwap(h, h.next) {
			return h.val
		}
	}
}

func (p *Pool[T]) Free(obj *T) {
	if obj == nil {
		panic("Free(nil)")
	}
	if p.resetFn != nil {
		p.resetFn(obj)
	}
	if p.zeroOnFree {
		*obj = *new(T)
	}
	if p.poisonOnFree {
		poisonObject(obj, p.poisonPattern)
	}
	if poolDebug {
		p.debugMarkFree(obj)
	}
	switch p.backend {
	case PoolBackendSync:
		p.pool.Put(obj)
	default:
		n := &poolNode[T]{val: obj}
		for {
			h := p.head.Load()
			n.next = h
			if p.head.CompareAndSwap(h, n) {
				break
			}
		}
	}
	atomic.AddInt64(&p.inUse, -1)
}

// Release is an intent-based alias for Free.
func (p *Pool[T]) Release(obj *T) {
	p.Free(obj)
}

// WithBorrow borrows an object, executes fn, and always releases the object.
func (p *Pool[T]) WithBorrow(fn func(*T) error) error {
	if fn == nil {
		return nil
	}
	obj := p.Borrow()
	defer p.Release(obj)
	return fn(obj)
}

type PoolStats struct {
	Backend      PoolBackend
	TotalObjects int
	PeakObjects  int
	TotalBytes   int
	PeakBytes    int
	InUse        int
}

func (p *Pool[T]) Stats() PoolStats {
	size := int(unsafe.Sizeof(*new(T)))
	totalObjects := int(atomic.LoadInt64(&p.total))
	peakObjects := int(atomic.LoadInt64(&p.peak))
	return PoolStats{
		Backend:      p.backend,
		TotalObjects: totalObjects,
		PeakObjects:  peakObjects,
		TotalBytes:   totalObjects * size,
		PeakBytes:    peakObjects * size,
		InUse:        int(atomic.LoadInt64(&p.inUse)),
	}
}

// Peak helper
func updatePeak64(peak *int64, val int64) {
	for {
		old := atomic.LoadInt64(peak)
		if val <= old {
			return
		}
		if atomic.CompareAndSwapInt64(peak, old, val) {
			return
		}
	}
}

func (p *Pool[T]) debugMarkAlloc(obj *T) {
	ptr := uintptr(unsafe.Pointer(obj))
	p.trackerMu.Lock()
	p.tracker[ptr] = struct{}{}
	p.trackerMu.Unlock()
}

func (p *Pool[T]) debugMarkFree(obj *T) {
	ptr := uintptr(unsafe.Pointer(obj))
	p.trackerMu.Lock()
	_, ok := p.tracker[ptr]
	if !ok {
		p.trackerMu.Unlock()
		panic("double free or foreign object passed to Pool.Free")
	}
	delete(p.tracker, ptr)
	p.trackerMu.Unlock()
}

func poisonObject[T any](obj *T, pattern byte) {
	size := int(unsafe.Sizeof(*obj))
	raw := unsafe.Slice((*byte)(unsafe.Pointer(obj)), size)
	for i := range raw {
		raw[i] = pattern
	}
}
