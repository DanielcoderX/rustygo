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

### 4. Single-Goroutine Bump Allocation (`LocalArena` vs Atomic CAS)

| Strategy | Latency (`ns/op`) | Memory (`B/op`) | Heap Allocs (`allocs/op`) | Speedup |
| :--- | :--- | :--- | :--- | :--- |
| **`rustygo.LocalArena` (Unsync Pointer Bump)** | **3.20 ns** | **0 B** | **0 allocs** | **~1.69x faster (~41% lower latency)** |
| **`rustygo.Arena` (Atomic CAS Fast-Path)** | **5.40 ns** | **0 B** | **0 allocs** | Baseline |

### 5. Zero-Copy JSON Stream Parsing (`codec/json` vs `encoding/json`)

| Parser Strategy | Latency (`ns/op`) | Memory (`B/op`) | Heap Allocs (`allocs/op`) | Speedup |
| :--- | :--- | :--- | :--- | :--- |
| **RustyGo Arena JSON Scanner (`codec.JSONScanner`)** | **239.8 ns** | **100 B** | **2 allocs** | **~4.0x faster** |
| **Standard Library (`encoding/json.Unmarshal`)** | **955.1 ns** | **280 B** | **7 allocs** | Baseline |

---

## Reproducing Benchmarks Locally

To run and verify these benchmarks on your system:

```bash
# Execute standard Go benchmark suite with memory allocation profiling
go test -bench=. -benchmem ./rustygo_test/benchmarks/...

# Execute WebAssembly memory comparison harness
go run ./rustygo_test/wasm_mem.go
```
