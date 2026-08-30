// Package analyzer provides AST candidate collection and AST rewriting gates.
// Per ARCHITECTURE.md, escape classification logic is consolidated onto internal/analysis/pipeline,
// which serves as the single source of truth for lifetime analysis, inter-procedural function summaries,
// and escape classification across the RustyGo toolchain.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"

	"rustygo/internal/analysis/escape"
	"rustygo/internal/analysis/pipeline"
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
	exprPtr  *ast.Expr
	call     *ast.CallExpr
	typeExpr ast.Expr
	lenExpr  ast.Expr
	capExpr  ast.Expr
}

func run(pass *analysis.Pass) (interface{}, error) {
	result := &Result{}

	funcDecls := make(map[string]*ast.FuncDecl)
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			if fd, ok := n.(*ast.FuncDecl); ok {
				if obj, ok := pass.TypesInfo.ObjectOf(fd.Name).(*types.Func); ok {
					funcDecls[obj.FullName()] = fd
				}
				return false
			}
			return true
		})
	}

	for _, file := range pass.Files {
		if isGenerated(file) {
			continue
		}
		allSites := collectSites(file, pass.TypesInfo)
		result.Total += len(allSites)
		for _, site := range filterEligibleSites(pass.Fset, file, pass.Files, pass.TypesInfo, allSites, funcDecls) {
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
			if fn.Body != nil {
				sites = append(sites, collectCandidates(fn.Body, info)...)
			}
			return false
		case *ast.FuncLit:
			if fn.Body != nil {
				sites = append(sites, collectCandidates(fn.Body, info)...)
			}
			return false
		default:
			return true
		}
	})
	return sites
}

func getPackage(info *types.Info, file *ast.File) *types.Package {
	for _, obj := range info.Defs {
		if obj != nil && obj.Pkg() != nil {
			return obj.Pkg()
		}
	}
	for _, obj := range info.Uses {
		if obj != nil && obj.Pkg() != nil {
			return obj.Pkg()
		}
	}
	if file.Name != nil {
		return types.NewPackage(file.Name.Name, file.Name.Name)
	}
	return types.NewPackage("main", "main")
}

func registerImports(prog *ssa.Program, pkg *types.Package, visited map[*types.Package]bool) {
	if visited[pkg] {
		return
	}
	visited[pkg] = true
	for _, imp := range pkg.Imports() {
		prog.CreatePackage(imp, nil, nil, true)
		registerImports(prog, imp, visited)
	}
}

func filterEligibleSites(fset *token.FileSet, file *ast.File, files []*ast.File, info *types.Info, candidates []site, funcDecls map[string]*ast.FuncDecl) []site {
	pkg := getPackage(info, file)
	prog := ssa.NewProgram(fset, 0)
	registerImports(prog, pkg, make(map[*types.Package]bool))
	ssaPkg := prog.CreatePackage(pkg, files, info, true)
	ssaPkg.Build()

	res, err := pipeline.Run(prog)
	if err != nil {
		return nil
	}

	var eligible []site
	for _, candidate := range candidates {
		posCand := fset.Position(candidate.pos)
		for _, dec := range res.Decisions {
			if dec.Decision == escape.Arena {
				posAlloc := dec.Allocation.Position
				if posAlloc.Line == posCand.Line && (posAlloc.Filename == posCand.Filename || filepath.Base(posAlloc.Filename) == filepath.Base(posCand.Filename)) {
					eligible = append(eligible, candidate)
					break
				}
			}
		}
	}
	return eligible
}

type candidateVisitor struct {
	body       *ast.BlockStmt
	info       *types.Info
	loopDepth  int
	candidates []site
}

func (v *candidateVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}

	switch stmt := node.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return &candidateVisitor{
			body:       v.body,
			info:       v.info,
			loopDepth:  v.loopDepth + 1,
			candidates: v.candidates,
		}
	case *ast.AssignStmt:
		if v.loopDepth == 0 {
			v.candidates = append(v.candidates, sitesFromAssign(stmt, v.body, v.info)...)
		}
	case *ast.DeclStmt:
		if v.loopDepth == 0 {
			gen, ok := stmt.Decl.(*ast.GenDecl)
			if ok && gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					if valueSpec, ok := spec.(*ast.ValueSpec); ok {
						v.candidates = append(v.candidates, sitesFromValueSpec(valueSpec, v.body, v.info)...)
					}
				}
			}
		}
	}
	return v
}

func collectCandidates(body *ast.BlockStmt, info *types.Info) []site {
	v := &candidateVisitor{
		body: body,
		info: info,
	}
	ast.Walk(v, body)
	return v.candidates
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
		if site, ok := makeSite(obj, body, &stmt.Rhs[i], info); ok {
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
		if site, ok := makeSite(obj, body, &spec.Values[i], info); ok {
			sites = append(sites, site)
		}
	}
	return sites
}

func makeSite(obj *types.Var, body *ast.BlockStmt, exprPtr *ast.Expr, info *types.Info) (site, bool) {
	expr := *exprPtr
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
				exprPtr:  exprPtr,
				call:     expr,
				typeExpr: expr.Args[0],
			}, true
		case "make":
			if len(expr.Args) < 1 {
				return site{}, false
			}
			t := info.TypeOf(expr.Args[0])
			if t == nil {
				return site{}, false
			}
			switch ut := t.Underlying().(type) {
			case *types.Slice:
				if len(expr.Args) < 2 {
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
					exprPtr:  exprPtr,
					call:     expr,
					typeExpr: expr.Args[0],
					lenExpr:  expr.Args[1],
				}
				if len(expr.Args) > 2 {
					site.capExpr = expr.Args[2]
				}
				return site, true
			case *types.Map:
				site := site{
					pos:      expr.Pos(),
					end:      expr.End(),
					kind:     "make_map",
					typeName: shortTypeString(ut),
					target:   obj,
					fixable:  true,
					body:     body,
					exprPtr:  exprPtr,
					typeExpr: expr.Args[0],
				}
				if len(expr.Args) > 1 {
					site.lenExpr = expr.Args[1]
				}
				return site, true
			case *types.Chan:
				site := site{
					pos:      expr.Pos(),
					end:      expr.End(),
					kind:     "make_chan",
					typeName: shortTypeString(t),
					target:   obj,
					fixable:  true,
					body:     body,
					exprPtr:  exprPtr,
					typeExpr: expr.Args[0],
				}
				if len(expr.Args) > 1 {
					site.lenExpr = expr.Args[1]
				}
				return site, true
			}
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
			fixable:  true,
			body:     body,
			exprPtr:  exprPtr,
			typeExpr: expr.Type,
			lenExpr:  expr,
		}, true
	case *ast.UnaryExpr:
		if expr.Op == token.AND {
			if lit, ok := unparen(expr.X).(*ast.CompositeLit); ok {
				t := info.TypeOf(lit)
				if t != nil {
					if _, ok := t.Underlying().(*types.Struct); ok {
						return site{
							pos:      expr.Pos(),
							end:      expr.End(),
							kind:     "pointer_literal",
							typeName: shortTypeString(t),
							target:   obj,
							fixable:  true,
							body:     body,
							exprPtr:  exprPtr,
							typeExpr: lit.Type,
							lenExpr:  lit,
						}, true
					}
				}
			}
		}
	}
	return site{}, false
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
