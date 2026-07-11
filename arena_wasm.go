//go:build !windows && !linux && !darwin && !freebsd

package rustygo

import "sync"

var wasmBufferPools sync.Map

func getWasmBufferPool(size int) *sync.Pool {
	if p, ok := wasmBufferPools.Load(size); ok {
		return p.(*sync.Pool)
	}
	p := &sync.Pool{
		New: func() any {
			return make([]byte, size)
		},
	}
	actual, _ := wasmBufferPools.LoadOrStore(size, p)
	return actual.(*sync.Pool)
}

func allocArenaBuffer(size int) ([]byte, func([]byte) error, error) {
	pool := getWasmBufferPool(size)
	buf := pool.Get().([]byte)
	clear(buf)
	return buf, func(b []byte) error {
		pool.Put(b)
		return nil
	}, nil
}
