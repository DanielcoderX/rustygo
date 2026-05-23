# rustygo/analyzer

Static analysis pass that scans Go code and reports allocations eligible for
rustygo arena management.

## What it detects

| Pattern | Eligible if |
|---|---|
| `new(T)` | result does not escape the function |
| `make([]T, n)` | slice does not escape the function |
| `T{...}` composite literal | struct does not escape the function |

## Install

```bash
go install rustygo/analyzer/cmd/rustygocheck@latest
```

## Run

```bash
# scan a package
rustygocheck ./...

# scan a specific package
rustygocheck ./pkg/network/...

# rewrite eligible sites in place
rustygocheck -fix ./...

# rewrite with a larger inserted arena size
rustygocheck -fix -arena-bytes=131072 ./...
```

## Output example

```
./server.go:42:12: [rustygo] new(Packet) is arena-eligible
./server.go:87:8:  [rustygo] make([]byte) is arena-eligible

[rustygo] arena eligibility: 34/51 allocations (66.7%)
```

## How to use the result

For each flagged site, replace the allocation with the rustygo equivalent:

```go
// before
buf := make([]byte, 4096)

// after
arena := rustygo.NewArena(64 * 1024)
scope := arena.EnterScope()
defer scope.Exit()
buf := rustygo.AllocSlice[byte](scope, 4096)
```

## Integration with go vet

```bash
go vet -vettool=$(which rustygocheck) ./...
```

## Auto-fix coverage

Current rewrite support:

- `new(T)` -> `rustygo.AllocValue[T](scope)`
- `make([]T, n)` -> `rustygo.AllocSlice[T](scope, n)`
- `make([]T, n, cap)` -> `rustygo.AllocSliceCap[T](scope, n, cap)`

Composite literals are reported but not auto-rewritten yet.
The fixer uses `65536` bytes by default and can be tuned with `-arena-bytes`.

## Roadmap

- [x] Auto-rewrite mode (`-fix` flag) for eligible `new` and `make([]T, ...)` sites
- [ ] Lifetime inference across call boundaries
- [ ] Java port (JSR-292 / Project Valhalla arena hints)
- [ ] Runtime integration - redirect `mallocgc` calls for eligible types
