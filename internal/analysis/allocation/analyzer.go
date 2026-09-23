package allocation

import (
	"sync/atomic"

	"golang.org/x/tools/go/ssa"
)

var lastResult atomic.Pointer[Result]

// Analyze discovers allocations in all functions of an SSA program.
func Analyze(program *ssa.Program) (*Result, error) {
	res := &Result{
		Allocations: make(map[int]*Allocation),
	}
	counter := 0

	for _, pkg := range program.AllPackages() {
		for _, member := range pkg.Members {
			if fn, ok := member.(*ssa.Function); ok {
				analyzeFuncInternal(fn, res, &counter)
			}
		}
	}

	lastResult.Store(res)
	return res, nil
}

// AnalyzeFunction discovers allocations inside a single SSA function.
func AnalyzeFunction(fn *ssa.Function) *Result {
	res := &Result{
		Allocations: make(map[int]*Allocation),
	}
	counter := 0
	analyzeFuncInternal(fn, res, &counter)
	lastResult.Store(res)
	return res
}

// GetAllocation returns the allocation with the given stable ID.
func GetAllocation(id int) *Allocation {
	res := lastResult.Load()
	if res == nil {
		return nil
	}
	return res.Allocations[id]
}

func analyzeFuncInternal(fn *ssa.Function, res *Result, counter *int) {
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
				*counter++
				id := *counter
				pos := fn.Prog.Fset.Position(instr.Pos())
				res.Allocations[id] = &Allocation{
					ID:          id,
					Type:        val.Type(),
					Function:    fn,
					Instruction: instr,
					Position:    pos,
				}
			}
		}
	}
}
