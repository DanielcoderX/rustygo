package compilerplugin_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rustygo/compilerplugin"
)

func TestRewriteModule(t *testing.T) {
	result, err := compilerplugin.RewriteModule(compilerplugin.Config{
		WorkDir:    repoRoot(t),
		ArenaBytes: 1234,
	}, []string{"./internal/compilerplugintest/basic"})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.RewrittenFiles) == 0 {
		t.Fatal("expected rewritten files")
	}

	data, err := os.ReadFile(result.RewrittenFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`rustygoArena := rg.NewArena(1234)`,
		`n := rg.AllocValue[Node](rustygoScope)`,
		`buf := rg.AllocSlice[byte](rustygoScope, 16)`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten file missing %q\n%s", want, got)
		}
	}
}

func TestBuild(t *testing.T) {
	var stdout bytes.Buffer
	result, err := compilerplugin.Build(compilerplugin.Config{
		WorkDir:    repoRoot(t),
		ArenaBytes: 2048,
		Stdout:     &stdout,
		Stderr:     &stdout,
	}, []string{"./internal/compilerplugintest/basic"}, nil)
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, stdout.String())
	}
	if result == nil {
		t.Fatal("expected build result")
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
