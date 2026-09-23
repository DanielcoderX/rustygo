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
	pageNoAccess  = 0x01
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc   = kernel32.NewProc("VirtualAlloc")
	procVirtualFree    = kernel32.NewProc("VirtualFree")
	procVirtualProtect = kernel32.NewProc("VirtualProtect")
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

	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	buf := unsafe.Slice((*byte)(ptr), size)
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

func allocArenaBufferWithGuard(size int) ([]byte, func([]byte) error, error) {
	const pageSize = 4096
	total := size + pageSize
	addr, _, err := procVirtualAlloc.Call(
		0,
		uintptr(total),
		uintptr(memCommit|memReserve),
		uintptr(pageReadWrite),
	)
	if addr == 0 {
		if err == syscall.Errno(0) {
			err = fmt.Errorf("VirtualAlloc failed")
		}
		return nil, nil, err
	}

	guardAddr := addr + uintptr(size)
	var oldProtect uint32
	ret, _, pErr := procVirtualProtect.Call(
		guardAddr,
		uintptr(pageSize),
		uintptr(pageNoAccess),
		uintptr(unsafe.Pointer(&oldProtect)),
	)
	if ret == 0 {
		if pErr == syscall.Errno(0) {
			pErr = fmt.Errorf("VirtualProtect failed")
		}
		_, _, _ = procVirtualFree.Call(addr, 0, uintptr(memRelease))
		return nil, nil, pErr
	}

	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	buf := unsafe.Slice((*byte)(ptr), size)
	return buf, func(b []byte) error {
		if len(b) == 0 {
			return nil
		}
		p := uintptr(unsafe.Pointer(&b[0]))
		ret, _, freeErr := procVirtualFree.Call(p, 0, uintptr(memRelease))
		if ret == 0 {
			if freeErr == syscall.Errno(0) {
				return fmt.Errorf("VirtualFree failed")
			}
			return freeErr
		}
		return nil
	}, nil
}
