package analyzer_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"rustygo/analyzer"

	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer, "basic")
}

func TestRewriteFile(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  analysistest.TestData(),
	}
	pkgs, err := packages.Load(cfg, "./src/basic")
	if err != nil {
		t.Fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages.Load returned errors")
	}

	out, changed, err := analyzer.RewriteFile(pkgs[0].Fset, pkgs[0].Syntax[0], pkgs[0].TypesInfo, pkgs[0].PkgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected rewrite to change file")
	}

	got := string(out)
	for _, want := range []string{
		`import rg "rustygo"`,
		`rustygoArena := rg.NewArena(`,
		`65536`,
		`rustygoScope := rustygoArena.EnterScope(`,
		`rustygoScope.Exit()`,
		`n := rg.AllocValue[Node](rustygoScope)`,
		`s := rg.AllocSlice[int](rustygoScope, 64)`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewrite output missing %q\n%s", want, got)
		}
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "basic.go", got, parser.ParseComments); err != nil {
		t.Fatalf("rewrite output is not valid Go: %v\n%s", err, got)
	}
}
