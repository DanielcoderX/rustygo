package rustygo_test

import (
	rg "rustygo"
	"sync"
	"testing"
)

type concurrentObj struct {
	A int
	B [32]byte
}

func TestSessionConcurrentBorrowRelease(t *testing.T) {
	s := rg.NewSession()

	const workers = 16
	const iters = 5000

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				buf := s.Borrow(256)
				buf[0] = byte(worker)
				buf[255] = byte(i)
				s.Release(buf)
			}
		}(w)
	}
	wg.Wait()

	stats := s.Stats()
	if stats.Hits+stats.Misses == 0 {
		t.Fatal("expected non-zero session activity stats")
	}
}

func TestPoolConcurrentBorrowRelease(t *testing.T) {
	p := rg.NewPool(
		func() *concurrentObj { return new(concurrentObj) },
		rg.WithPoolBackend[concurrentObj](rg.PoolBackendSync),
		rg.WithResetFn[concurrentObj](func(v *concurrentObj) { *v = concurrentObj{} }),
	)

	const workers = 16
	const iters = 5000

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				obj := p.Borrow()
				obj.A = worker + i
				p.Release(obj)
			}
		}(w)
	}
	wg.Wait()

	stats := p.Stats()
	if stats.InUse != 0 {
		t.Fatalf("expected InUse=0 after concurrent release, got %d", stats.InUse)
	}
}
