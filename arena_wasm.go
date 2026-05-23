//go:build js || wasip1

package rustygo

func allocArenaBuffer(size int) ([]byte, func([]byte) error, error) {
	buf := make([]byte, size)
	return buf, func([]byte) error { return nil }, nil
}
