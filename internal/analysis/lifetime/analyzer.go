package lifetime

import (
	"fmt"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Analyze runs the lifetime analysis pass over an entire ssa.Program.
func Analyze(program *ssa.Program) (*LifetimeResult, error) {
	res := &LifetimeResult{
		Reports:     make(map[string]*LifetimeReport),
		ResultState: Safe,
	}

	for _, pkg := range program.AllPackages() {
		pkgRes := analyzePkgInternal(pkg)
		for k, v := range pkgRes.Reports {
			res.Reports[k] = v
		}
		res.Violations = append(res.Violations, pkgRes.Violations...)
	}

	if len(res.Violations) > 0 {
		res.ResultState = Unsafe
	}

	lastResult = res
	return res, nil
}

// AnalyzePackage runs the lifetime analysis pass over a single package.
func AnalyzePackage(pkg *ssa.Package) {
	res := analyzePkgInternal(pkg)
	lastResult = res
}

func analyzePkgInternal(pkg *ssa.Package) *LifetimeResult {
	res := &LifetimeResult{
		Reports:     make(map[string]*LifetimeReport),
		ResultState: Safe,
	}

	for _, member := range pkg.Members {
		if fn, ok := member.(*ssa.Function); ok {
			fnRes := analyzeFuncInternal(fn)
			for k, v := range fnRes.Reports {
				res.Reports[k] = v
			}
			res.Violations = append(res.Violations, fnRes.Violations...)
		}
	}

	return res
}

// AnalyzeFunction runs the lifetime analysis pass over a single function.
func AnalyzeFunction(fn *ssa.Function) {
	res := analyzeFuncInternal(fn)
	lastResult = res
}

func analyzeFuncInternal(fn *ssa.Function) *LifetimeResult {
	res := &LifetimeResult{
		Reports:     make(map[string]*LifetimeReport),
		ResultState: Safe,
	}

	if fn == nil || len(fn.Blocks) == 0 {
		return res
	}

	regions := BuildRegions(fn)
	tracker := NewOwnershipTracker()
	g := NewLifetimeGraph()

	var allocations []ssa.Value

	// First pass: identify allocations and register basic nodes
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			switch x := instr.(type) {
			case *ssa.Alloc:
				allocations = append(allocations, x)
				tracker.RegisterAlloc(x, fn.Name())
				g.GetOrCreateNode(x, nil, fmt.Sprintf("Alloc(%s)", x.Type().String()))
			case *ssa.MakeSlice:
				allocations = append(allocations, x)
				tracker.RegisterAlloc(x, fn.Name())
				g.GetOrCreateNode(x, nil, fmt.Sprintf("MakeSlice(%s)", x.Type().String()))
			case *ssa.MakeMap:
				allocations = append(allocations, x)
				tracker.RegisterAlloc(x, fn.Name())
				g.GetOrCreateNode(x, nil, fmt.Sprintf("MakeMap(%s)", x.Type().String()))
			case *ssa.MakeChan:
				allocations = append(allocations, x)
				tracker.RegisterAlloc(x, fn.Name())
				g.GetOrCreateNode(x, nil, fmt.Sprintf("MakeChan(%s)", x.Type().String()))
			}
		}
	}

	// Second pass: trace flows, assignments, and dependencies
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			val, ok := instr.(ssa.Value)
			if ok {
				// Register instruction node
				g.GetOrCreateNode(val, nil, val.Name())
			}

			switch x := instr.(type) {
			case *ssa.Store:
				valNode := g.GetOrCreateNode(x.Val, nil, "val")
				addrNode := g.GetOrCreateNode(x.Addr, nil, "addr")
				g.AddEdge(valNode, addrNode, "Store")
				tracker.AddAlias(x.Addr, x.Val)

			case *ssa.Field:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "Field")
				g.AddEdge(valNode, xNode, "FieldOf")
				tracker.AddAlias(val, x.X)

			case *ssa.FieldAddr:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "FieldAddr")
				g.AddEdge(valNode, xNode, "FieldAddrOf")
				tracker.AddAlias(val, x.X)

			case *ssa.IndexAddr:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "IndexAddr")
				g.AddEdge(valNode, xNode, "IndexAddrOf")
				tracker.AddAlias(val, x.X)

			case *ssa.Slice:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "Slice")
				if HasPointers(val.Type()) {
					tracker.AddAlias(val, x.X)
				}

			case *ssa.ChangeInterface:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "ChangeInterface")
				tracker.AddAlias(val, x.X)

			case *ssa.Convert:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "Convert")
				tracker.AddAlias(val, x.X)

			case *ssa.TypeAssert:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "TypeAssert")
				tracker.AddAlias(val, x.X)

			case *ssa.Phi:
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				for _, edgeVal := range x.Edges {
					edgeNode := g.GetOrCreateNode(edgeVal, nil, edgeVal.Name())
					g.AddEdge(edgeNode, valNode, "Phi")
					tracker.AddAlias(val, edgeVal)
				}

			case *ssa.Select:
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				for _, state := range x.States {
					stateNode := g.GetOrCreateNode(state.Chan, nil, "select_chan")
					g.AddEdge(stateNode, valNode, "Select")
					tracker.AddAlias(val, state.Chan)
				}

			case *ssa.MakeClosure:
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				for _, bind := range x.Bindings {
					bindNode := g.GetOrCreateNode(bind, nil, "closure_bind")
					g.AddEdge(bindNode, valNode, "ClosureBind")
					tracker.AddAlias(val, bind)
				}

			case *ssa.MakeInterface:
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				valNode := g.GetOrCreateNode(val, nil, val.Name())
				g.AddEdge(xNode, valNode, "MakeInterface")
				miNode := g.GetOrCreateNode(nil, x, "MakeInterfaceInstr")
				g.AddEdge(xNode, miNode, "MakeInterfaceAction")
				tracker.AddAlias(val, x.X)

			case *ssa.Send:
				chanNode := g.GetOrCreateNode(x.Chan, nil, "chan")
				xNode := g.GetOrCreateNode(x.X, nil, "x")
				g.AddEdge(xNode, chanNode, "Send")
				sendNode := g.GetOrCreateNode(nil, x, "SendInstr")
				g.AddEdge(xNode, sendNode, "SendAction")

			case *ssa.Return:
				for _, retVal := range x.Results {
					retNode := g.GetOrCreateNode(retVal, nil, "retVal")
					returnNode := g.GetOrCreateNode(nil, x, "ReturnInstr")
					g.AddEdge(retNode, returnNode, "Return")
				}

			case *ssa.Go:
				goNode := g.GetOrCreateNode(nil, x, "GoInstr")
				funcNode := g.GetOrCreateNode(x.Call.Value, nil, "go_func")
				g.AddEdge(funcNode, goNode, "GoFunc")
				for _, arg := range x.Call.Args {
					argNode := g.GetOrCreateNode(arg, nil, "go_arg")
					g.AddEdge(argNode, goNode, "GoArg")
				}

			case *ssa.Defer:
				deferNode := g.GetOrCreateNode(nil, x, "DeferInstr")
				funcNode := g.GetOrCreateNode(x.Call.Value, nil, "defer_func")
				g.AddEdge(funcNode, deferNode, "DeferFunc")
				for _, arg := range x.Call.Args {
					argNode := g.GetOrCreateNode(arg, nil, "defer_arg")
					g.AddEdge(argNode, deferNode, "DeferArg")
				}

			case *ssa.Call:
				callNode := g.GetOrCreateNode(nil, x, "CallInstr")
				funcNode := g.GetOrCreateNode(x.Call.Value, nil, "call_func")
				g.AddEdge(funcNode, callNode, "CallFunc")
				for _, arg := range x.Call.Args {
					argNode := g.GetOrCreateNode(arg, nil, "call_arg")
					g.AddEdge(argNode, callNode, "CallArg")
				}

			case *ssa.UnOp:
				if x.Op == token.MUL {
					if HasPointers(x.Type()) {
						xNode := g.GetOrCreateNode(x.X, nil, "x")
						valNode := g.GetOrCreateNode(val, nil, val.Name())
						g.AddEdge(xNode, valNode, "UnOp")
						tracker.AddAlias(val, x.X)
					}
				} else if x.Op == token.ARROW {
					// Channel receive does not propagate channel lifetime
				} else {
					xNode := g.GetOrCreateNode(x.X, nil, "x")
					valNode := g.GetOrCreateNode(val, nil, val.Name())
					g.AddEdge(xNode, valNode, "UnOp")
					tracker.AddAlias(val, x.X)
				}


			}
		}
	}

	// Third pass: evaluate allocations and produce reports
	for _, alloc := range allocations {
		allocNode := g.GetOrCreateNode(alloc, nil, "")
		state, reason, violations, refs := CheckEscape(g, allocNode, tracker)

		// Map to a block region if possible
		var allocRegion *Region = regions
		for _, child := range regions.Children {
			if alloc.Pos() >= child.Start && alloc.Pos() <= child.End {
				allocRegion = child
				break
			}
		}

		id := fmt.Sprintf("alloc_%s_%d", fn.Name(), alloc.Pos())
		report := &LifetimeReport{
			ObjectID:           id,
			Function:           fn,
			Position:           fn.Prog.Fset.Position(alloc.Pos()),
			Region:             allocRegion,
			OwnerScope:         fn.Name(),
			LifetimeState:      state,
			EscapeReason:       reason,
			LifetimeViolations: violations,
			References:         refs,
		}

		res.Reports[id] = report
		if state != Safe {
			res.Violations = append(res.Violations, fmt.Sprintf("Allocation %s violated safety: %s", id, reason))
		}
	}

	return res
}

// ReportForObject retrieves the report for the given object ID.
func ReportForObject(id string) *LifetimeReport {
	if lastResult == nil {
		return nil
	}
	return lastResult.Reports[id]
}
