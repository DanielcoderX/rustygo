package rewrite

import (
	"go/ast"
	"go/token"
	"go/types"

	"rustygo/analyzer"
	"rustygo/internal/analysis/escape"
)

// Rewrite applies arena allocation transformations to an AST file based on pipeline decisions.
func Rewrite(fset *token.FileSet, file *ast.File, files []*ast.File, info *types.Info, decisions []escape.AllocationDecision, opts RewriteOptions) (*RewriteResult, error) {
	// Filter decisions for Arena allocations
	var arenaCount int
	for _, dec := range decisions {
		if dec.Decision == escape.Arena {
			arenaCount++
		}
	}

	if arenaCount == 0 {
		return &RewriteResult{
			Content:        nil,
			Changed:        false,
			RewrittenCount: 0,
		}, nil
	}

	cfg := analyzer.RewriteConfig{
		ArenaBytes: opts.ArenaBytes,
	}

	funcDecls := make(map[string]*ast.FuncDecl)
	ast.Inspect(file, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok {
			if obj, ok := info.ObjectOf(fd.Name).(*types.Func); ok {
				funcDecls[obj.FullName()] = fd
			}
			return false
		}
		return true
	})

	outBytes, changed, err := analyzer.RewriteFileWithConfig(fset, file, files, info, opts.PkgPath, cfg, funcDecls)
	if err != nil {
		return nil, err
	}

	return &RewriteResult{
		Content:        outBytes,
		Changed:        changed,
		RewrittenCount: arenaCount,
	}, nil
}
