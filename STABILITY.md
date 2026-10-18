# RustyGo Stability Contract & Compatibility Promise

**Current Milestone**: `v1.0.0` (Production General Availability)  
**SemVer Guarantee**: Strict Semantic Versioning 2.0.0

---

## 1. SemVer Commitment

RustyGo adheres strictly to [Semantic Versioning 2.0.0](https://semver.org/).

- **Major Releases (`vX.0.0`)**: May introduce backward-incompatible API changes with minimum 6 months deprecation warnings.
- **Minor Releases (`v1.X.0`)**: Introduce new features, optimizations, allocators, and codecs in a 100% backward-compatible manner.
- **Patch Releases (`v1.0.X`)**: Bug fixes, performance improvements, and documentation updates.

No existing public API in `rustygo`, `rustygo/codec`, or `rustygo/analyzer` will be modified in a breaking manner within the `v1.x` series.

---

## 2. Go Version Support Policy

RustyGo supports the current and previous major versions of the Go programming language:

| Go Version | Compatibility Status | Test Suite Verification |
| :--- | :--- | :--- |
| **Go 1.25** | ✅ Fully Supported (Primary) | Multi-OS CI (Ubuntu, macOS, Windows) |
| **Go 1.24** | ✅ Fully Supported | Multi-OS CI (Ubuntu, macOS, Windows) |
| **Go 1.23** | ✅ Fully Supported | Source Compilation & Runtime Tested |
| **WebAssembly (`wasip1` / `js`)** | ✅ Fully Supported | Linear memory edge allocators |

---

## 3. Core Invariant Guarantees

Every release in the `v1.x` lineage guarantees the following runtime invariants:

1. **Unknown Defaults to Heap Fallback**: If Static Single Assignment (SSA) dataflow cannot mathematically prove an allocation's lifetime, it is routed to standard Go GC heap. Safety is never sacrificed for performance.
2. **Zero-Copy Invariant**: Functions labeled as zero-copy (`AllocSlice`, `AllocValue`, `JSONScanner`, `NewMsgPackDecoder`, `NewProtoDecoder`, `ReadResponseBody`) perform zero runtime heap allocations when executed in an active Scope.
3. **Hardware Isolation**: Hardware guard pages use OS kernel memory flags (`PAGE_NOACCESS` on Windows, `PROT_NONE` on Unix) to convert memory overruns into deterministic SIGSEGV traps.
4. **Data Race Freedom**: Multi-chunk allocators, sessions, and `ShardedArenaPool` instances pass the Go runtime race detector (`-race`) under continuous high concurrency.
