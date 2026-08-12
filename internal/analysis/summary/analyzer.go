package summary

import (
	"golang.org/x/tools/go/ssa"
)

var lastSummaries SummaryMap

// AnalyzeSummaries builds inter-procedural function summaries for all functions in an SSA program.
func AnalyzeSummaries(prog *ssa.Program) SummaryMap {
	summaries := make(SummaryMap)

	var allFuncs []*ssa.Function
	var collectFuncs func(fn *ssa.Function)
	collectFuncs = func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		allFuncs = append(allFuncs, fn)
		for _, anon := range fn.AnonFuncs {
			collectFuncs(anon)
		}
	}

	for _, pkg := range prog.AllPackages() {
		for _, member := range pkg.Members {
			if fn, ok := member.(*ssa.Function); ok {
				collectFuncs(fn)
			}
		}
	}

	// Initialize basic summaries
	for _, fn := range allFuncs {
		fullName := fn.String()
		hasBody := len(fn.Blocks) > 0
		isExt := !hasBody || fn.Pkg == nil

		s := &FunctionSummary{
			Function:   fn,
			FullName:   fullName,
			HasBody:    hasBody,
			IsExternal: isExt,
			Params:     make([]ParamSummary, len(fn.Params)),
			Returns:    make([]ReturnSummary, fn.Signature.Results().Len()),
		}

		for i, p := range fn.Params {
			s.Params[i] = ParamSummary{
				Index: i,
				Name:  p.Name(),
			}
		}

		for j := 0; j < len(s.Returns); j++ {
			s.Returns[j] = ReturnSummary{
				Index:            j,
				DerivedFromParam: -1,
			}
		}

		summaries[fullName] = s
	}

	// Fixpoint iteration over parameter escape / return propagation
	for {
		changed := false
		for _, fn := range allFuncs {
			s := summaries[fn.String()]
			if !s.HasBody {
				continue
			}

			for i, p := range fn.Params {
				pSum := &s.Params[i]
				visited := make(map[ssa.Value]bool)
				escapes, returned, stored := analyzeValueFlow(p, summaries, visited)

				if !pSum.Escapes && escapes {
					pSum.Escapes = true
					changed = true
				}
				if !pSum.Returned && returned {
					pSum.Returned = true
					changed = true
				}
				if !pSum.Stored && stored {
					pSum.Stored = true
					changed = true
				}
			}
		}

		if !changed {
			break
		}
	}

	lastSummaries = summaries
	return summaries
}

// GetSummary returns the function summary for a given SSA function.
func GetSummary(fn *ssa.Function) *FunctionSummary {
	if fn == nil || lastSummaries == nil {
		return nil
	}
	return lastSummaries[fn.String()]
}

func analyzeValueFlow(val ssa.Value, summaries SummaryMap, visited map[ssa.Value]bool) (escapes, returned, stored bool) {
	if val == nil || visited[val] {
		return false, false, false
	}
	visited[val] = true

	fn := val.Parent()
	if fn == nil {
		return true, false, true
	}

	var uses []ssa.Instruction
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			for _, op := range instr.Operands(nil) {
				if op != nil && *op == val {
					uses = append(uses, instr)
					break
				}
			}
		}
	}

	for _, instr := range uses {
		switch x := instr.(type) {
		case *ssa.Return:
			returned = true
			escapes = true

		case *ssa.Store:
			if x.Val == val {
				stored = true
				escapes = true
			}

		case *ssa.Send:
			if x.X == val {
				escapes = true
			}

		case *ssa.Go:
			escapes = true

		case *ssa.Call:
			callee := x.Call.StaticCallee()
			if callee != nil {
				if cSum, ok := summaries[callee.String()]; ok {
					for idx, arg := range x.Call.Args {
						if arg == val && idx < len(cSum.Params) {
							if cSum.Params[idx].Escapes {
								escapes = true
							}
							if cSum.Params[idx].Stored {
								stored = true
							}
						}
					}
				} else {
					escapes = true
				}
			} else {
				escapes = true
			}

		case *ssa.FieldAddr:
			e, r, s := analyzeValueFlow(x, summaries, visited)
			escapes = escapes || e
			returned = returned || r
			stored = stored || s

		case *ssa.IndexAddr:
			e, r, s := analyzeValueFlow(x, summaries, visited)
			escapes = escapes || e
			returned = returned || r
			stored = stored || s
		}
	}

	return escapes, returned, stored
}
