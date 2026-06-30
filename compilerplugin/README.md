# rustygo/compilerplugin

Prototype compiler pass for Go toolchain integration.

Current models:

1. **`-toolexec` wrapper (Recommended)**
   Intercepts `go build` natively.
   ```bash
   go build -toolexec="go run ./compilerplugin/cmd/rustygoc" ./...
   ```

2. **Standalone tree rewriter**
   - load package syntax with `go/packages`
   - rewrite eligible `new` and `make([]T, ...)` sites using the analyzer logic
   - copy the current module to a temporary work tree
   - write rewritten files into that temp tree
   - invoke `go build` from the temp module root

   Entry point:
   ```bash
   go run ./compilerplugin/cmd/rustygoc build ./...
   ```

Useful flags for standalone build:

- `-arena-bytes=N` controls the inserted arena size
- `-work` keeps and prints the temporary rewritten module directory
