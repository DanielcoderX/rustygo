// Package rustygo provides explicit, low-level memory management
// primitives for performance-critical Go code, plus a high-level
// session API for zero-config usage.
//
// It is NOT a replacement for Go's GC.
//
// Quick start (high-level):
//   - Borrow(size) / Release(buf) for default session-based buffer reuse
//   - WithBorrow(size, fn) for callback-safe lifecycle handling
//   - NewSession(...) for per-component memory reuse with safe defaults
//
// Use Arena when:
//   - You need very fast bulk allocation
//   - Objects share the same lifetime
//   - You can reset all allocations at once
//
// Use Pool when:
//   - Objects are reused frequently
//   - You want deterministic reuse
//   - You want allocation statistics
//
// Scope semantics:
//   - Scope allocations are committed immediately to the arena offset
//   - EnterScope/Exit define lifecycle only; Exit does not commit memory
//   - Use Scope.UsedBytes() for per-scope diagnostics
//
// Pool backend selection:
//   - PoolBackendTreiber: deterministic LIFO reuse, simple lock-free stack, may allocate node wrappers on Free
//   - PoolBackendSync: lower allocation overhead under concurrency, runtime-managed reuse behavior, best default for high-throughput workloads
//
// Rules:
//   - Never use Arena allocations after Reset()
//   - Never Free the same pool object twice
//   - Objects returned from Pool are NOT zeroed
//   - GC is NEVER modified automatically
//
// This library prioritizes correctness and predictability
// over convenience or magic behavior.
package rustygo
