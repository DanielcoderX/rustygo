package rewrite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"

	"rustygo/internal/analysis/pipeline"
)

func buildSSAAndAST(src string) (*token.FileSet, *ast.File, []*ast.File, *types.Info, *ssa.Program, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		return nil, nil, nil, nil, nil, err
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
		return nil, nil, nil, nil, nil, err
	}

	prog := ssa.NewProgram(fset, 0)
	ssaPkg := prog.CreatePackage(pkg, []*ast.File{file}, info, true)
	ssaPkg.Build()

	return fset, file, []*ast.File{file}, info, prog, nil
}

func TestArenaRewriter(t *testing.T) {
	src := `package main
func f() int {
    x := new(int)
    *x = 42
    return *x
}`

	fset, file, files, info, prog, err := buildSSAAndAST(src)
	if err != nil {
		t.Fatalf("failed to build SSA and AST: %v", err)
	}

	res, err := pipeline.Run(prog)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	for _, d := range res.Decisions {
		t.Logf("DECISION: alloc=%v decision=%v reason=%v", d.Allocation.ID, d.Decision, d.Reason)
	}

	opts := RewriteOptions{
		ArenaBytes: 65536,
		PkgPath:    "main",
	}

	rewritten, err := Rewrite(fset, file, files, info, res.Decisions, opts)
	if err != nil {
		t.Fatalf("rewrite failed: %v", err)
	}

	if !rewritten.Changed {
		t.Errorf("expected source to be rewritten")
	}

	outStr := string(rewritten.Content)
	if !strings.Contains(outStr, "rustygoArena") || !strings.Contains(outStr, "AllocValue") {
		t.Errorf("rewritten code missing rustygo arena setup, output:\n%s", outStr)
	}
}
