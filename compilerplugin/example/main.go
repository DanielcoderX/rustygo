package main

import (
	"fmt"
	"runtime"
	"time"

	rg "rustygo"
)

var _ = rg.NewArena

// BigStruct is large enough to exceed Go's stack allocation limits natively
// when doing lots of processing, causing heap allocations.
type BigStruct struct {
	Data [128 * 1024]byte // 128 KB
}

// processItem represents a typical request handler or packet processor.
// The rustygo compiler plugin will wrap this function body in an Arena.
//go:noinline
func processItem(i int) byte {
	// These allocations are eligible for arena rewrite because they don't
	// escape this function's scope.
	val := new(BigStruct)
	buf := make([]byte, 128*1024)
	
	val.Data[0] = byte(i)
	buf[0] = byte(i)
	
	return val.Data[0] + buf[0]
}

func main() {
	var m1, m2 runtime.MemStats
	
	runtime.GC()
	runtime.ReadMemStats(&m1)

	start := time.Now()
	
	var sum byte
	// Simulate processing a batch of requests
	for i := 0; i < 100000; i++ {
		sum += processItem(i)
	}
	
	elapsed := time.Since(start)

	runtime.GC()
	runtime.ReadMemStats(&m2)

	totalAlloc := m2.TotalAlloc - m1.TotalAlloc

	fmt.Println("=== Memory Usage Report ===")
	fmt.Printf("Result       : %d\n", sum)
	fmt.Printf("Time Elapsed : %v\n", elapsed)
	fmt.Printf("Total Alloc  : %d MB\n", totalAlloc/1024/1024)
	fmt.Println("===========================")
}
