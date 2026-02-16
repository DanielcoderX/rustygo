package rustygo_test

import (
	"fmt"
	. "rustygo"
	"sync"
	"testing"
	"unsafe"
)

// ---------------------------------------------------
// Struct for testing
// ---------------------------------------------------
type MyStruct struct {
	A int
	B [64]byte
}

// ---------------------------------------------------
// Benchmark: ScopedArena allocation with multiple goroutines
// ---------------------------------------------------
func BenchmarkScopedArenaAllocConcurrent(b *testing.B) {
	arena := NewArena(128 * 1024 * 1024) // 128 MB arena
	const goroutines = 8
	const allocSize = int(unsafe.Sizeof(MyStruct{})) // size of struct

	// Calculate safe per-goroutine allocation to avoid OOM
	totalAllocCap := arena.Capacity() / allocSize
	perG := b.N / goroutines
	maxPerG := totalAllocCap / goroutines
	if perG > maxPerG {
		perG = maxPerG
	}

	var wg sync.WaitGroup
	b.ResetTimer()
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope := arena.EnterScope()
			defer func() {
				if r := recover(); r != nil {
					b.Logf("scope.Alloc recovered from panic: %v", r)
				}
			}()
			for i := 0; i < perG; i++ {
				buf := scope.Alloc(allocSize)
				if buf == nil {
					break // arena full
				}
			}
			scope.Exit()
		}()
	}
	wg.Wait()
	b.StopTimer()

	used, capacity := arena.Stats()
	fmt.Printf("[ScopedArena-Concurrent] Used: %.2f MB, Capacity: %.2f MB\n",
		float64(used)/1024/1024, float64(capacity)/1024/1024)
}

// ---------------------------------------------------
// Benchmark: Pool allocation with multiple goroutines
// ---------------------------------------------------
func BenchmarkStructPoolAllocConcurrent(b *testing.B) {
	const goroutines = 8
	perG := b.N / goroutines

	pool := NewPool[MyStruct](func() *MyStruct { return new(MyStruct) })
	var wg sync.WaitGroup
	wgObjs := make([][]*MyStruct, goroutines)

	b.ResetTimer()
	for g := 0; g < goroutines; g++ {
		wgObjs[g] = make([]*MyStruct, 0, perG)
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				obj := pool.Alloc()
				wgObjs[id] = append(wgObjs[id], obj)
			}
		}(g)
	}
	wg.Wait()
	b.StopTimer()

	// Print pool memory stats after concurrent allocation
	stats := pool.Stats()
	fmt.Printf("[Pool-Concurrent] Total: %.2f MB, Peak: %.2f MB, InUse: %d\n",
		float64(stats.TotalBytes)/1024/1024, float64(stats.PeakBytes)/1024/1024, stats.InUse)

	// Return all objects to pool
	for g := 0; g < goroutines; g++ {
		for _, obj := range wgObjs[g] {
			pool.Free(obj)
		}
	}
}

// ---------------------------------------------------
// Benchmark: Heap allocation with multiple goroutines
// ---------------------------------------------------
func BenchmarkHeapAllocConcurrent(b *testing.B) {
	const goroutines = 8
	perG := b.N / goroutines

	var wg sync.WaitGroup
	wgObjs := make([][]*MyStruct, goroutines)

	b.ResetTimer()
	for g := 0; g < goroutines; g++ {
		wgObjs[g] = make([]*MyStruct, 0, perG)
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				obj := &MyStruct{A: i}
				wgObjs[id] = append(wgObjs[id], obj)
			}
		}(g)
	}
	wg.Wait()
	b.StopTimer()

	// Memory summary printed once
	total := 0
	for g := 0; g < goroutines; g++ {
		total += len(wgObjs[g]) * int(unsafe.Sizeof(MyStruct{}))
	}
	fmt.Printf("[Heap-Concurrent] Total memory used: %.2f MB\n", float64(total)/1024/1024)
}
