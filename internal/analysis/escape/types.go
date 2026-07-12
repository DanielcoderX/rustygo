package escape

import (
	"rustygo/internal/analysis/allocation"
)

// Decision specifies where the allocation should reside.
type Decision int

const (
	Heap Decision = iota
	Arena
	Unknown
)

// EscapeReason identifies why an allocation was not placed in an arena.
type EscapeReason int

const (
	None EscapeReason = iota
	EscapesReturn
	EscapesClosure
	EscapesGoroutine
	EscapesChannel
	EscapesGlobal
	EscapesInterface
	EscapesMap
	EscapesSlice
	EscapesExternalCall
	EscapesUnsafe
	UnknownLifetime
)

// AllocationDecision holds the final optimization choice for a candidate.
type AllocationDecision struct {
	Allocation *allocation.Allocation
	Decision   Decision
	Reason     EscapeReason
	Path       []string
}
