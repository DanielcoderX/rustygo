package lifetime

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// LifetimeState represents the safety status of an allocation.
type LifetimeState string

const (
	Safe    LifetimeState = "SAFE"
	Unsafe  LifetimeState = "UNSAFE"
	Unknown LifetimeState = "UNKNOWN"
)

// ViolationType defines the type of lifetime violation encountered.
type ViolationType string

const (
	EscapedReturn     ViolationType = "EscapedReturn"
	EscapedClosure    ViolationType = "EscapedClosure"
	EscapedGlobal     ViolationType = "EscapedGlobal"
	EscapedInterface  ViolationType = "EscapedInterface"
	EscapedChannel    ViolationType = "EscapedChannel"
	EscapedMap        ViolationType = "EscapedMap"
	EscapedSlice      ViolationType = "EscapedSlice"
	EscapedReflection ViolationType = "EscapedReflection"
	EscapedUnsafe     ViolationType = "EscapedUnsafe"
	UnknownCall       ViolationType = "UnknownCall"
	UnknownAlias      ViolationType = "UnknownAlias"
	UnknownOwnership  ViolationType = "UnknownOwnership"
	UnknownRegion     ViolationType = "UnknownRegion"
)

// Region represents a lexical scope region.
type Region struct {
	ID       string
	Parent   *Region
	Children []*Region
	Start    token.Pos
	End      token.Pos
	Name     string
}

// LifetimeReport contains the lifetime details of a specific allocation.
type LifetimeReport struct {
	ObjectID           string
	Function           *ssa.Function
	Position           token.Position
	Region             *Region
	OwnerScope         string
	LifetimeState      LifetimeState
	EscapeReason       string
	LifetimeViolations []ViolationType
	References         []string
}

// LifetimeResult is the overall output of the lifetime analysis.
type LifetimeResult struct {
	Reports     map[string]*LifetimeReport
	Violations  []string
	ResultState LifetimeState
}

// HasPointers returns true if type t can contain pointers.
func HasPointers(t types.Type) bool {
	if t == nil {
		return false
	}
	switch ut := t.Underlying().(type) {
	case *types.Pointer, *types.Signature, *types.Map, *types.Chan, *types.Slice, *types.Interface:
		return true
	case *types.Struct:
		for i := 0; i < ut.NumFields(); i++ {
			if HasPointers(ut.Field(i).Type()) {
				return true
			}
		}
	case *types.Array:
		return HasPointers(ut.Elem())
	}
	return false
}
