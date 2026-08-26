//go:build !windows

package terminal

import (
	"os"
	"syscall"
	"unsafe"
)

// winsize mirrors the kernel struct winsize filled in by the TIOCGWINSZ ioctl.
type winsize struct {
	Rows    uint16
	Cols    uint16
	XPixels uint16
	YPixels uint16
}

// querySize asks the controlling terminal for its geometry via TIOCGWINSZ.
func querySize() (int, int, bool) {
	var ws winsize
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, 0, false
	}
	if ws.Cols == 0 && ws.Rows == 0 {
		return 0, 0, false
	}
	return int(ws.Cols), int(ws.Rows), true
}
