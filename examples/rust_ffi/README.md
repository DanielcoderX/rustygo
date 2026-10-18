# RustyGo Rust FFI Example

Demonstrates calling RustyGo's arena memory engine directly from Rust via the C-ABI export.

## Prerequisites

- Go 1.23+
- Rust 1.70+ with `cargo`

## How to Run

### Step 1: Build the C Shared Library
From this directory:

**Windows (PowerShell):**
```powershell
go build -buildmode=c-shared -o rustygo.dll ../../cmd/librustygo
```

**Linux / macOS:**
```bash
go build -buildmode=c-shared -o librustygo.so ../../cmd/librustygo
```

### Step 2: Run with Cargo
```powershell
cargo run
```

### Expected Output
```text
🦀 Initializing 64KB RustyGo Arena from Rust...
🚀 Read back from Arena: Zero-copy from Rust to RustyGo!
📊 Used bytes: 128
🔄 Arena reset. New used bytes: 0
```
