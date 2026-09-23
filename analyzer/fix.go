package analyzer

import (
	"bytes"
	"fmt"
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
	return RewriteFileWithConfig(fset, file, []*ast.File{file}, info, pkgPath, RewriteConfig{}, funcDecls)
}

func RewriteFileWithConfig(fset *token.FileSet, file *ast.File, files []*ast.File, info *types.Info, pkgPath string, cfg RewriteConfig, funcDecls map[string]*ast.FuncDecl) ([]byte, bool, error) {
	if isGenerated(file) {
		return nil, false, nil
	}

	sites := filterEligibleSites(fset, file, files, info, collectSites(file, info), funcDecls)
	byBody := map[*ast.BlockStmt][]site{}
	for _, site := range sites {
		if !site.fixable || site.body == nil || site.exprPtr == nil {
			continue
		}
		byBody[site.body] = append(byBody[site.body], site)
	}
	if len(byBody) == 0 {
		return nil, false, nil
	}

	qualifier := rewriteQualifier(fset, file, pkgPath)
	hasBulk := false
	for body, bodySites := range byBody {
		var bulkSites []site
		var otherSites []site
		for _, s := range bodySites {
			if s.kind == "new" || s.kind == "make" || s.kind == "literal" || s.kind == "pointer_literal" {
				bulkSites = append(bulkSites, s)
			} else {
				otherSites = append(otherSites, s)
			}
		}

		scopeName := uniqueName(body, "rustygoScope")
		arenaName := uniqueName(body, "rustygoArena")
		insertScopeSetup(file, body, qualifier, arenaName, scopeName, cfg.arenaBytesOrDefault())

		if len(bulkSites) >= 2 {
			hasBulk = true
			rewriteBulk(body, bulkSites, qualifier, scopeName)
		} else {
			otherSites = append(otherSites, bulkSites...)
		}

		for _, s := range otherSites {
			rewriteSite(s, qualifier, scopeName)
		}
	}

	if hasBulk {
		astutil.AddImport(fset, file, "unsafe")
	}

	file.Comments = nil
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
			if imp.Name.Name == "_" {
				continue
			}
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

func isLoopBody(file *ast.File, body *ast.BlockStmt) bool {
	isLoop := false
	ast.Inspect(file, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.ForStmt:
			if stmt.Body == body {
				isLoop = true
				return false
			}
		case *ast.RangeStmt:
			if stmt.Body == body {
				isLoop = true
				return false
			}
		}
		return true
	})
	return isLoop
}

func insertScopeSetup(file *ast.File, body *ast.BlockStmt, qualifier, arenaName, scopeName string, arenaBytes int) {
	if isLoopBody(file, body) {
		// Non-deferred scope lifecycle for loop iterations: prevents defer frame accumulation
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
		}, body.List...)

		body.List = append(body.List,
			&ast.ExprStmt{
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(scopeName),
						Sel: ast.NewIdent("Exit"),
					},
				},
			},
			&ast.ExprStmt{
				X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(arenaName),
						Sel: ast.NewIdent("Close"),
					},
				},
			},
		)
		return
	}

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
					X:   ast.NewIdent(arenaName),
					Sel: ast.NewIdent("Close"),
				},
			},
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
		*site.exprPtr = &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     selectorOrIdent(qualifier, "AllocValue"),
				Index: site.typeExpr,
			},
			Args: []ast.Expr{ast.NewIdent(scopeName)},
		}
	case "make":
		method := "AllocSlice"
		args := []ast.Expr{ast.NewIdent(scopeName), site.lenExpr}
		if site.capExpr != nil {
			method = "AllocSliceCap"
			args = append(args, site.capExpr)
		}
		*site.exprPtr = &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     selectorOrIdent(qualifier, method),
				Index: sliceElemExpr(site.typeExpr),
			},
			Args: args,
		}
	case "make_map":
		mapType, ok := site.typeExpr.(*ast.MapType)
		if !ok {
			return
		}
		*site.exprPtr = &ast.CallExpr{
			Fun: &ast.IndexListExpr{
				X:       selectorOrIdent(qualifier, "AllocMap"),
				Indices: []ast.Expr{mapType.Key, mapType.Value},
			},
			Args: []ast.Expr{ast.NewIdent(scopeName)},
		}
	case "make_chan":
		chanType, ok := site.typeExpr.(*ast.ChanType)
		if !ok {
			return
		}
		capExpr := site.lenExpr
		if capExpr == nil {
			capExpr = &ast.BasicLit{Kind: token.INT, Value: "0"}
		}
		*site.exprPtr = &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     selectorOrIdent(qualifier, "AllocChan"),
				Index: chanType.Value,
			},
			Args: []ast.Expr{ast.NewIdent(scopeName), capExpr},
		}
	case "pointer_literal":
		rIdent := ast.NewIdent("r")
		*site.exprPtr = &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params: &ast.FieldList{},
					Results: &ast.FieldList{
						List: []*ast.Field{
							{
								Type: &ast.StarExpr{X: site.typeExpr},
							},
						},
					},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{rIdent},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{&ast.CallExpr{
								Fun: &ast.IndexExpr{
									X:     selectorOrIdent(qualifier, "AllocValue"),
									Index: site.typeExpr,
								},
								Args: []ast.Expr{ast.NewIdent(scopeName)},
							}},
						},
						&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.StarExpr{X: rIdent}},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{site.lenExpr},
						},
						&ast.ReturnStmt{
							Results: []ast.Expr{rIdent},
						},
					},
				},
			},
		}
	case "literal":
		rIdent := ast.NewIdent("r")
		iife := &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params: &ast.FieldList{},
					Results: &ast.FieldList{
						List: []*ast.Field{
							{
								Type: &ast.StarExpr{X: site.typeExpr},
							},
						},
					},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{rIdent},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{&ast.CallExpr{
								Fun: &ast.IndexExpr{
									X:     selectorOrIdent(qualifier, "AllocValue"),
									Index: site.typeExpr,
								},
								Args: []ast.Expr{ast.NewIdent(scopeName)},
							}},
						},
						&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.StarExpr{X: rIdent}},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{site.lenExpr},
						},
						&ast.ReturnStmt{
							Results: []ast.Expr{rIdent},
						},
					},
				},
			},
		}
		*site.exprPtr = &ast.StarExpr{X: iife}
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

func rewriteBulk(body *ast.BlockStmt, sites []site, qualifier, scopeName string) {
	bulkBufName := uniqueName(body, "rustygoBulk")
	var stmts []ast.Stmt
	
	prevOffExpr := ast.Expr(&ast.BasicLit{Kind: token.INT, Value: "0"})
	var lastSzExpr ast.Expr
	
	for i, s := range sites {
		szName := uniqueName(body, fmt.Sprintf("rustygoSz%d", i))
		alName := uniqueName(body, fmt.Sprintf("rustygoAl%d", i))
		offName := uniqueName(body, fmt.Sprintf("rustygoOff%d", i))
		
		var elemType ast.Expr
		if s.kind == "make" {
			elemType = sliceElemExpr(s.typeExpr)
		} else {
			elemType = s.typeExpr
		}
		
		nilCall := &ast.CallExpr{
			Fun: &ast.ParenExpr{X: &ast.StarExpr{X: elemType}},
			Args: []ast.Expr{ast.NewIdent("nil")},
		}
		starNilCall := &ast.StarExpr{X: nilCall}
		
		szVal := &ast.CallExpr{
			Fun: ast.NewIdent("int"),
			Args: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("unsafe"),
					Sel: ast.NewIdent("Sizeof"),
				},
				Args: []ast.Expr{starNilCall},
			}},
		}
		
		if s.kind == "make" {
			szVal = &ast.CallExpr{
				Fun: ast.NewIdent("int"),
				Args: []ast.Expr{&ast.BinaryExpr{
					X:  szVal,
					Op: token.MUL,
					Y:  s.lenExpr,
				}},
			}
		}
		
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(szName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{szVal},
		})
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("_")},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent(szName)},
		})
		
		alVal := &ast.CallExpr{
			Fun: ast.NewIdent("int"),
			Args: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("unsafe"),
					Sel: ast.NewIdent("Alignof"),
				},
				Args: []ast.Expr{starNilCall},
			}},
		}
		
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(alName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{alVal},
		})
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("_")},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent(alName)},
		})
		
		var offVal ast.Expr
		if i == 0 {
			offVal = &ast.BasicLit{Kind: token.INT, Value: "0"}
		} else {
			prevSum := &ast.BinaryExpr{
				X:  prevOffExpr,
				Op: token.ADD,
				Y:  lastSzExpr,
			}
			numerator := &ast.BinaryExpr{
				X: &ast.BinaryExpr{
					X:  prevSum,
					Op: token.ADD,
					Y:  ast.NewIdent(alName),
				},
				Op: token.SUB,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
			}
			mask := &ast.UnaryExpr{
				Op: token.XOR,
				X: &ast.BinaryExpr{
					X:  ast.NewIdent(alName),
					Op: token.SUB,
					Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
				},
			}
			offVal = &ast.BinaryExpr{
				X:  numerator,
				Op: token.AND,
				Y:  mask,
			}
		}
		
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(offName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{offVal},
		})
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("_")},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent(offName)},
		})
		
		rewriteSiteInBulk(s, bulkBufName, offName, elemType)
		
		prevOffExpr = ast.NewIdent(offName)
		lastSzExpr = ast.NewIdent(szName)
	}
	
	totalSizeExpr := &ast.BinaryExpr{
		X:  prevOffExpr,
		Op: token.ADD,
		Y:  lastSzExpr,
	}
	
	bulkAllocStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(bulkBufName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     selectorOrIdent(qualifier, "AllocSlice"),
				Index: ast.NewIdent("byte"),
			},
			Args: []ast.Expr{ast.NewIdent(scopeName), totalSizeExpr},
		}},
	}
	
	body.List = append(append(body.List[:4], append(stmts, bulkAllocStmt)...), body.List[4:]...)
}

func rewriteSiteInBulk(s site, bulkBufName, offName string, elemType ast.Expr) {
	ptrExpr := &ast.CallExpr{
		Fun: &ast.ParenExpr{X: &ast.StarExpr{X: elemType}},
		Args: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("unsafe"),
				Sel: ast.NewIdent("Pointer"),
			},
			Args: []ast.Expr{&ast.UnaryExpr{
				Op: token.AND,
				X: &ast.IndexExpr{
					X:     ast.NewIdent(bulkBufName),
					Index: ast.NewIdent(offName),
				},
			}},
		}},
	}
	
	switch s.kind {
	case "new":
		*s.exprPtr = ptrExpr
	case "make":
		capExpr := s.capExpr
		if capExpr == nil {
			capExpr = s.lenExpr
		}
		sliceCall := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("unsafe"),
				Sel: ast.NewIdent("Slice"),
			},
			Args: []ast.Expr{ptrExpr, capExpr},
		}
		if s.capExpr != nil {
			*s.exprPtr = &ast.SliceExpr{
				X:    sliceCall,
				High: s.lenExpr,
			}
		} else {
			*s.exprPtr = sliceCall
		}
	case "pointer_literal":
		rIdent := ast.NewIdent("r")
		*s.exprPtr = &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params: &ast.FieldList{},
					Results: &ast.FieldList{
						List: []*ast.Field{
							{
								Type: &ast.StarExpr{X: s.typeExpr},
							},
						},
					},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{rIdent},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{ptrExpr},
						},
						&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.StarExpr{X: rIdent}},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{s.lenExpr},
						},
						&ast.ReturnStmt{
							Results: []ast.Expr{rIdent},
						},
					},
				},
			},
		}
	case "literal":
		rIdent := ast.NewIdent("r")
		iife := &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Params: &ast.FieldList{},
					Results: &ast.FieldList{
						List: []*ast.Field{
							{
								Type: &ast.StarExpr{X: s.typeExpr},
							},
						},
					},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{rIdent},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{ptrExpr},
						},
						&ast.AssignStmt{
							Lhs: []ast.Expr{&ast.StarExpr{X: rIdent}},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{s.lenExpr},
						},
						&ast.ReturnStmt{
							Results: []ast.Expr{rIdent},
						},
					},
				},
			},
		}
		*s.exprPtr = &ast.StarExpr{X: iife}
	}
}
