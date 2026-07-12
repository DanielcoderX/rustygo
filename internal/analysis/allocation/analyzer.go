package allocation

import (
	"golang.org/x/tools/go/ssa"
)

var lastResult *Result
var idCounter int

// Analyze discovers allocations in all functions of an SSA program.
func Analyze(program *ssa.Program) (*Result, error) {
	res := &Result{
		Allocations: make(map[int]*Allocation),
	}
	idCounter = 0

	for _, pkg := range program.AllPackages() {
		for _, member := range pkg.Members {
			if fn, ok := member.(*ssa.Function); ok {
				analyzeFuncInternal(fn, res)
			}
		}
	}

	lastResult = res
	return res, nil
}

// AnalyzeFunction discovers allocations inside a single SSA function.
func AnalyzeFunction(fn *ssa.Function) {
	res := &Result{
		Allocations: make(map[int]*Allocation),
	}
	idCounter = 0
	analyzeFuncInternal(fn, res)
	lastResult = res
}

// GetAllocation returns the allocation with the given stable ID.
func GetAllocation(id int) *Allocation {
	if lastResult == nil {
		return nil
	}
	return lastResult.Allocations[id]
}

func analyzeFuncInternal(fn *ssa.Function, res *Result) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			var isAlloc bool
			switch instr.(type) {
			case *ssa.Alloc, *ssa.MakeSlice, *ssa.MakeMap, *ssa.MakeChan:
				isAlloc = true
			}

			if isAlloc {
				val := instr.(ssa.Value)
				idCounter++
				pos := fn.Prog.Fset.Position(instr.Pos())
				res.Allocations[idCounter] = &Allocation{
					ID:          idCounter,
					Type:        val.Type(),
					Function:    fn,
					Instruction: instr,
					Position:    pos,
				}
			}
		}
	}
}
