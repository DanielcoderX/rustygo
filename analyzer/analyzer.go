package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
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

	var allFuncs []*ssa.Function
	var collectFuncs func(fn *ssa.Function)
	collectFuncs = func(fn *ssa.Function) {
		allFuncs = append(allFuncs, fn)
		for _, anon := range fn.AnonFuncs {
			collectFuncs(anon)
		}
	}
	for _, member := range ssaPkg.Members {
		if fn, ok := member.(*ssa.Function); ok {
			collectFuncs(fn)
		}
	}

	paramEscapes := make(map[*ssa.Parameter]bool)
	for {
		changed := false
		for _, fn := range allFuncs {
			for _, param := range fn.Params {
				if paramEscapes[param] {
					continue
				}
				visited := make(map[ssa.Value]bool)
				if ssaValueEscapes(param, paramEscapes, visited) {
					paramEscapes[param] = true
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	var allocations []ssa.Value
	for _, fn := range allFuncs {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				switch x := instr.(type) {
				case *ssa.Alloc:
					allocations = append(allocations, x)
				case *ssa.MakeSlice:
					allocations = append(allocations, x)
				case *ssa.MakeMap:
					allocations = append(allocations, x)
				case *ssa.MakeChan:
					allocations = append(allocations, x)
				}
			}
		}
	}

	var eligible []site
	for _, candidate := range candidates {
		var matchingAlloc ssa.Value
		for _, alloc := range allocations {
			posAlloc := fset.Position(alloc.Pos())
			posCand := fset.Position(candidate.pos)
			if posAlloc.Line == posCand.Line && posAlloc.Filename == posCand.Filename {
				matchingAlloc = alloc
				break
			}
		}

		if matchingAlloc == nil {
			continue
		}

		visited := make(map[ssa.Value]bool)
		if !ssaValueEscapes(matchingAlloc, paramEscapes, visited) {
			eligible = append(eligible, candidate)
		}
	}
	return eligible
}

func ssaValueEscapes(v ssa.Value, paramEscapes map[*ssa.Parameter]bool, visited map[ssa.Value]bool) bool {
	if v == nil {
		return false
	}
	if visited[v] {
		return false
	}
	visited[v] = true

	fn := v.Parent()
	if fn == nil {
		return true
	}

	var uses []ssa.Instruction
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for _, op := range instr.Operands(nil) {
				if op != nil && *op == v {
					uses = append(uses, instr)
					break
				}
			}
		}
	}

	for _, instr := range uses {
		switch x := instr.(type) {
		case *ssa.Return:
			return true

		case *ssa.Store:
			if x.Val == v {
				root := getRoot(x.Addr)
				if root == nil {
					return true
				}
				if _, ok := root.(*ssa.Global); ok {
					return true
				}
				if p, ok := root.(*ssa.Parameter); ok {
					if paramEscapes[p] {
						return true
					}
					if ssaValueEscapes(p, paramEscapes, visited) {
						return true
					}
				}
				if root != v && ssaValueEscapes(root, paramEscapes, visited) {
					return true
				}
			}

		case *ssa.Send:
			if x.X == v {
				return true
			}

		case *ssa.Call:
			argIdx := -1
			for idx, arg := range x.Call.Args {
				if arg == v {
					argIdx = idx
					break
				}
			}
			if argIdx >= 0 {
				if x.Call.Value != nil {
					if builtin, ok := x.Call.Value.(*ssa.Builtin); ok {
						if builtin.Name() == "len" || builtin.Name() == "cap" || builtin.Name() == "append" {
							continue
						}
					}
				}
				callee := x.Call.StaticCallee()
				if callee != nil {
					if callee.Pkg != nil && callee.Pkg.Pkg.Path() == fn.Pkg.Pkg.Path() {
						if argIdx < len(callee.Params) {
							param := callee.Params[argIdx]
							if paramEscapes[param] {
								return true
							}
							if ssaValueEscapes(param, paramEscapes, visited) {
								return true
							}
						}
					} else {
						return true
					}
				} else {
					return true
				}
			}

		case *ssa.FieldAddr:
			if ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		case *ssa.IndexAddr:
			if ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		case *ssa.Slice:
			if ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}

		case *ssa.Field:
			if hasPointers(x.Type()) && ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		case *ssa.Index:
			if hasPointers(x.Type()) && ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		case *ssa.UnOp:
			if x.Op == token.MUL {
				if hasPointers(x.Type()) && ssaValueEscapes(x, paramEscapes, visited) {
					return true
				}
			} else if x.Op == token.ARROW {
				// Receive from channel does not make the channel escape
				continue
			} else {
				if ssaValueEscapes(x, paramEscapes, visited) {
					return true
				}
			}
		case *ssa.BinOp:
			if ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		case *ssa.ChangeType:
			if ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		case *ssa.Convert:
			if ssaValueEscapes(x, paramEscapes, visited) {
				return true
			}
		}
	}

	return false
}

func getRoot(v ssa.Value) ssa.Value {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case *ssa.FieldAddr:
		return getRoot(x.X)
	case *ssa.IndexAddr:
		return getRoot(x.X)
	case *ssa.Field:
		return getRoot(x.X)
	case *ssa.Index:
		return getRoot(x.X)
	case *ssa.UnOp:
		if x.Op == token.MUL {
			return getRoot(x.X)
		}
	}
	return v
}

func hasPointers(t types.Type) bool {
	if t == nil {
		return false
	}
	switch ut := t.Underlying().(type) {
	case *types.Pointer, *types.Signature, *types.Map, *types.Chan, *types.Slice, *types.Interface:
		return true
	case *types.Struct:
		for i := 0; i < ut.NumFields(); i++ {
			if hasPointers(ut.Field(i).Type()) {
				return true
			}
		}
	case *types.Array:
		return hasPointers(ut.Elem())
	}
	return false
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
