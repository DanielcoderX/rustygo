package rustygo_test

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	rg "rustygo"
)

func TestFuzzConcurrentScopeLifecycle(t *testing.T) {
	arena := rg.NewArena(64*1024, rg.WithPoisonOnScopeExit(0xAA))
	defer arena.Close()

	const workers = 30
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(id + int(time.Now().UnixNano()))))

			for i := 0; i < iterations; i++ {
				scope := arena.EnterScope()
				allocCount := rng.Intn(5) + 1

				slices := make([][]byte, allocCount)
				for a := 0; a < allocCount; a++ {
					sz := rng.Intn(128) + 1
					s := rg.AllocSlice[byte](scope, sz)
					s[0] = byte(id)
					slices[a] = s
				}

				scope.Exit()

				// Verify memory poisoning on first allocated slice
				if len(slices[0]) > 0 && slices[0][0] != 0xAA {
					t.Errorf("worker %d: expected poison pattern 0xAA, got 0x%X", id, slices[0][0])
				}
			}
		}(w)
	}

	wg.Wait()
}

func TestFuzzSavepointRollbackUnderLoad(t *testing.T) {
	arena := rg.NewArena(16 * 1024)
	defer arena.Close()

	const cycles = 200
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < cycles; i++ {
		sp := arena.Savepoint()

		allocs := rng.Intn(10) + 1
		for j := 0; j < allocs; j++ {
			_ = arena.Alloc(rng.Intn(256) + 1)
		}

		// Randomly rollback or keep
		if rng.Float32() < 0.5 {
			if err := arena.Rollback(sp); err != nil {
				t.Fatalf("cycle %d: rollback error: %v", i, err)
			}
		}
	}
}
