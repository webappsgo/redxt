//go:build !windows
// +build !windows

package pidfile

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// isProcessRunningPlatform checks if a process with the given PID exists (Unix).
func isProcessRunningPlatform(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds - need to send signal 0
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means the process exists but belongs to another user - it IS running
	return errors.Is(err, syscall.EPERM)
}

// isOurProcessPlatform verifies the process is actually our binary (Unix).
func isOurProcessPlatform(pid int) bool {
	// Read /proc/{pid}/exe symlink (Linux)
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		// On macOS/BSD, use ps command
		return isOurProcessBSD(pid)
	}
	// Exact match - substring matching would also match redxt-cli
	return filepath.Base(exePath) == BinaryName
}

// isOurProcessBSD checks process identity on macOS/BSD via ps.
func isOurProcessBSD(pid int) bool {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// Exact match - substring matching would also match redxt-cli
	return strings.TrimSpace(string(output)) == BinaryName
}
