package lifetime

import (
	"golang.org/x/tools/go/ssa"
)

// OwnershipTracker tracks the ownership and aliases of allocations.
type OwnershipTracker struct {
	aliases map[ssa.Value]ssa.Value
	owners  map[ssa.Value]string
}

// NewOwnershipTracker initializes a new tracker.
func NewOwnershipTracker() *OwnershipTracker {
	return &OwnershipTracker{
		aliases: make(map[ssa.Value]ssa.Value),
		owners:  make(map[ssa.Value]string),
	}
}

// RegisterAlloc registers a new allocation and its initial owner.
func (t *OwnershipTracker) RegisterAlloc(alloc ssa.Value, ownerScope string) {
	t.aliases[alloc] = alloc
	t.owners[alloc] = ownerScope
}

// AddAlias records that value v is an alias of the root allocation of src.
func (t *OwnershipTracker) AddAlias(v, src ssa.Value) {
	if root := t.GetRoot(src); root != nil {
		t.aliases[v] = root
	}
}

// GetRoot retrieves the root allocation for any value, or nil if not tracked.
func (t *OwnershipTracker) GetRoot(v ssa.Value) ssa.Value {
	if v == nil {
		return nil
	}
	if root, ok := t.aliases[v]; ok {
		if root == v {
			return root
		}
		// Path compression
		resolved := t.GetRoot(root)
		t.aliases[v] = resolved
		return resolved
	}
	return nil
}

// GetOwner returns the owner scope of a root allocation.
func (t *OwnershipTracker) GetOwner(v ssa.Value) string {
	if root := t.GetRoot(v); root != nil {
		return t.owners[root]
	}
	return ""
}
