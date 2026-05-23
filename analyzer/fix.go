package analyzer

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"
)

const defaultArenaBytes = 64 * 1024

type RewriteConfig struct {
	ArenaBytes int
}

func RewriteFile(fset *token.FileSet, file *ast.File, info *types.Info, pkgPath string) ([]byte, bool, error) {
	return RewriteFileWithConfig(fset, file, info, pkgPath, RewriteConfig{})
}

func RewriteFileWithConfig(fset *token.FileSet, file *ast.File, info *types.Info, pkgPath string, cfg RewriteConfig) ([]byte, bool, error) {
	if isGenerated(file) {
		return nil, false, nil
	}

	sites := filterEligibleSites(collectSites(file, info), info)
	byBody := map[*ast.BlockStmt][]site{}
	for _, site := range sites {
		if !site.fixable || site.body == nil || site.call == nil {
			continue
		}
		byBody[site.body] = append(byBody[site.body], site)
	}
	if len(byBody) == 0 {
		return nil, false, nil
	}

	qualifier := rewriteQualifier(fset, file, pkgPath)
	for body, bodySites := range byBody {
		scopeName := uniqueName(body, "rustygoScope")
		arenaName := uniqueName(body, "rustygoArena")
		insertScopeSetup(body, qualifier, arenaName, scopeName, cfg.arenaBytesOrDefault())
		for _, site := range bodySites {
			rewriteSite(site, qualifier, scopeName)
		}
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

func (cfg RewriteConfig) arenaBytesOrDefault() int {
	if cfg.ArenaBytes > 0 {
		return cfg.ArenaBytes
	}
	return defaultArenaBytes
}

func rewriteQualifier(fset *token.FileSet, file *ast.File, pkgPath string) string {
	if pkgPath == "rustygo" {
		return ""
	}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != "rustygo" {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "rustygo"
	}
	astutil.AddNamedImport(fset, file, "rg", "rustygo")
	return "rg"
}

func uniqueName(body *ast.BlockStmt, base string) string {
	name := base
	index := 2
	for hasIdentName(body, name) {
		name = base + strconv.Itoa(index)
		index++
	}
	return name
}

func hasIdentName(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func insertScopeSetup(body *ast.BlockStmt, qualifier, arenaName, scopeName string, arenaBytes int) {
	body.List = append([]ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(arenaName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  selectorOrIdent(qualifier, "NewArena"),
				Args: []ast.Expr{arenaSizeExpr(arenaBytes)},
			}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(scopeName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(arenaName),
					Sel: ast.NewIdent("EnterScope"),
				},
			}},
		},
		&ast.DeferStmt{
			Call: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(scopeName),
					Sel: ast.NewIdent("Exit"),
				},
			},
		},
	}, body.List...)
}

func arenaSizeExpr(arenaBytes int) ast.Expr {
	return &ast.BasicLit{
		Kind:  token.INT,
		Value: strconv.Itoa(arenaBytes),
	}
}

func rewriteSite(site site, qualifier, scopeName string) {
	switch site.kind {
	case "new":
		site.call.Fun = &ast.IndexExpr{
			X:     selectorOrIdent(qualifier, "AllocValue"),
			Index: site.typeExpr,
		}
		site.call.Args = []ast.Expr{ast.NewIdent(scopeName)}
	case "make":
		method := "AllocSlice"
		args := []ast.Expr{ast.NewIdent(scopeName), site.lenExpr}
		if site.capExpr != nil {
			method = "AllocSliceCap"
			args = append(args, site.capExpr)
		}
		site.call.Fun = &ast.IndexExpr{
			X:     selectorOrIdent(qualifier, method),
			Index: sliceElemExpr(site.typeExpr),
		}
		site.call.Args = args
	}
}

func sliceElemExpr(expr ast.Expr) ast.Expr {
	arrayType, ok := expr.(*ast.ArrayType)
	if !ok {
		return expr
	}
	return arrayType.Elt
}

func selectorOrIdent(qualifier, name string) ast.Expr {
	if qualifier == "" {
		return ast.NewIdent(name)
	}
	return &ast.SelectorExpr{
		X:   ast.NewIdent(qualifier),
		Sel: ast.NewIdent(name),
	}
}
