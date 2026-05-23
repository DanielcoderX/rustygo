# rustygo/compilerplugin

Prototype compiler pass for eventual Go toolchain integration.

Current model:

- load package syntax with `go/packages`
- rewrite eligible `new` and `make([]T, ...)` sites using the analyzer logic
- copy the current module to a temporary work tree
- write rewritten files into that temp tree
- invoke `go build` from the temp module root

Entry point:

```bash
go run ./compilerplugin/cmd/rustygoc build ./...
```

Useful flags:

- `-arena-bytes=N` controls the inserted arena size
- `-work` keeps and prints the temporary rewritten module directory

Why this exists:

- it exercises the rewrite path in a compiler-like workflow
- it gives you a concrete integration surface before modifying `cmd/compile`
- it keeps the current repo independent from a forked Go toolchain
