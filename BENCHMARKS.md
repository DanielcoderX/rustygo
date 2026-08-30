# RustyGo Microbenchmarks & Measured Performance Results

This document presents empirical benchmark results comparing thread-local arena allocations against Go runtime heap allocations (`new` / `make`) and standard library `sync.Pool`.

---

## Benchmark System Information

- **Date of Run:** August 30, 2026
- **Go Version:** `go1.25.2 windows/amd64`
- **CPU:** AMD Ryzen 7 7435HS (16 execution threads)
- **OS/Arch:** `windows/amd64`

> [!NOTE]
> **Synthetic Workload Disclaimer**: The measurements below are synthetic microbenchmarks evaluating raw allocation and deallocation latency under isolated loop conditions. They measure local memory management overhead, not overall end-to-end application throughput.

---

## Measured Benchmark Results

Command executed:
```bash
go test -bench=. -benchmem ./rustygo_test/benchmarks/...
```

### 1. Allocation & Reset Overhead (Serial Byte Allocation)

| Allocation Strategy | Latency (`ns/op`) | Memory (`B/op`) | Heap Allocs (`allocs/op`) | Speedup vs Heap |
| :--- | :--- | :--- | :--- | :--- |
| **RustyGo Thread-Local Arena (`Arena.TryAlloc`)** | **9.45 ns** | **0 B** | **0 allocs** | **~5.38x faster** |
| **Standard Heap Allocation (`make([]byte, 256)`)** | **50.80 ns** | **256 B** | **1 allocs** | Baseline |

### 2. Object Lifecycle Benchmarks (Serial Execution)

| Strategy | Latency (`ns/op`) | Memory (`B/op`) | Heap Allocs (`allocs/op`) |
| :--- | :--- | :--- | :--- |
| **Standard Library `sync.Pool`** | **20.56 ns** | **0 B** | **0 allocs** |
| **Standard Go Heap (`new(Struct)`)** | **28.24 ns** | **80 B** | **1 allocs** |
| **Treiber Stack Pool** | **36.90 ns** | **16 B** | **1 allocs** |

### 3. Object Lifecycle Benchmarks (Parallel Execution - 16 Threads)

| Strategy | Latency (`ns/op`) | Memory (`B/op`) | Heap Allocs (`allocs/op`) |
| :--- | :--- | :--- | :--- |
| **Standard Go Heap (`new(Struct)`)** | **28.20 ns** | **80 B** | **1 allocs** |
| **Standard Library `sync.Pool`** | **59.48 ns** | **0 B** | **0 allocs** |
| **Treiber Stack Pool** | **348.80 ns** | **16 B** | **1 allocs** |

---

## Reproducing Benchmarks Locally

To run and verify these benchmarks on your system:

```bash
# Execute standard Go benchmark suite with memory allocation profiling
go test -bench=. -benchmem ./rustygo_test/benchmarks/...

# Execute WebAssembly memory comparison harness
go run ./rustygo_test/wasm_mem.go
```
