//go:build windows

package rustygo

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	memCommit     = 0x1000
	memReserve    = 0x2000
	memRelease    = 0x8000
	pageReadWrite = 0x04
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc = kernel32.NewProc("VirtualAlloc")
	procVirtualFree  = kernel32.NewProc("VirtualFree")
)

func allocArenaBuffer(size int) ([]byte, func([]byte) error, error) {
	addr, _, err := procVirtualAlloc.Call(
		0,
		uintptr(size),
		uintptr(memCommit|memReserve),
		uintptr(pageReadWrite),
	)
	if addr == 0 {
		if err == syscall.Errno(0) {
			err = fmt.Errorf("VirtualAlloc failed")
		}
		return nil, nil, err
	}

	buf := unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)
	return buf, func(buf []byte) error {
		if len(buf) == 0 {
			return nil
		}
		ptr := uintptr(unsafe.Pointer(&buf[0]))
		ret, _, freeErr := procVirtualFree.Call(ptr, 0, uintptr(memRelease))
		if ret == 0 {
			if freeErr == syscall.Errno(0) {
				return fmt.Errorf("VirtualFree failed")
			}
			return freeErr
		}
		return nil
	}, nil
}
