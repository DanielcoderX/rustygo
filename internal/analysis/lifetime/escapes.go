package lifetime

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

// CheckEscape traverses the lifetime graph from an allocation node to detect escapes.
func CheckEscape(g *LifetimeGraph, allocNode *GraphNode, tracker *OwnershipTracker) (LifetimeState, string, []ViolationType, []string) {
	visited := make(map[string]bool)
	var path []string
	var violations []ViolationType
	var reason string

	state := Safe

	var dfs func(node *GraphNode) bool
	dfs = func(curr *GraphNode) bool {
		if curr == nil {
			return false
		}
		if visited[curr.ID] {
			return false
		}
		visited[curr.ID] = true
		path = append(path, curr.Name)

		// Check node type for escapes
		if curr.Val != nil {
			if _, ok := curr.Val.(*ssa.Global); ok {
				state = Unsafe
				violations = append(violations, EscapedGlobal)
				reason = fmt.Sprintf("Stored in global variable: %s", curr.Val.Name())
				return true
			}
		}

		if curr.Instr != nil {
			switch x := curr.Instr.(type) {
			case *ssa.Return:
				state = Unsafe
				violations = append(violations, EscapedReturn)
				reason = "Returned from function scope"
				return true

			case *ssa.Go:
				state = Unsafe
				violations = append(violations, EscapedClosure)
				reason = "Captured by goroutine statement"
				return true

			case *ssa.Send:
				state = Unsafe
				violations = append(violations, EscapedChannel)
				reason = "Sent through channel"
				return true

			case *ssa.Call:
				// If external call or dynamic call
				callee := x.Call.StaticCallee()
				if callee == nil || callee.Pkg == nil {
					state = Unknown
					violations = append(violations, UnknownCall)
					reason = "Passed to unknown or external function call"
					return true
				}

			case *ssa.MakeInterface:
				state = Unknown
				violations = append(violations, EscapedInterface)
				reason = "Converted to interface value with unknown flow"
				return true
			}
		}

		// Recurse to successors
		for _, edge := range g.Adjacency[curr.ID] {
			if dfs(edge.To) {
				return true
			}
		}

		path = path[:len(path)-1]
		return false
	}

	dfs(allocNode)

	if state != Safe {
		return state, reason, violations, path
	}
	return Safe, "Allocation remains valid inside owning scope", nil, path
}

// Helper to determine if a type contains reflection or unsafe pointers.
func hasUnsafeOrReflection(t ssa.Value) bool {
	if t == nil || t.Type() == nil {
		return false
	}
	ts := t.Type().String()
	return ts == "unsafe.Pointer" || ts == "reflect.Value"
}
