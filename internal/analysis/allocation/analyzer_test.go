package allocation

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

func TestAllocationDiscovery(t *testing.T) {
	src := `package main
func f() {
    x := new(int)
    _ = x
    s := make([]int, 10)
    _ = s
}`

	pkg, err := buildSSA(src)
	if err != nil {
		t.Fatalf("failed to build SSA: %v", err)
	}

	var res *Result
	for _, member := range pkg.Members {
		if fn, ok := member.(*ssa.Function); ok && fn.Name() == "f" {
			res = AnalyzeFunction(fn)
		}
	}

	if res == nil || len(res.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %v", res)
	}
}
