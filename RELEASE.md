# Release Guide

## API Freeze Process

1. Set `Version` in `version.go` to the target release (for example `v0.1.0`).
2. Run verification:
   - `go test ./...`
   - `go test -race ./...`
   - `go test -tags rustygo_debug ./...`
   - `go test -run ^$ -bench . -benchmem ./...`
3. Update docs (`README.md`, `doc.go`, and tests if behavior changed).
4. Create and push the tag:
   - `git tag v0.1.0`
   - `git push origin v0.1.0`

## Versioning Policy

- `v0.x`: API may change between minor versions.
- `v1.x+`: backward compatibility is preserved by default.

