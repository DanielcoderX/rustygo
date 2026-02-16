package rustygo

import "runtime/debug"

type GCState struct {
	prev int
}

// DisableGC disables Go GC and returns previous state.
func DisableGC() GCState {
	prev := debug.SetGCPercent(-1)
	return GCState{prev: prev}
}

// RestoreGC restores GC to previous state.
func RestoreGC(s GCState) {
	debug.SetGCPercent(s.prev)
}

// WithGCDisabled runs fn with GC disabled and restores the previous GC state.
func WithGCDisabled(fn func() error) error {
	if fn == nil {
		return nil
	}
	state := DisableGC()
	defer RestoreGC(state)
	return fn()
}

// WithGCPercent runs fn with a temporary GC percentage and restores the previous state.
func WithGCPercent(percent int, fn func() error) error {
	if fn == nil {
		return nil
	}
	prev := debug.SetGCPercent(percent)
	defer debug.SetGCPercent(prev)
	return fn()
}
