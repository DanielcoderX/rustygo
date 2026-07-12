package escape

import (
	"fmt"

	"rustygo/internal/analysis/allocation"
	"rustygo/internal/analysis/lifetime"
)

// Classify maps allocation and lifetime results into optimization decisions.
func Classify(allocs *allocation.Result, lifetimes *lifetime.LifetimeResult) []AllocationDecision {
	var decisions []AllocationDecision

	for _, alloc := range allocs.Allocations {
		id := fmt.Sprintf("alloc_%s_%d", alloc.Function.Name(), alloc.Instruction.Pos())
		report, ok := lifetimes.Reports[id]

		dec := Unknown
		reason := UnknownLifetime
		var path []string

		if ok {
			path = report.References
			switch report.LifetimeState {
			case lifetime.Safe:
				dec = Arena
				reason = None
			case lifetime.Unsafe:
				dec = Heap
				reason = mapViolation(report.LifetimeViolations)
			case lifetime.Unknown:
				dec = Unknown
				reason = mapViolation(report.LifetimeViolations)
			}
		}

		decisions = append(decisions, AllocationDecision{
			Allocation: alloc,
			Decision:   dec,
			Reason:     reason,
			Path:       path,
		})
	}

	return decisions
}

func mapViolation(violations []lifetime.ViolationType) EscapeReason {
	if len(violations) == 0 {
		return UnknownLifetime
	}
	switch violations[0] {
	case lifetime.EscapedReturn:
		return EscapesReturn
	case lifetime.EscapedClosure:
		return EscapesClosure
	case lifetime.EscapedGlobal:
		return EscapesGlobal
	case lifetime.EscapedInterface:
		return EscapesInterface
	case lifetime.EscapedChannel:
		return EscapesChannel
	case lifetime.EscapedMap:
		return EscapesMap
	case lifetime.EscapedSlice:
		return EscapesSlice
	case lifetime.EscapedReflection, lifetime.EscapedUnsafe:
		return EscapesUnsafe
	case lifetime.UnknownCall:
		return EscapesExternalCall
	}
	return UnknownLifetime
}
