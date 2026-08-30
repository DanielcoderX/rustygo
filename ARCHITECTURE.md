# RustyGo System Architecture & Analysis Consolidation

This document outlines the architectural design and analysis flow of the RustyGo memory optimization framework.

---

## Single Source of Truth for Escape Analysis

As of v0.4.0+, RustyGo consolidates all static analysis onto the unified SSA pipeline under `internal/analysis/pipeline`.

### Pipeline Structure

```
Source Code ──> ssa.Program ──> Function Summaries ──> Allocation Discovery ──> Lifetime Checker ──> Escape Classification ──> AST Rewriter
                                (summary package)       (allocation package)     (lifetime package)    (escape package)        (rewrite package)
```

1. **`internal/analysis/summary`**: Computes inter-procedural function parameter escape and return flow summaries via fixpoint iteration across package call graphs.
2. **`internal/analysis/allocation`**: Identifies all memory allocation candidate instructions (`new`, `make`, literals) with stable IDs.
3. **`internal/analysis/lifetime`**: Constructs a directed alias and ownership flow graph over hierarchical lexical regions (`Function -> Block -> Loop -> Scope`), detecting escape violations.
4. **`internal/analysis/escape`**: Maps lifetime states (`SAFE`, `UNSAFE`, `UNKNOWN`) into optimization decisions (`Arena`, `Heap`, `Unknown`). Enforces that `UNKNOWN` defaults to `UNSAFE` (fallback to Go heap) and gates arena allocation on `!HasPointers(T)`.
5. **`internal/analysis/pipeline`**: Serves as the single source of truth for both compiler build plugins (`rustygoc`, `-toolexec`), standalone linters (`rustygo-vet`), and AST rewriters (`analyzer/fix.go`).

### Consolidation Rationale

Prior versions maintained duplicate dataflow escape analysis logic between `analyzer/analyzer.go` (`filterEligibleSites`) and `internal/analysis/lifetime`. Consolidating onto `internal/analysis/pipeline` ensures:
- **Zero Divergence:** A single, authoritative dataflow implementation prevents mismatched escape classifications.
- **Safety Fallback Uniformity:** Unhandled call summaries, external boundaries, and pointer-containing types strictly fall back to standard Go heap allocation across all tools.
