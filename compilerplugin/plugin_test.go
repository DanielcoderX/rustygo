package compilerplugin_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuild(t *testing.T) {
	root := repoRoot(t)
	rustygoc := filepath.Join(root, "compilerplugin", "cmd", "rustygoc")

	// We must first go install rustygoc or run it with go run. Let's use go run.
	cmd := exec.Command("go", "run", rustygoc, "build", "./internal/compilerplugintest/basic")
	cmd.Dir = root
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	cmd.Env = append(os.Environ(), "RUSTYGO_ARENA_BYTES=2048")

	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, stdout.String())
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
