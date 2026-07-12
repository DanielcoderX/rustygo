package allocation

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Allocation represents a discovered memory allocation candidate.
type Allocation struct {
	ID          int
	Type        types.Type
	Function    *ssa.Function
	Instruction ssa.Instruction
	Position    token.Position
}

// Result holds all discovered allocations mapped by their stable ID.
type Result struct {
	Allocations map[int]*Allocation
}
