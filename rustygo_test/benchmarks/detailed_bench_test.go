package rustygo_test

import (
	rg "rustygo"
	"sync/atomic"
	"testing"
	"unsafe"
)

type lifecycleObj struct {
	ID   int64
	Pad  [64]byte
	Flag uint64
}

var (
	sinkByte byte
	sinkInt  int64
	sinkBuf  []byte
	sinkObj  atomic.Pointer[lifecycleObj]
)

//go:noinline
func consumeBuf(v []byte) {
	sinkBuf = v
}

//go:noinline
func consumeObj(v *lifecycleObj) {
	sinkObj.Store(v)
}

func BenchmarkDetailedArenaVsHeapBytesSerial(b *testing.B) {
	const payload = 256
	b.SetBytes(payload)

	b.Run("Arena_TryAlloc_ResetWhenFull", func(b *testing.B) {
		arena := rg.NewArena(4 * 1024 * 1024)
		scope := arena.EnterScope()
		defer func() {
			if scope.Active() {
				scope.Exit()
			}
		}()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf, ok := scope.TryAlloc(payload)
			if !ok {
				scope.Exit()
				arena.Reset()
				scope = arena.EnterScope()
				buf, _ = scope.TryAlloc(payload)
			}
			buf[0] = byte(i)
			buf[payload-1] = byte(i >> 8)
			sinkByte ^= buf[0]
			consumeBuf(buf)
		}
	})

	b.Run("Heap_make", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := make([]byte, payload)
			buf[0] = byte(i)
			buf[payload-1] = byte(i >> 8)
			sinkByte ^= buf[0]
			consumeBuf(buf)
		}
	})
}

func BenchmarkDetailedObjectLifecycleSerial(b *testing.B) {
	objSize := int64(unsafe.Sizeof(lifecycleObj{}))
	b.SetBytes(objSize)

	newObj := func() *lifecycleObj { return new(lifecycleObj) }

	b.Run("Pool_Treiber", func(b *testing.B) {
		pool := rg.NewPool(
			newObj,
			rg.WithPoolBackend[lifecycleObj](rg.PoolBackendTreiber),
			rg.WithResetFn[lifecycleObj](func(v *lifecycleObj) { *v = lifecycleObj{} }),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obj := pool.Alloc()
			obj.ID = int64(i)
			obj.Flag = uint64(i)
			sinkInt ^= obj.ID
			consumeObj(obj)
			pool.Free(obj)
		}
	})

	b.Run("Pool_Sync", func(b *testing.B) {
		pool := rg.NewPool(
			newObj,
			rg.WithPoolBackend[lifecycleObj](rg.PoolBackendSync),
			rg.WithResetFn[lifecycleObj](func(v *lifecycleObj) { *v = lifecycleObj{} }),
		)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obj := pool.Alloc()
			obj.ID = int64(i)
			obj.Flag = uint64(i)
			sinkInt ^= obj.ID
			consumeObj(obj)
			pool.Free(obj)
		}
	})

	b.Run("Heap_new", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			obj := &lifecycleObj{ID: int64(i), Flag: uint64(i)}
			sinkInt ^= obj.ID
			consumeObj(obj)
		}
	})
}

func BenchmarkDetailedObjectLifecycleParallel(b *testing.B) {
	objSize := int64(unsafe.Sizeof(lifecycleObj{}))
	b.SetBytes(objSize)

	newObj := func() *lifecycleObj { return new(lifecycleObj) }

	b.Run("Pool_Treiber", func(b *testing.B) {
		pool := rg.NewPool(
			newObj,
			rg.WithPoolBackend[lifecycleObj](rg.PoolBackendTreiber),
			rg.WithResetFn[lifecycleObj](func(v *lifecycleObj) { *v = lifecycleObj{} }),
		)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			local := int64(0)
			for pb.Next() {
				obj := pool.Alloc()
				obj.ID = local
				obj.Flag = uint64(local)
				local++
				consumeObj(obj)
				pool.Free(obj)
			}
			sinkInt ^= local
		})
	})

	b.Run("Pool_Sync", func(b *testing.B) {
		pool := rg.NewPool(
			newObj,
			rg.WithPoolBackend[lifecycleObj](rg.PoolBackendSync),
			rg.WithResetFn[lifecycleObj](func(v *lifecycleObj) { *v = lifecycleObj{} }),
		)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			local := int64(0)
			for pb.Next() {
				obj := pool.Alloc()
				obj.ID = local
				obj.Flag = uint64(local)
				local++
				consumeObj(obj)
				pool.Free(obj)
			}
			sinkInt ^= local
		})
	})

	b.Run("Heap_new", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			local := int64(0)
			for pb.Next() {
				obj := &lifecycleObj{ID: local, Flag: uint64(local)}
				local += obj.ID + 1
				consumeObj(obj)
			}
			sinkInt ^= local
		})
	})
}
