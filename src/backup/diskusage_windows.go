//go:build windows

package backup

import (
	"syscall"
	"unsafe"
)

// diskUsage returns the total and free byte capacity of the volume holding
// path, via GetDiskFreeSpaceExW loaded from kernel32.dll through the
// stdlib syscall package — no third-party dependency required.
func diskUsage(path string) (totalBytes, freeBytes uint64, err error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")

	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}

	var freeAvail, total, totalFree uint64
	ret, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if ret == 0 {
		return 0, 0, callErr
	}
	return total, freeAvail, nil
}
