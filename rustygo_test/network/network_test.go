package rustygo_test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	rg "rustygo"
)

// ---------------------------------------------------
// Simulated network packet
// ---------------------------------------------------
type Packet struct {
	ID      int
	Payload [512]byte
}

// ---------------------------------------------------
// Benchmark: simulate high-throughput packet processing
// ---------------------------------------------------
func BenchmarkNetworkPacketProcessing(b *testing.B) {
	const numWorkers = 32
	const packetsPerWorker = 100_000

	pool := rg.NewPool[Packet](func() *Packet { return new(Packet) })
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	start := time.Now()

	for w := 0; w < numWorkers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < packetsPerWorker; i++ {
				// allocate a packet from Pool
				pkt := pool.Alloc()

				// simulate filling packet with random data
				for j := range pkt.Payload {
					pkt.Payload[j] = byte(rand.Intn(256))
				}

				// simulate processing (dummy)
				_ = pkt.Payload[0] + pkt.Payload[511]

				// return packet to pool
				pool.Free(pkt)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("[Network] Processed %d packets in %s\n",
		numWorkers*packetsPerWorker, elapsed)

	// Print pool stats
	stats := pool.Stats()
	fmt.Printf("[Network] Pool stats - Total: %.2f MB, Peak: %.2f MB, InUse: %d\n",
		float64(stats.TotalBytes)/1024/1024, float64(stats.PeakBytes)/1024/1024, stats.InUse)
}

// ---------------------------------------------------
// Benchmark: Arena with high-concurrency allocation
// ---------------------------------------------------
func BenchmarkNetworkArenaAlloc(b *testing.B) {
	const numWorkers = 32
	const packetsPerWorker = 100_000
	// Need: 32 * 100,000 * 512 bytes = ~1.6 GB, use 2 GB to be safe
	arena := rg.NewArena(2 * 1024 * 1024 * 1024) // 2GB arena
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	start := time.Now()
	for w := 0; w < numWorkers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < packetsPerWorker; i++ {
				buf := arena.Alloc(512)
				// simulate filling packet
				for j := range buf {
					buf[j] = byte(rand.Intn(256))
				}
				// dummy processing
				_ = buf[0] + buf[511]
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("[Arena] Allocated %d packets in %s\n",
		numWorkers*packetsPerWorker, elapsed)

	used, capacity := arena.Stats()
	fmt.Printf("[Arena] Used: %.2f MB, Capacity: %.2f MB\n",
		float64(used)/1024/1024, float64(capacity)/1024/1024)
}
