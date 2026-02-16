package main

import (
	"fmt"
	. "rustygo"
	"sync"
	"time"
	"unsafe"
)

// Struct to allocate
type MyStruct struct {
	A int
	B [64]byte
}

func main() {
	const goroutines = 8
	const totalObjects = 10_000_000
	const allocSize = int(unsafe.Sizeof(MyStruct{}))

	fmt.Printf("Running allocation speed stress test for %d objects with %d goroutines...\n\n", totalObjects, goroutines)

	// -----------------------------------
	// 1️⃣ ScopedArena Test
	// -----------------------------------
	arena := NewArena(totalObjects * allocSize) // pre-allocate exact needed space
	perG := totalObjects / goroutines

	start := time.Now()
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope := arena.EnterScope()
			for i := 0; i < perG; i++ {
				scope.Alloc(allocSize)
			}
			scope.Exit()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	used, capacity := arena.Stats()
	fmt.Printf("[ScopedArena] Time: %v, Used: %.2f MB / Capacity: %.2f MB, Per Object: %.3f ns\n",
		elapsed, float64(used)/1024/1024, float64(capacity)/1024/1024,
		float64(elapsed.Nanoseconds())/float64(totalObjects),
	)

	// -----------------------------------
	// 2️⃣ Pool Test
	// -----------------------------------
	pool := NewPool[MyStruct](func() *MyStruct { return new(MyStruct) })
	wgObjs := make([][]*MyStruct, goroutines)
	start = time.Now()
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
	elapsed = time.Since(start)
	stats := pool.Stats()
	fmt.Printf("[Pool] Time: %v, Total: %.2f MB, Peak: %.2f MB, InUse: %d, Per Object: %.3f ns\n",
		elapsed, float64(stats.TotalBytes)/1024/1024, float64(stats.PeakBytes)/1024/1024, stats.InUse,
		float64(elapsed.Nanoseconds())/float64(totalObjects),
	)
	// Free all objects
	for g := 0; g < goroutines; g++ {
		for _, obj := range wgObjs[g] {
			pool.Free(obj)
		}
	}

	// -----------------------------------
	// 3️⃣ Heap Test
	// -----------------------------------
	wgObjs = make([][]*MyStruct, goroutines)
	start = time.Now()
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
	elapsed = time.Since(start)

	// Memory summary
	total := 0
	for g := 0; g < goroutines; g++ {
		total += len(wgObjs[g]) * allocSize
	}
	fmt.Printf("[Heap] Time: %v, Total memory used: %.2f MB, Per Object: %.3f ns\n",
		elapsed, float64(total)/1024/1024, float64(elapsed.Nanoseconds())/float64(totalObjects),
	)
}
