//go:build (js && wasm) || wasip1 || (!windows && !linux && !darwin && !freebsd)

package rustygo

func allocArenaBuffer(size int) ([]byte, func([]byte) error, error) {
	buf := make([]byte, size)
	release := func([]byte) error {
		return nil
	}
	return buf, release, nil
}

func allocArenaBufferWithGuard(size int) ([]byte, func([]byte) error, error) {
	// Guard pages cannot be hardware memory-mapped with NOACCESS in WASM linear space;
	// fallback to standard continuous linear memory slab.
	return allocArenaBuffer(size)
}
