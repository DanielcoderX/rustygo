//go:build linux || darwin || freebsd

package rustygo

import (
	"syscall"
	"unsafe"
)

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

func allocArenaBufferWithGuard(size int) ([]byte, func([]byte) error, error) {
	pageSize := syscall.Getpagesize()
	alignedSize := (size + pageSize - 1) & ^(pageSize - 1)
	total := alignedSize + pageSize
	buf, err := syscall.Mmap(
		-1,
		0,
		total,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		return nil, nil, err
	}
	if err := syscall.Mprotect(buf[alignedSize:], syscall.PROT_NONE); err != nil {
		_ = syscall.Munmap(buf)
		return nil, nil, err
	}
	return buf[:size], func(b []byte) error {
		orig := unsafe.Slice(&b[0], total)
		return syscall.Munmap(orig)
	}, nil
}
