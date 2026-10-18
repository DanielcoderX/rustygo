package rustygo

import (
	"errors"
	"sync/atomic"
)

var (
	ErrInvalidSavepoint = errors.New("rustygo/checkpoint: invalid or stale savepoint")
)

// Savepoint records an instantaneous snapshot of all arena chunk allocation offsets.
type Savepoint struct {
	arena          *Arena
	offsets        []uint64
	activeChunkIdx int
}

// Savepoint captures the current allocation watermark across all arena chunks.
func (a *Arena) Savepoint() *Savepoint {
	a.mu.Lock()
	defer a.mu.Unlock()

	sp := &Savepoint{
		arena:   a,
		offsets: make([]uint64, len(a.chunks)),
	}

	act := a.active.Load()
	for i, c := range a.chunks {
		sp.offsets[i] = atomic.LoadUint64(&c.off)
		if c == act {
			sp.activeChunkIdx = i
		}
	}

	return sp
}

// Rollback restores all chunk allocation offsets back to the saved watermark.
// All memory allocated after the Savepoint is invalidated and immediately available for reuse.
func (a *Arena) Rollback(sp *Savepoint) error {
	if sp == nil || sp.arena != a {
		return ErrInvalidSavepoint
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if len(sp.offsets) > len(a.chunks) {
		return ErrInvalidSavepoint
	}

	// Restore offsets of chunks present when savepoint was created
	for i, off := range sp.offsets {
		atomic.StoreUint64(&a.chunks[i].off, off)
	}

	// Any chunks dynamically grown after the savepoint are reset to 0
	for i := len(sp.offsets); i < len(a.chunks); i++ {
		atomic.StoreUint64(&a.chunks[i].off, 0)
	}

	// Restore active chunk
	if sp.activeChunkIdx >= 0 && sp.activeChunkIdx < len(a.chunks) {
		a.active.Store(a.chunks[sp.activeChunkIdx])
	}

	return nil
}

// Transaction executes fn within an isolated transactional boundary.
// If fn returns an error or panics, the arena is automatically rolled back to its pre-transaction state.
func (a *Arena) Transaction(fn func(a *Arena) error) (err error) {
	sp := a.Savepoint()

	defer func() {
		if r := recover(); r != nil {
			_ = a.Rollback(sp)
			panic(r)
		}
	}()

	if err := fn(a); err != nil {
		_ = a.Rollback(sp)
		return err
	}

	return nil
}
