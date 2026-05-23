# rustygo - **WIP**

Low-level memory primitives for Go, with a zero-config default path.

## API Stability and Versioning

- Current module API version: `v0.1.0` (`rustygo.Version`)
- Stability contract:
  - `v0.x`: API may evolve between minor versions.
  - `v1.x+`: backward-compatible API by default.

Release tagging command:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Quick Start (Import and Run)

Use the high-level default session API if you want memory reuse without tuning.

```go
package main

import (
	"fmt"
	rg "rustygo"
)

func main() {
	err := rg.WithBorrow(1024, func(buf []byte) error {
		copy(buf, []byte("hello"))
		fmt.Println(string(buf[:5]))
		return nil
	})
	if err != nil {
		panic(err)
	}
}
```

You can also use explicit lifecycle calls:

```go
buf := rg.Borrow(4096)
defer rg.Release(buf)
```

For deterministic, zero-ceremony arena allocation:

```go
r := rg.NewRegion(64 * 1024)
defer r.Done()

node := rg.New[MyNode](r)
buf := rg.Slice[byte](r, 4096)
```

## Advanced Usage (Optional)

Use advanced APIs only when you need deterministic control.

- `Region`: one-line arena+scope lifecycle for the common single-lifetime case.
- `Arena`: explicit bump allocation, `Mark/Rewind`, aligned allocation.
- Typed scope helpers: `AllocValue[T](scope)`, `AllocSlice[T](scope, n)`, `AllocSliceCap[T](scope, len, cap)`.
- `Pool`: backend tuning (`Treiber` vs `sync.Pool`), reset/poison/zero options.
- GC lifecycle helpers: `WithGCDisabled`, `WithGCPercent`.

## Safety Rules

- Never use arena slices after `Arena.Reset()` or `Arena.Rewind(...)` that rewinds before their allocation.
- Never double-free pooled objects.
- Treat pooled objects as reusable scratch objects; always fully initialize before use.
- Prefer callback lifecycles (`WithBorrow`, `WithScope`, `Pool.WithBorrow`) to avoid cleanup leaks.

## Observability Guidance

### Why memory may not "drop" immediately

This library focuses on reducing allocations and reusing memory. In Go, reused memory is often retained by the runtime and may not immediately reduce RSS/process memory.

Arena backing is OS-managed on supported targets:

- `linux`, `darwin`, `freebsd`: `mmap`
- `windows`: `VirtualAlloc`
- `js/wasm`, `wasip1`: heap-backed fallback

### What to observe

- Allocation pressure:
  - benchmark metrics `B/op`, `allocs/op`
  - `SessionStats` hit/miss rates
  - `Pool.Stats()` (`InUse`, `TotalObjects`, `PeakObjects`)
- Runtime behavior:
  - `runtime.ReadMemStats` (`Mallocs`, `Frees`, `HeapAlloc`, `HeapInuse`)
  - `GODEBUG=gctrace=1` for GC pacing/pressure

### Interpreting improvements

- Good sign: lower `allocs/op`, lower `B/op`, lower GC frequency.
- Not required: immediate drop in process RSS.
- Expected: steady-state memory plateau with stable reuse.

## Test and Benchmark Commands

```bash
go test ./...
go test -race ./...
go test -tags rustygo_debug ./...
go test -run ^$ -bench . -benchmem ./...
```

Detailed benchmark suite:

```bash
go test ./rustygo_test/benchmarks -run ^$ -bench BenchmarkDetailed -benchmem
go test ./rustygo_test/benchmarks -run ^$ -bench BenchmarkRequestBatchArenaVsHeap -benchmem
```

## Measured Results

On `BenchmarkRequestBatchArenaVsHeap`, the arena path hit the headline result:
`0 B/op` and `0 allocs/op`.

Measured output on this machine:

```text
BenchmarkRequestBatchArenaVsHeap/Heap-16         	  804920	      1601 ns/op	    1776 B/op	      24 allocs/op
BenchmarkRequestBatchArenaVsHeap/Arena-16        	 1000000	      1030 ns/op	       0 B/op	       0 allocs/op
```

Command used:

```bash
go test ./rustygo_test/benchmarks -run ^$ -bench BenchmarkRequestBatchArenaVsHeap -benchmem
```

## Compiler Pass Prototype

This repo also includes a prototype compiler-wrapper pass in `compilerplugin/`.
It is not a Go toolchain patch yet; it rewrites eligible allocations into a
temporary module tree and then invokes `go build`.

```bash
go run ./compilerplugin/cmd/rustygoc build ./internal/compilerplugintest/basic
go run ./compilerplugin/cmd/rustygoc build -arena-bytes=131072 ./...
```

## Test Layout

See `rustygo_test/TEST_CLASSIFICATION.md` for categorized tests and benchmarks.
