package lifetime

import (
	"fmt"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// BuildRegions constructs the lexical region tree for a function from its SSA blocks.
func BuildRegions(fn *ssa.Function) *Region {
	if fn == nil {
		return nil
	}

	root := &Region{
		ID:    fmt.Sprintf("%s_root", fn.Name()),
		Name:  fn.Name(),
		Start: fn.Pos(),
	}

	// Find the end position of the function by scanning instructions
	var maxPos token.Pos
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Pos() > maxPos {
				maxPos = instr.Pos()
			}
		}
	}
	root.End = maxPos

	// Create sub-regions for each basic block
	for _, block := range fn.Blocks {
		var start, end token.Pos
		for _, instr := range block.Instrs {
			if start == 0 && instr.Pos() != 0 {
				start = instr.Pos()
			}
			if instr.Pos() != 0 {
				end = instr.Pos()
			}
		}

		if start != 0 && end != 0 {
			blockRegion := &Region{
				ID:     fmt.Sprintf("%s_block_%d", fn.Name(), block.Index),
				Name:   fmt.Sprintf("Block%d", block.Index),
				Parent: root,
				Start:  start,
				End:    end,
			}
			root.Children = append(root.Children, blockRegion)
		}
	}

	return root
}
