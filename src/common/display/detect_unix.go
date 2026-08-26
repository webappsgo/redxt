//go:build !windows

package display

import (
	"os"
	"runtime"
)

// detectDisplay reports the native display available on Unix-like systems,
// preferring Wayland over X11 and falling back to Quartz on macOS.
func detectDisplay() (bool, string) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true, "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return true, "x11"
	}
	if runtime.GOOS == "darwin" {
		return true, "quartz"
	}
	return false, ""
}
