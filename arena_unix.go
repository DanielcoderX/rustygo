//go:build linux || darwin || freebsd

package rustygo

import "syscall"

func allocArenaBuffer(size int) ([]byte, func([]byte) error, error) {
	buf, err := syscall.Mmap(
		-1,
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, nil, err
	}
	return buf, syscall.Munmap, nil
}
