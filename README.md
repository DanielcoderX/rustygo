# rustygo 🚀

**Zero-cost memory primitives and true compiler integration for Go.** 

`rustygo` brings determinism and massive memory footprint reductions to Go, completely abstracting away the garbage collector. Whether through direct APIs or our **transparent compiler wrapper (`rustygoc`)**, you can drop memory usage by orders of magnitude without changing how you write Go.

---

## 🌟 The Star Feature: `rustygoc` Compiler Plugin

The `rustygoc` compiler wrapper is a zero-configuration `-toolexec` wrapper that automatically injects Arena allocations directly into your Go AST during compilation. 

### How it Works
1. **AST Interception:** `rustygoc` intercepts `go tool compile` and inspects your module's AST.
2. **Escape Analysis:** It runs a strict, conservative escape analysis. Any object that escapes its function scope, loop body, or gets captured by a goroutine or closure is safely ignored.
3. **Transparent Rewriting:** For safe, short-lived allocations (e.g., `new()`, `make()`, or struct literals), it injects a transparent `rustygo` Arena block. The memory is instantly released to the OS (`VirtualFree`/`mmap`) as soon as the function returns.
4. **Seamless Integration:** It safely ignores the standard library, relying heavily on Go's standard build caching to ensure lightning-fast builds.

### Real-World Impact
In our `compilerplugin/example` benchmark, an application allocating 250KB per request across 100,000 iterations typically triggers massive heap growth due to GC latency. 
With `rustygoc`, peak memory footprint drops from **25,000 MB (25 GB)** down to **11 MB** with zero code changes, while avoiding all OOM crashes.

### How to Use `rustygoc`

```bash
# 1. Install the wrapper
go install ./compilerplugin/cmd/rustygoc

# 2. Build your project seamlessly
rustygoc build -o optimized_app.exe ./...
```

You can optionally configure the injected Arena size via environment variables:
`RUSTYGO_ARENA_BYTES=2097152 rustygoc build .`

---

## 🧠 Generic `SyncPool[T]`

A zero-alloc generic wrapper for `sync.Pool` that works beautifully with structs:
```go
pool := rg.NewSyncPoolWrapper(func() *MyStruct {
	return new(MyStruct)
})

obj := pool.Get()
defer pool.Put(obj)
```

---

## ⚡ Direct API (Manual Usage)

If you prefer not to use the compiler plugin, you can manually use the high-level default session API.

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

For deterministic, zero-ceremony arena allocation:

```go
r := rg.NewRegion(64 * 1024)
defer r.Done()

node := rg.New[MyNode](r)
buf := rg.Slice[byte](r, 4096)
```

---

## 🔬 Advanced Usage (Optional)

Use advanced APIs only when you need deterministic control.

- `Region`: one-line arena+scope lifecycle for the common single-lifetime case.
- `Arena`: explicit bump allocation, `Mark/Rewind`, aligned allocation.
- Typed scope helpers: `AllocValue[T](scope)`, `AllocSlice[T](scope, n)`, `AllocSliceCap[T](scope, len, cap)`.
- `Pool`: backend tuning (`Treiber` vs `sync.Pool`), reset/poison/zero options.
- GC lifecycle helpers: `WithGCDisabled`, `WithGCPercent`.

---

## 🛡️ Safety Rules

- Never use arena slices after `Arena.Reset()` or `Arena.Rewind(...)` that rewinds before their allocation.
- Never double-free pooled objects.
- Treat pooled objects as reusable scratch objects; always fully initialize before use.
- Prefer callback lifecycles (`WithBorrow`, `WithScope`, `Pool.WithBorrow`) to avoid cleanup leaks.

---

## 📊 Observability Guidance

### Why memory may not "drop" immediately

This library focuses on reducing allocations and reusing memory. In Go, reused memory is often retained by the runtime and may not immediately reduce RSS/process memory.

Arena backing is OS-managed on supported targets:
- `linux`, `darwin`, `freebsd`: `mmap`
- `windows`: `VirtualAlloc`
- `js/wasm`, `wasip1`: heap-backed fallback

### Interpreting improvements
- **Good sign:** lower `allocs/op`, lower `B/op`, lower GC frequency.
- **Not required:** immediate drop in process RSS.
- **Expected:** steady-state memory plateau with stable reuse.

---

## 📉 Measured Benchmark Results

On `BenchmarkRequestBatchArenaVsHeap`, the arena path hit the headline result:
**`0 B/op` and `0 allocs/op`.**

Measured output on this machine:

```text
BenchmarkRequestBatchArenaVsHeap/Heap-16         	  804920	      1601 ns/op	    1776 B/op	      24 allocs/op
BenchmarkRequestBatchArenaVsHeap/Arena-16        	 1000000	      1030 ns/op	       0 B/op	       0 allocs/op
```

---

## 🛠️ API Stability and Versioning

- Current module API version: `v0.1.0` (`rustygo.Version`)
- Stability contract:
  - `v0.x`: API may evolve between minor versions.
  - `v1.x+`: backward-compatible API by default.

## Test Layout

See `rustygo_test/TEST_CLASSIFICATION.md` for categorized tests and benchmarks.
