# Compiler Plugin Example

This directory contains an example program demonstrating the power of the `rustygo/compilerplugin`.
The program simulates a heavy workload that processes 100,000 requests, allocating large buffers (`128 KB` slices and `4 KB` structs) for each request.

## Running Natively

If you run the program natively, the Go compiler allocates the large buffers on the heap (exceeding stack limits), creating immense heap pressure:

```bash
go run .
```

You should see output similar to:
```
=== Memory Usage Report ===
Result       : 96
Time Elapsed : ~2.4s
Total Alloc  : 25000 MB
===========================
```

## Running with RustyGo Compiler Plugin

When built with the `rustygo` compiler plugin wrapper, the AST is intercepted and rewritten. The `new(BigStruct)` and `make([]byte, ...)` calls are transparently redirected to `rustygo` arenas that are created and destroyed for each request handler!

To build and run with the compiler plugin:

```bash
go build -toolexec="go run ../cmd/rustygoc" -o example_rewritten.exe .
./example_rewritten.exe
```

You will see a massive drop in `Total Alloc` as memory is served directly from the OS-backed arenas rather than the Go runtime heap:

```
=== Memory Usage Report ===
Result       : 96
Time Elapsed : ~0.5s
Total Alloc  : 10 MB
===========================
```

To build and run with the compiler plugin:

```bash
go build -toolexec="go run ../cmd/rustygoc" -o example_rewritten.exe .
./example_rewritten.exe
```

*Note: Depending on your exact toolchain configuration and `analyzer` tuning, you will see a massive drop in `Total Alloc` as memory is served directly from the OS-backed arenas rather than the Go runtime heap!*
