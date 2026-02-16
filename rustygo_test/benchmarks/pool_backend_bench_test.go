package rustygo_test

import (
	rg "rustygo"
	"testing"
)

type benchPoolObj struct {
	A int
	B [64]byte
}

func newBenchPool(backend rg.PoolBackend) *rg.Pool[benchPoolObj] {
	return rg.NewPool(
		func() *benchPoolObj { return new(benchPoolObj) },
		rg.WithPoolBackend[benchPoolObj](backend),
		rg.WithResetFn[benchPoolObj](func(v *benchPoolObj) {
			*v = benchPoolObj{}
		}),
	)
}

func benchmarkPoolBackendParallel(b *testing.B, backend rg.PoolBackend) {
	pool := newBenchPool(backend)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			obj := pool.Alloc()
			obj.A++
			pool.Free(obj)
		}
	})
}

func BenchmarkPoolTreiberParallel(b *testing.B) {
	benchmarkPoolBackendParallel(b, rg.PoolBackendTreiber)
}

func BenchmarkPoolSyncParallel(b *testing.B) {
	benchmarkPoolBackendParallel(b, rg.PoolBackendSync)
}
