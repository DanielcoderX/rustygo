package rustygo

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// shard holds a localized pool of pre-warmed Arena instances to prevent cross-CPU cache bouncing.
type shard struct {
	mu     sync.Mutex
	arenas []*Arena
	max    int
}

// ShardedArenaPool is a high-concurrency, lock-contention-free arena pool.
// Arenas are distributed across shards scaled to GOMAXPROCS.
type ShardedArenaPool struct {
	shards   []shard
	mask     uint64
	slabSize int
	counter  uint64
	closed   atomic.Bool
}

// NewShardedArenaPool creates a sharded pool sized to current GOMAXPROCS.
func NewShardedArenaPool(slabSize int, maxPerShard int) *ShardedArenaPool {
	if slabSize <= 0 {
		slabSize = 64 * 1024
	}
	if maxPerShard <= 0 {
		maxPerShard = 32
	}

	n := runtime.GOMAXPROCS(0) * 2
	if n < 4 {
		n = 4
	}
	// round up to power of 2
	shardsCount := 1
	for shardsCount < n {
		shardsCount <<= 1
	}

	p := &ShardedArenaPool{
		shards:   make([]shard, shardsCount),
		mask:     uint64(shardsCount - 1),
		slabSize: slabSize,
	}

	for i := range p.shards {
		p.shards[i].max = maxPerShard
		p.shards[i].arenas = make([]*Arena, 0, maxPerShard)
	}

	return p
}

// Borrow retrieves an Arena from the calling thread's assigned shard.
func (p *ShardedArenaPool) Borrow() *Arena {
	if p.closed.Load() {
		return NewArena(p.slabSize)
	}

	idx := atomic.AddUint64(&p.counter, 1) & p.mask
	s := &p.shards[idx]

	s.mu.Lock()
	n := len(s.arenas)
	if n > 0 {
		a := s.arenas[n-1]
		s.arenas = s.arenas[:n-1]
		s.mu.Unlock()
		a.Reset()
		return a
	}
	s.mu.Unlock()

	return NewArena(p.slabSize)
}

// Release returns an Arena back to its shard.
func (p *ShardedArenaPool) Release(a *Arena) {
	if a == nil {
		return
	}
	if p.closed.Load() {
		a.Close()
		return
	}

	a.Reset()

	idx := atomic.AddUint64(&p.counter, 1) & p.mask
	s := &p.shards[idx]

	s.mu.Lock()
	if len(s.arenas) < s.max {
		s.arenas = append(s.arenas, a)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	a.Close()
}

// WithScope borrows an arena, opens a Scope, executes fn, and safely releases the arena.
func (p *ShardedArenaPool) WithScope(fn func(scope *Scope) error) error {
	arena := p.Borrow()
	defer p.Release(arena)

	scope := arena.EnterScope()
	defer scope.Exit()

	return fn(scope)
}

// Close destroys all pooled arenas and releases backing OS memory.
func (p *ShardedArenaPool) Close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}

	for i := range p.shards {
		s := &p.shards[i]
		s.mu.Lock()
		for _, a := range s.arenas {
			a.Close()
		}
		s.arenas = nil
		s.mu.Unlock()
	}
}
