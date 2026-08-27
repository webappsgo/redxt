//go:build windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

// replaceBinary replaces the running binary at currentPath with
// newBinaryPath (Windows). Windows cannot delete or rename over a running
// executable directly, so the current binary is first renamed to a
// sibling ".old" file (which Windows does allow), the new binary is
// moved into place, and the ".old" file is scheduled for deletion on
// the next reboot.
func replaceBinary(currentPath, newBinaryPath string) error {
	oldPath := currentPath + ".old"

	// Remove any stale .old file left over from a previous update.
	os.Remove(oldPath)

	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("update: rename current binary: %w", err)
	}

	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		// Best-effort restore of the original binary on failure.
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("update: move new binary into place: %w", err)
	}

	oldPathPtr, err := windows.UTF16PtrFromString(oldPath)
	if err == nil {
		windows.MoveFileEx(oldPathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}

	return nil
}

// restartSelf starts a new instance of the current process and exits
// (Windows). Windows has no exec()-style in-place process replacement,
// so a new process is spawned and this one exits once it has started.
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update: start new process: %w", err)
	}

	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
	return nil
}
