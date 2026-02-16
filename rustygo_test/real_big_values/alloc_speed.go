package main

import (
	"fmt"
	. "rustygo"
	"sync"
	"time"
	"unsafe"
)

// Struct we’re allocating
type MyStruct struct {
	A int64
	B [128]byte // slightly bigger to ramp up memory usage
}

func main() {
	const goroutines = 8
	const totalObjects = 50_000_000 // ~6GB (50M * size of MyStruct)
	const allocSize = int(unsafe.Sizeof(MyStruct{}))

	fmt.Printf("Running extreme allocation test for %d objects (~6GB) with %d goroutines...\n\n", totalObjects, goroutines)

	perG := totalObjects / goroutines

	// -----------------------------------
	// 1️⃣ ScopedArena Test
	// -----------------------------------
	fmt.Println("=== ScopedArena Test ===")
	arena := NewArena(totalObjects * allocSize)
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
	fmt.Printf("[ScopedArena] Time: %v, Used: %.2f GB / Capacity: %.2f GB, Per Object: %.3f ns\n\n",
		elapsed, float64(used)/1024/1024/1024, float64(capacity)/1024/1024/1024,
		float64(elapsed.Nanoseconds())/float64(totalObjects),
	)

	// -----------------------------------
	// 2️⃣ Heap Test
	// -----------------------------------
	fmt.Println("=== Heap Test ===")
	wgObjs := make([][]*MyStruct, goroutines)
	start = time.Now()
	for g := 0; g < goroutines; g++ {
		wgObjs[g] = make([]*MyStruct, 0, perG)
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				obj := &MyStruct{A: int64(i)}
				wgObjs[id] = append(wgObjs[id], obj)
			}
		}(g)
	}
	wg.Wait()
	elapsed = time.Since(start)
	total := 0
	for g := 0; g < goroutines; g++ {
		total += len(wgObjs[g]) * allocSize
	}
	fmt.Printf("[Heap] Time: %v, Total memory used: %.2f GB, Per Object: %.3f ns\n\n",
		elapsed, float64(total)/1024/1024/1024, float64(elapsed.Nanoseconds())/float64(totalObjects),
	)
}
