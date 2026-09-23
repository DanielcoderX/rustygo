package rustygo_test

import (
	"sync"
	"testing"

	rg "rustygo"
)

func TestShardedArenaPoolConcurrency(t *testing.T) {
	pool := rg.NewShardedArenaPool(32*1024, 16)
	defer pool.Close()

	const workers = 100
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				err := pool.WithScope(func(s *rg.Scope) error {
					slice := rg.AllocSlice[int](s, 64)
					slice[0] = id + i
					val := rg.AllocValue[int](s)
					*val = slice[0]
					if *val != id+i {
						t.Errorf("data mismatch in worker %d", id)
					}
					return nil
				})
				if err != nil {
					t.Errorf("WithScope returned error: %v", err)
				}
			}
		}(w)
	}

	wg.Wait()
}

func TestShardedArenaPoolManualBorrowRelease(t *testing.T) {
	pool := rg.NewShardedArenaPool(16*1024, 8)
	defer pool.Close()

	a1 := pool.Borrow()
	s1 := a1.EnterScope()
	buf := rg.AllocSlice[byte](s1, 1024)
	buf[0] = 0xAA
	s1.Exit()
	pool.Release(a1)

	a2 := pool.Borrow()
	s2 := a2.EnterScope()
	buf2 := rg.AllocSlice[byte](s2, 1024)
	if buf2[0] != 0 {
		t.Fatalf("expected reset memory to be zeroed or clean")
	}
	s2.Exit()
	pool.Release(a2)
}
