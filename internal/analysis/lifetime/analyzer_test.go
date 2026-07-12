package lifetime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func buildSSA(src string) (*ssa.Package, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	conf := types.Config{}

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

			AnalyzePackage(pkg)

			found := false
			for _, report := range lastResult.Reports {
				if strings.Contains(report.ObjectID, "f") {
					found = true
					if report.LifetimeState != tt.expectedState {
						t.Errorf("expected state %s, got %s (reason: %s)", tt.expectedState, report.LifetimeState, report.EscapeReason)
					}
				}
			}

			if !found && len(lastResult.Reports) > 0 {
				// Fallback to asserting first report
				for _, report := range lastResult.Reports {
					if report.LifetimeState != tt.expectedState {
						t.Errorf("expected state %s, got %s", tt.expectedState, report.LifetimeState)
					}
					break
				}
			}
		})
	}
}
