package lifetime

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"

	"rustygo/internal/analysis/summary"
)

func buildSSA(src string) (*ssa.Package, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	conf := types.Config{
		Importer: importer.Default(),
	}

	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}

	pkg, err := conf.Check("main", fset, []*ast.File{file}, info)
	if err != nil {
		return nil, err
	}

	prog := ssa.NewProgram(fset, 0)
	var registerImports func(*types.Package, map[*types.Package]bool)
	registerImports = func(p *types.Package, visited map[*types.Package]bool) {
		if visited[p] {
			return
		}
		visited[p] = true
		for _, imp := range p.Imports() {
			prog.CreatePackage(imp, nil, nil, true)
			registerImports(imp, visited)
		}
	}
	registerImports(pkg, make(map[*types.Package]bool))

	ssaPkg := prog.CreatePackage(pkg, []*ast.File{file}, info, true)
	ssaPkg.Build()

	return ssaPkg, nil
}

func TestLifetimeChecker(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		expectedState LifetimeState
	}{
		{
			name: "Simple stack allocation",
			source: `package main
func f() {
    x := new(int)
    _ = *x
}`,
			expectedState: Safe,
		},
		{
			name: "Returned pointer allocation",
			source: `package main
func f() *int {
    x := new(int)
    return x
}`,
			expectedState: Unsafe,
		},
		{
			name: "Goroutine capture",
			source: `package main
func f() {
    x := new(int)
    go func() {
        _ = x
    }()
}`,
			expectedState: Unsafe,
		},
		{
			name: "Channel send",
			source: `package main
func f(ch chan *int) {
    x := new(int)
    ch <- x
}`,
			expectedState: Unsafe,
		},
		{
			name: "Global variable store",
			source: `package main
var global *int
func f() {
    x := new(int)
    global = x
}`,
			expectedState: Unsafe,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := buildSSA(tt.source)
			if err != nil {
				t.Fatalf("failed to build SSA: %v", err)
			}

			res := AnalyzePackage(pkg)

			found := false
			for _, report := range res.Reports {
				if strings.Contains(report.ObjectID, "f") {
					found = true
					if report.LifetimeState != tt.expectedState {
						t.Errorf("expected state %s, got %s (reason: %s)", tt.expectedState, report.LifetimeState, report.EscapeReason)
					}
				}
			}

			if !found && len(res.Reports) > 0 {
				// Fallback to asserting first report
				for _, report := range res.Reports {
					if report.LifetimeState != tt.expectedState {
						t.Errorf("expected state %s, got %s", tt.expectedState, report.LifetimeState)
					}
					break
				}
			}
		})
	}
}

func TestCallSummaryHandling(t *testing.T) {
	src := `package main
import "fmt"

func helper(p *int) {
	_ = *p
}

func fSafe() {
	x := new(int)
	helper(x)
}

func fExternal() {
	x := new(int)
	fmt.Println(x)
}
`
	pkg, err := buildSSA(src)
	if err != nil {
		t.Fatalf("failed to build SSA: %v", err)
	}

	summary.AnalyzeSummaries(pkg.Prog)
	res, err := Analyze(pkg.Prog)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	foundSafe := false
	foundExternal := false

	for _, r := range res.Reports {
		if strings.Contains(r.ObjectID, "fExternal") {
			foundExternal = true
			if r.LifetimeState == Safe {
				t.Errorf("external call with missing summary should NOT be Safe, got %s", r.LifetimeState)
			}
		}
		if strings.Contains(r.ObjectID, "fSafe") {
			foundSafe = true
			if r.LifetimeState != Safe {
				t.Errorf("helper call with non-escaping summary should be Safe, got %s (%s)", r.LifetimeState, r.EscapeReason)
			}
		}
	}

	if !foundSafe || !foundExternal {
		t.Errorf("failed to locate report targets: safe=%v external=%v", foundSafe, foundExternal)
	}
}
