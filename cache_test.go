package rustygo_test

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	rg "rustygo"
)

func TestArenaCacheBasicAndRotation(t *testing.T) {
	// Small 2KB generation limit to easily trigger rotations
	cache := rg.NewArenaCache(2 * 1024)
	defer cache.Close()

	// Insert items
	cache.Set("key1", []byte("val1"))
	cache.Set("key2", []byte("val2"))

	v, ok := cache.Get("key1")
	if !ok || !bytes.Equal(v, []byte("val1")) {
		t.Fatalf("expected val1, got %s (ok=%v)", v, ok)
	}

	// Trigger generational rotation by inserting enough data
	largePayload := make([]byte, 1024)
	cache.Set("large1", largePayload)
	cache.Set("large2", largePayload) // triggers swap

	// key1 should still be accessible from old generation and promoted
	v, ok = cache.Get("key1")
	if !ok || !bytes.Equal(v, []byte("val1")) {
		t.Fatalf("expected key1 to survive first rotation")
	}

	// Trigger second rotation which will wipe old generation
	cache.Set("large3", largePayload)
	cache.Set("large4", largePayload)

	// key2 was not accessed and should be evicted
	_, ok = cache.Get("key2")
	if ok {
		t.Fatalf("expected key2 to be evicted after multiple rotations")
	}
}

func TestArenaCacheConcurrent(t *testing.T) {
	cache := rg.NewArenaCache(64 * 1024)
	defer cache.Close()

	const workers = 50
	const ops = 200

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("k-%d-%d", id, i%20)
				val := []byte(fmt.Sprintf("v-%d-%d", id, i))
				cache.Set(key, val)
				_, _ = cache.Get(key)
			}
		}(w)
	}

	wg.Wait()
}

func TestArenaCacheGetScope(t *testing.T) {
	cache := rg.NewArenaCache(16 * 1024)
	defer cache.Close()

	cache.Set("token", []byte("bearer-xyz-123"))

	arena := rg.NewArena(4096)
	defer arena.Close()
	scope := arena.EnterScope()
	defer scope.Exit()

	val, ok := cache.GetScope("token", scope)
	if !ok || !bytes.Equal(val, []byte("bearer-xyz-123")) {
		t.Fatalf("expected bearer-xyz-123, got %s", val)
	}
}
