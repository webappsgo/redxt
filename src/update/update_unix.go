//go:build !windows

package update

import (
	"fmt"
	"os"
	"syscall"
)

// replaceBinary replaces the running binary at currentPath with
// newBinaryPath (Unix). Unix allows renaming over a running executable —
// the old inode stays mapped in memory until the process exits, and the
// new file takes over on the next start.
func replaceBinary(currentPath, newBinaryPath string) error {
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("update: stat current binary: %w", err)
	}

	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		return fmt.Errorf("update: replace binary: %w", err)
	}

	if err := os.Chmod(currentPath, info.Mode()); err != nil {
		return fmt.Errorf("update: restore permissions: %w", err)
	}

	return nil
}

// restartSelf re-executes the current process in place (Unix).
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
