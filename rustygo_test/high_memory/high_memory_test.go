package rustygo_test

import (
	"fmt"
	"math/rand"
	. "rustygo"
	"sync"
	"testing"
	"unsafe"
)

// ---------------------------------------------------
// BigStruct for high memory usage
// ---------------------------------------------------
type BigStruct struct {
	A int
	B [1024]byte // 1 KB
	C [512]byte  // 0.5 KB
}

// ---------------------------------------------------
// Benchmark: High RAM usage with Arena
// ---------------------------------------------------
func BenchmarkArenaHighMemory(b *testing.B) {
	arena := NewArena(512 * 1024 * 1024) // 512 MB arena
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		size := rand.Intn(1024) + 256 // allocate 256~1280 bytes
		_ = arena.Alloc(size)
	}

	b.StopTimer()
	used, capacity := arena.Stats()
	fmt.Printf("[Arena] Used: %.2f MB, Capacity: %.2f MB\n",
		float64(used)/1024/1024, float64(capacity)/1024/1024)
}

// ---------------------------------------------------
// Benchmark: High RAM usage with Pool
// ---------------------------------------------------
func BenchmarkPoolHighMemory(b *testing.B) {
	pool := NewPool[BigStruct](func() *BigStruct { return new(BigStruct) })
	objs := make([]*BigStruct, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := pool.Alloc()
		objs = append(objs, obj)
	}
	b.StopTimer()

	// Show pool stats
	stats := pool.Stats()
	fmt.Printf("[Pool] Total: %.2f MB, Peak: %.2f MB, InUse: %d\n",
		float64(stats.TotalBytes)/1024/1024, float64(stats.PeakBytes)/1024/1024, stats.InUse)

	// Free objects
	for _, obj := range objs {
		pool.Free(obj)
	}
}

// ---------------------------------------------------
// Benchmark: High RAM usage with Heap
// ---------------------------------------------------
func BenchmarkHeapHighMemory(b *testing.B) {
	objs := make([]*BigStruct, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := &BigStruct{}
		objs = append(objs, obj)
	}
	b.StopTimer()

	total := len(objs) * int(unsafe.Sizeof(BigStruct{}))
	fmt.Printf("[Heap] Total memory used: %.2f MB\n", float64(total)/1024/1024)
}

// ---------------------------------------------------
// Concurrent allocation test
// ---------------------------------------------------
func BenchmarkPoolHighMemoryConcurrent(b *testing.B) {
	const workers = 8
	pool := NewPool[BigStruct](func() *BigStruct { return new(BigStruct) })
	var wg sync.WaitGroup

	objs := make([][]*BigStruct, workers)
	for i := range objs {
		objs[i] = make([]*BigStruct, 0, b.N/workers)
	}

	b.ResetTimer()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for i := 0; i < b.N/workers; i++ {
				obj := pool.Alloc()
				objs[idx] = append(objs[idx], obj)
			}
		}(w)
	}
	wg.Wait()
	b.StopTimer()

	// Show pool stats
	stats := pool.Stats()
	fmt.Printf("[Pool-Concurrent] Total: %.2f MB, Peak: %.2f MB, InUse: %d\n",
		float64(stats.TotalBytes)/1024/1024, float64(stats.PeakBytes)/1024/1024, stats.InUse)

	// Free objects
	for w := 0; w < workers; w++ {
		for _, obj := range objs[w] {
			pool.Free(obj)
		}
	}
}
