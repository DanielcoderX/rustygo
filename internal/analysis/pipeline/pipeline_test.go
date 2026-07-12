package pipeline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"

	"rustygo/internal/analysis/escape"
)

func buildSSA(src string) (*ssa.Package, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	conf := types.Config{}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
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

func TestPipeline(t *testing.T) {
	tests := []struct {
		name             string
		source           string
		expectedDecision escape.Decision
		expectedReason   escape.EscapeReason
	}{
		{
			name: "Simple Safe Allocation",
			source: `package main
type User struct { Name string }
func f() {
    x := &User{}
    _ = x.Name
}`,
			expectedDecision: escape.Arena,
			expectedReason:   escape.None,
		},
		{
			name: "Returned Pointer",
			source: `package main
type User struct { Name string }
func f() *User {
    return &User{}
}`,
			expectedDecision: escape.Heap,
			expectedReason:   escape.EscapesReturn,
		},
		{
			name: "Goroutine Capture",
			source: `package main
type User struct { Name string }
func f() {
    x := &User{}
    go func() {
        _ = x
    }()
}`,
			expectedDecision: escape.Heap,
			expectedReason:   escape.EscapesClosure,
		},
		{
			name: "Channel Send",
			source: `package main
type User struct { Name string }
func f(ch chan *User) {
    x := &User{}
    ch <- x
}`,
			expectedDecision: escape.Heap,
			expectedReason:   escape.EscapesChannel,
		},
		{
			name: "Global Store",
			source: `package main
type User struct { Name string }
var global *User
func f() {
    x := &User{}
    global = x
}`,
			expectedDecision: escape.Heap,
			expectedReason:   escape.EscapesGlobal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := buildSSA(tt.source)
			if err != nil {
				t.Fatalf("failed to build SSA: %v", err)
			}

			res, err := Run(pkg.Prog)
			if err != nil {
				t.Fatalf("pipeline run failed: %v", err)
			}

			found := false
			for _, dec := range res.Decisions {
				if dec.Allocation.Function.Name() == "f" {
					found = true
					if dec.Decision != tt.expectedDecision {
						t.Errorf("expected decision %v, got %v", tt.expectedDecision, dec.Decision)
					}
					if dec.Reason != tt.expectedReason {
						t.Errorf("expected reason %v, got %v", tt.expectedReason, dec.Reason)
					}
				}
			}

			if !found {
				// Fallback to checking any decision
				for _, dec := range res.Decisions {
					if strings.Contains(dec.Allocation.Function.Name(), "f") {
						found = true
						if dec.Decision != tt.expectedDecision {
							t.Errorf("expected decision %v, got %v", tt.expectedDecision, dec.Decision)
						}
						if dec.Reason != tt.expectedReason {
							t.Errorf("expected reason %v, got %v", tt.expectedReason, dec.Reason)
						}
					}
				}
			}
		})
	}
}
