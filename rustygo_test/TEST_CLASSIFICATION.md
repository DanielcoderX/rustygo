# rustygo_test Classification

## 1. Core correctness and safety
- `rustygo_test/core/arena_api_test.go`
  - Arena safe allocation API (`TryAlloc`, `AllocOrErr`)
  - Scope lifecycle (`WithScope`, `Active`, `UsedBytes`)
  - Mark/rewind behavior
  - Aligned allocation behavior
- `rustygo_test/core/pool_safety_test.go`
  - `WithZeroOnFree`
  - `WithPoisonOnFree`
- `rustygo_test/core/session_test.go`
  - High-level default session API behavior (`Borrow/Release/WithBorrow`)
- `rustygo_test/core/lifecycle_helpers_test.go`
  - Pool callback lifecycle helper
  - GC callback lifecycle helpers
- `rustygo_test/core/concurrency_misuse_test.go`
  - Concurrent `Borrow/Release` misuse-stress checks for Session and Pool
- `rustygo_test/core/pool_debug_test.go` (build tag: `rustygo_debug`)
  - Debug double-free/foreign-pointer detection in `Pool.Free`

## 2. Fuzzing and edge-case robustness
- `rustygo_test/core/arena_fuzz_test.go`
  - Fuzzes alignment inputs and rewind edge cases

## 3. General benchmark comparisons
- `rustygo_test/benchmarks/general_bench_test.go`
  - Concurrent scoped arena allocation
  - Concurrent pool allocation
  - Concurrent heap allocation
- `rustygo_test/benchmarks/pool_backend_bench_test.go`
  - Parallel backend comparison: Treiber vs `sync.Pool`
- `rustygo_test/benchmarks/detailed_bench_test.go`
  - Detailed serial/parallel benchmark suite with `ReportAllocs`, `SetBytes`, and sub-benchmarks

## 4. High-memory stress benchmarks
- `rustygo_test/high_memory/high_memory_test.go`
  - Arena/pool/heap behavior under high memory pressure
  - Parallel high-memory pool benchmark

## 5. Network-oriented benchmarks
- `rustygo_test/network/network_test.go`
  - Packet-processing throughput simulation
  - Arena allocation in high-concurrency packet workloads
- `rustygo_test/network/tcp_network_test.go`
  - TCP benchmark using hybrid allocation strategy

## 6. Standalone stress executables (not `go test`)
- `rustygo_test/real_values/alloc_speed.go`
- `rustygo_test/real_big_values/alloc_speed.go`

These are manual programs (`package main`) for long-running stress/perf runs.

## Recommended commands
- All tests: `go test ./...`
- Race tests: `go test -race ./...`
- Debug safety tests: `go test -tags rustygo_debug ./...`
- Fuzzing (sample): `go test ./rustygo_test/core -run ^$ -fuzz FuzzArena -fuzztime=10s`
- Benchmarks with memory stats: `go test -run ^$ -bench . -benchmem ./...`
