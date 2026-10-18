package rustygo

import (
	"sync"
	"unsafe"
)

type cacheEntry struct {
	key []byte
	val []byte
}

type cacheGeneration struct {
	arena   *Arena
	scope   *Scope
	entries map[string]cacheEntry
	used    int
}

func newCacheGeneration(size int) *cacheGeneration {
	a := NewArena(size)
	return &cacheGeneration{
		arena:   a,
		scope:   a.EnterScope(),
		entries: make(map[string]cacheEntry),
		used:    0,
	}
}

func (g *cacheGeneration) reset() {
	if g.scope.Active() {
		g.scope.Exit()
	}
	g.arena.Reset()
	g.scope = g.arena.EnterScope()
	g.entries = make(map[string]cacheEntry)
	g.used = 0
}

func (g *cacheGeneration) close() {
	if g.scope.Active() {
		g.scope.Exit()
	}
	_ = g.arena.Close()
}

// ArenaCache is a zero-GC generational cache.
// Memory is partitioned into two rotating arenas, completely avoiding Go GC pointer scanning.
type ArenaCache struct {
	mu            sync.RWMutex
	active        *cacheGeneration
	old           *cacheGeneration
	maxGenBytes   int
	closed        bool
}

// NewArenaCache creates a new generational arena cache with the specified byte limit per generation.
func NewArenaCache(maxGenBytes int) *ArenaCache {
	if maxGenBytes <= 0 {
		maxGenBytes = 8 * 1024 * 1024 // 8 MB default
	}
	return &ArenaCache{
		active:      newCacheGeneration(maxGenBytes),
		old:         newCacheGeneration(maxGenBytes),
		maxGenBytes: maxGenBytes,
	}
}

// Set stores a key-value pair in the active arena generation.
func (c *ArenaCache) Set(key string, val []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	entryBytes := len(key) + len(val)
	if c.active.used+entryBytes > c.maxGenBytes {
		// Rotate generations: old is cleared, active becomes old, old becomes new active
		c.old.reset()
		c.old, c.active = c.active, c.old
	}

	kBuf := AllocSlice[byte](c.active.scope, len(key))
	copy(kBuf, key)

	vBuf := AllocSlice[byte](c.active.scope, len(val))
	copy(vBuf, val)

	kStr := unsafe.String(&kBuf[0], len(kBuf))
	c.active.entries[kStr] = cacheEntry{
		key: kBuf,
		val: vBuf,
	}
	c.active.used += entryBytes
}

// Get retrieves a value from the cache.
func (c *ArenaCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, false
	}

	// Check active generation
	if e, ok := c.active.entries[key]; ok {
		c.mu.RUnlock()
		return e.val, true
	}

	// Check old generation
	if e, ok := c.old.entries[key]; ok {
		c.mu.RUnlock()
		// Promote to active generation asynchronously or on next lock
		c.Set(key, e.val)
		return e.val, true
	}

	c.mu.RUnlock()
	return nil, false
}

// GetScope copies the cached value directly into the caller's Scope without heap escapes.
func (c *ArenaCache) GetScope(key string, scope *Scope) ([]byte, bool) {
	val, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	if scope == nil {
		return val, true
	}

	buf := AllocSlice[byte](scope, len(val))
	copy(buf, val)
	return buf, true
}

// Delete removes an entry from both generations.
func (c *ArenaCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.active.entries, key)
	delete(c.old.entries, key)
}

// Len returns the total number of entries in the active generation.
func (c *ArenaCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.active.entries)
}

// Close destroys both generations and frees all OS memory.
func (c *ArenaCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.closed = true
	c.active.close()
	c.old.close()
}
