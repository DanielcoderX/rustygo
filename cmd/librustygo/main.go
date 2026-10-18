//go:build cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"

	rg "rustygo"
)

var (
	handleSeq atomic.Uint64
	arenas    sync.Map
)

//export rustygo_arena_new
func rustygo_arena_new(size C.int64_t) C.uintptr_t {
	sz := int(size)
	if sz <= 0 {
		sz = 64 * 1024
	}
	a := rg.NewArena(sz)
	id := handleSeq.Add(1)
	arenas.Store(id, a)
	return C.uintptr_t(id)
}

//export rustygo_arena_alloc
func rustygo_arena_alloc(handle C.uintptr_t, size C.int64_t, align C.int32_t) unsafe.Pointer {
	val, ok := arenas.Load(uint64(handle))
	if !ok {
		return nil
	}
	a := val.(*rg.Arena)
	sz := int(size)
	al := int(align)
	if al <= 1 {
		buf, ok := a.TryAlloc(sz)
		if !ok || len(buf) == 0 {
			return nil
		}
		return unsafe.Pointer(&buf[0])
	}

	buf, ok := a.TryAllocAligned(sz, al)
	if !ok || len(buf) == 0 {
		return nil
	}
	return unsafe.Pointer(&buf[0])
}

//export rustygo_arena_reset
func rustygo_arena_reset(handle C.uintptr_t) {
	val, ok := arenas.Load(uint64(handle))
	if !ok {
		return
	}
	a := val.(*rg.Arena)
	a.Reset()
}

//export rustygo_arena_close
func rustygo_arena_close(handle C.uintptr_t) {
	val, ok := arenas.LoadAndDelete(uint64(handle))
	if !ok {
		return
	}
	a := val.(*rg.Arena)
	_ = a.Close()
}

//export rustygo_arena_used_bytes
func rustygo_arena_used_bytes(handle C.uintptr_t) C.int64_t {
	val, ok := arenas.Load(uint64(handle))
	if !ok {
		return 0
	}
	a := val.(*rg.Arena)
	return C.int64_t(a.UsedBytes())
}

func main() {}
