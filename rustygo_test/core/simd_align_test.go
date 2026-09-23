package rustygo_test

import (
	rg "rustygo"
	"testing"
	"unsafe"
)

type CacheBlock struct {
	Header uint64
	Data   [56]byte
}

func TestCacheLineAndSIMDAlignment(t *testing.T) {
	arena := rg.NewArena(1024)
	defer arena.Close()

	scope := arena.EnterScope()
	defer scope.Exit()

	// 1. Single value cache-aligned
	blk := rg.AllocCacheAligned[CacheBlock](scope)
	addr := uintptr(unsafe.Pointer(blk))
	if addr%rg.CacheLineSize != 0 {
		t.Fatalf("expected address %d to be aligned to %d bytes", addr, rg.CacheLineSize)
	}

	// 2. Slice cache-aligned
	slice := rg.AllocSliceCacheAligned[byte](scope, 128, 128)
	sliceAddr := uintptr(unsafe.Pointer(&slice[0]))
	if sliceAddr%rg.CacheLineSize != 0 {
		t.Fatalf("expected slice address %d to be aligned to %d bytes", sliceAddr, rg.CacheLineSize)
	}

	// 3. AVX2 SIMD aligned (32 bytes)
	simdSlice := rg.AllocSliceAligned[float32](scope, 16, 16, rg.SIMDAVX2Size)
	simdAddr := uintptr(unsafe.Pointer(&simdSlice[0]))
	if simdAddr%rg.SIMDAVX2Size != 0 {
		t.Fatalf("expected AVX2 slice address %d to be aligned to %d bytes", simdAddr, rg.SIMDAVX2Size)
	}
}

func TestLocalArenaCacheAligned(t *testing.T) {
	la := rg.NewLocalArena(1024)
	defer la.Close()

	_ = la.Alloc(7) // unalign
	buf := la.AllocCacheAligned(64)
	addr := uintptr(unsafe.Pointer(&buf[0]))
	if addr%rg.CacheLineSize != 0 {
		t.Fatalf("expected LocalArena cache-aligned address %d to be aligned to %d", addr, rg.CacheLineSize)
	}
}

func TestArenaWithGuardPages(t *testing.T) {
	arena := rg.NewArena(4096, rg.WithGuardPages(true))
	defer arena.Close()

	buf := arena.Alloc(4096)
	if len(buf) != 4096 {
		t.Fatalf("expected 4096 bytes allocated, got %d", len(buf))
	}
	// Buffer usable
	buf[0] = 0xAA
	buf[4095] = 0xBB

	// Clean reset & close
	arena.Reset()
	if err := arena.Close(); err != nil {
		t.Fatalf("arena close failed: %v", err)
	}
}
