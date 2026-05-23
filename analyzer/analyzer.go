package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name:       "rustygocheck",
	Doc:        "finds allocations eligible for rustygo arena management",
	Run:        run,
	ResultType: reflect.TypeOf((*Result)(nil)),
}

type Finding struct {
	Pos      token.Pos
	Kind     string
	TypeName string
	Fixable  bool
}

type Result struct {
	Findings []Finding
	Total    int
	Eligible int
}

type site struct {
	pos      token.Pos
	end      token.Pos
	kind     string
	typeName string
	target   *types.Var
	fixable  bool

	body     *ast.BlockStmt
	call     *ast.CallExpr
	typeExpr ast.Expr
	lenExpr  ast.Expr
	capExpr  ast.Expr
}

func run(pass *analysis.Pass) (interface{}, error) {
	result := &Result{}

	for _, file := range pass.Files {
		if isGenerated(file) {
			continue
		}
		allSites := collectSites(file, pass.TypesInfo)
		result.Total += len(allSites)
		for _, site := range filterEligibleSites(allSites, pass.TypesInfo) {
			result.Eligible++
			pass.Reportf(site.pos, "[rustygo] %s(%s) is arena-eligible", site.kind, site.typeName)
			result.Findings = append(result.Findings, Finding{
				Pos:      site.pos,
				Kind:     site.kind,
				TypeName: site.typeName,
				Fixable:  site.fixable,
			})
		}
	}

	if result.Total > 0 {
		pct := float64(result.Eligible) / float64(result.Total) * 100
		fmt.Printf("\n[rustygo] arena eligibility: %d/%d (%.1f%%)\n\n", result.Eligible, result.Total, pct)
	}

	return result, nil
}

func collectSites(file *ast.File, info *types.Info) []site {
	var sites []site
	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			sites = append(sites, collectCandidates(fn.Body, info)...)
			return false
		case *ast.FuncLit:
			sites = append(sites, collectCandidates(fn.Body, info)...)
			return false
		default:
			return true
		}
	})
	return sites
}

func filterEligibleSites(candidates []site, info *types.Info) []site {
	var eligible []site
	for _, candidate := range candidates {
		if candidate.target == nil || candidateEscapes(candidate.body, info, candidate.target) {
			continue
		}
		eligible = append(eligible, candidate)
	}
	return eligible
}

func collectCandidates(body *ast.BlockStmt, info *types.Info) []site {
	var candidates []site
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			candidates = append(candidates, sitesFromAssign(stmt, body, info)...)
		case *ast.DeclStmt:
			gen, ok := stmt.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				candidates = append(candidates, sitesFromValueSpec(valueSpec, body, info)...)
			}
		}
		return true
	})
	return candidates
}

func sitesFromAssign(stmt *ast.AssignStmt, body *ast.BlockStmt, info *types.Info) []site {
	if stmt.Tok != token.DEFINE && stmt.Tok != token.ASSIGN {
		return nil
	}

	n := len(stmt.Lhs)
	if len(stmt.Rhs) < n {
		n = len(stmt.Rhs)
	}

	var sites []site
	for i := 0; i < n; i++ {
		ident, ok := stmt.Lhs[i].(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}
		obj, ok := info.ObjectOf(ident).(*types.Var)
		if !ok {
			continue
		}
		if site, ok := makeSite(obj, body, stmt.Rhs[i], info); ok {
			sites = append(sites, site)
		}
	}
	return sites
}

func sitesFromValueSpec(spec *ast.ValueSpec, body *ast.BlockStmt, info *types.Info) []site {
	n := len(spec.Names)
	if len(spec.Values) < n {
		n = len(spec.Values)
	}

	var sites []site
	for i := 0; i < n; i++ {
		name := spec.Names[i]
		if name == nil || name.Name == "_" {
			continue
		}
		obj, ok := info.ObjectOf(name).(*types.Var)
		if !ok {
			continue
		}
		if site, ok := makeSite(obj, body, spec.Values[i], info); ok {
			sites = append(sites, site)
		}
	}
	return sites
}

func makeSite(obj *types.Var, body *ast.BlockStmt, expr ast.Expr, info *types.Info) (site, bool) {
	switch expr := expr.(type) {
	case *ast.CallExpr:
		ident, ok := expr.Fun.(*ast.Ident)
		if !ok {
			return site{}, false
		}
		switch ident.Name {
		case "new":
			if len(expr.Args) != 1 {
				return site{}, false
			}
			return site{
				pos:      expr.Pos(),
				end:      expr.End(),
				kind:     "new",
				typeName: shortTypeString(info.TypeOf(expr.Args[0])),
				target:   obj,
				fixable:  true,
				body:     body,
				call:     expr,
				typeExpr: expr.Args[0],
			}, true
		case "make":
			if len(expr.Args) < 2 {
				return site{}, false
			}
			t := info.TypeOf(expr.Args[0])
			if t == nil {
				return site{}, false
			}
			if _, ok := t.Underlying().(*types.Slice); !ok {
				return site{}, false
			}
			site := site{
				pos:      expr.Pos(),
				end:      expr.End(),
				kind:     "make",
				typeName: shortTypeString(t),
				target:   obj,
				fixable:  true,
				body:     body,
				call:     expr,
				typeExpr: expr.Args[0],
				lenExpr:  expr.Args[1],
			}
			if len(expr.Args) > 2 {
				site.capExpr = expr.Args[2]
			}
			return site, true
		}
	case *ast.CompositeLit:
		t := info.TypeOf(expr)
		if t == nil {
			return site{}, false
		}
		if _, ok := t.Underlying().(*types.Struct); !ok {
			return site{}, false
		}
		return site{
			pos:      expr.Pos(),
			end:      expr.End(),
			kind:     "literal",
			typeName: shortTypeString(t),
			target:   obj,
			body:     body,
		}, true
	}
	return site{}, false
}

func candidateEscapes(body *ast.BlockStmt, info *types.Info, obj *types.Var) bool {
	escapes := false

	ast.Inspect(body, func(n ast.Node) bool {
		if escapes {
			return false
		}

		switch node := n.(type) {
		case *ast.ReturnStmt:
			for _, result := range node.Results {
				if escapingValueUse(result, info, obj) {
					escapes = true
					return false
				}
			}
		case *ast.AssignStmt:
			for _, rhs := range node.Rhs {
				if escapingValueUse(rhs, info, obj) {
					escapes = true
					return false
				}
			}
		case *ast.CallExpr:
			if builtinSafeCall(node, info, obj) {
				return true
			}
			for _, arg := range node.Args {
				if escapingValueUse(arg, info, obj) {
					escapes = true
					return false
				}
			}
		case *ast.SendStmt:
			if escapingValueUse(node.Value, info, obj) {
				escapes = true
				return false
			}
		case *ast.UnaryExpr:
			if node.Op == token.AND && rootedInObject(node.X, info, obj) {
				escapes = true
				return false
			}
		}

		return true
	})

	return escapes
}

func builtinSafeCall(call *ast.CallExpr, info *types.Info, obj *types.Var) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name != "len" && ident.Name != "cap" {
		return false
	}
	if len(call.Args) != 1 {
		return false
	}
	return directUseOf(call.Args[0], info, obj)
}

func escapingValueUse(expr ast.Expr, info *types.Info, obj *types.Var) bool {
	switch expr := unparen(expr).(type) {
	case *ast.Ident:
		return directUseOf(expr, info, obj)
	case *ast.CompositeLit:
		for _, elt := range expr.Elts {
			if escapingValueUse(elt, info, obj) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return escapingValueUse(expr.Key, info, obj) || escapingValueUse(expr.Value, info, obj)
	case *ast.CallExpr:
		if builtinSafeCall(expr, info, obj) {
			return false
		}
		for _, arg := range expr.Args {
			if escapingValueUse(arg, info, obj) {
				return true
			}
		}
	case *ast.SelectorExpr:
		if directUseOf(expr.X, info, obj) {
			return false
		}
		return escapingValueUse(expr.X, info, obj)
	case *ast.IndexExpr:
		if rootedInObject(expr.X, info, obj) {
			return false
		}
		return escapingValueUse(expr.X, info, obj) || escapingValueUse(expr.Index, info, obj)
	case *ast.IndexListExpr:
		if rootedInObject(expr.X, info, obj) {
			return false
		}
		if escapingValueUse(expr.X, info, obj) {
			return true
		}
		for _, index := range expr.Indices {
			if escapingValueUse(index, info, obj) {
				return true
			}
		}
		return false
	case *ast.SliceExpr:
		return rootedInObject(expr.X, info, obj)
	case *ast.TypeAssertExpr:
		return directUseOf(expr.X, info, obj)
	case *ast.UnaryExpr:
		if expr.Op == token.AND {
			return rootedInObject(expr.X, info, obj)
		}
	}
	return false
}

func rootedInObject(expr ast.Expr, info *types.Info, obj *types.Var) bool {
	switch expr := unparen(expr).(type) {
	case *ast.Ident:
		return directUseOf(expr, info, obj)
	case *ast.SelectorExpr:
		return rootedInObject(expr.X, info, obj)
	case *ast.IndexExpr:
		return rootedInObject(expr.X, info, obj)
	case *ast.IndexListExpr:
		return rootedInObject(expr.X, info, obj)
	case *ast.SliceExpr:
		return rootedInObject(expr.X, info, obj)
	}
	return false
}

func directUseOf(expr ast.Expr, info *types.Info, obj *types.Var) bool {
	ident, ok := unparen(expr).(*ast.Ident)
	if !ok {
		return false
	}
	use, ok := info.ObjectOf(ident).(*types.Var)
	return ok && use == obj
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func shortTypeString(t types.Type) string {
	if t == nil {
		return "unknown"
	}
	return types.TypeString(t, func(*types.Package) string { return "" })
}

func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "Code generated") || strings.Contains(c.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}
