package escape

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"

	"rustygo/internal/analysis/allocation"
	"rustygo/internal/analysis/lifetime"
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

func TestEscapeClassifier(t *testing.T) {
	tests := []struct {
		name             string
		source           string
		expectedDecision Decision
		expectedReason   EscapeReason
	}{
		{
			name: "Safe Allocation",
			source: `package main
func f() {
    x := new(int)
    _ = *x
}`,
			expectedDecision: Arena,
			expectedReason:   None,
		},
		{
			name: "Returned Pointer",
			source: `package main
func f() *int {
    x := new(int)
    return x
}`,
			expectedDecision: Heap,
			expectedReason:   EscapesReturn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := buildSSA(tt.source)
			if err != nil {
				t.Fatalf("failed to build SSA: %v", err)
			}

			var targetFn *ssa.Function
			for _, member := range pkg.Members {
				if fn, ok := member.(*ssa.Function); ok && fn.Name() == "f" {
					targetFn = fn
				}
			}

			allocation.AnalyzeFunction(targetFn)
			allocRes := allocation.GetAllocation(1)
			if allocRes == nil {
				// Try finding by scanning allocations
				t.Fatal("no allocation discovered")
			}

			// Run lifetime analysis
			lifetime.AnalyzeFunction(targetFn)
			reports, err := lifetime.Analyze(targetFn.Prog)
			if err != nil {
				t.Fatalf("lifetime analysis failed: %v", err)
			}

			allocationsResult, _ := allocation.Analyze(targetFn.Prog)

			decisions := Classify(allocationsResult, reports)

			found := false
			for _, dec := range decisions {
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
				t.Fatal("decision not found")
			}
		})
	}
}
func TestEscapesMap(t *testing.T) {
	fmt.Println("TestEscapesMap stub")
}
