package summary

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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

func TestFunctionSummaries(t *testing.T) {
	src := `package main
func identity(p *int) *int {
    return p
}

func pure(p *int) int {
    return *p
}
`

	pkg, err := buildSSA(src)
	if err != nil {
		t.Fatalf("failed to build SSA: %v", err)
	}

	summaries := AnalyzeSummaries(pkg.Prog)

	foundIdentity := false
	foundPure := false

	for name, s := range summaries {
		if s.Function != nil && s.Function.Name() == "identity" {
			foundIdentity = true
			if len(s.Params) != 1 || !s.Params[0].Escapes || !s.Params[0].Returned {
				t.Errorf("identity param expected escapes and returned, got %+v", s.Params[0])
			}
		}
		if s.Function != nil && s.Function.Name() == "pure" {
			foundPure = true
			if len(s.Params) != 1 || s.Params[0].Escapes {
				t.Errorf("pure param expected non-escaping, got %+v", s.Params[0])
			}
		}
		_ = name
	}

	if !foundIdentity || !foundPure {
		t.Errorf("failed to find function summaries: identity=%v pure=%v", foundIdentity, foundPure)
	}
}
