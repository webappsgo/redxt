//go:build unix

package service

// Privilege drop for Unix-like targets (Linux, macOS, BSD). AI.md
// PART 24/25: the server starts elevated only long enough to bind
// privileged ports, then drops to the dedicated {internal_name} system
// user/group for the rest of its life.

import (
	"fmt"
	"os"
	"syscall"
)

// DropPrivileges drops the current process's supplementary groups, GID,
// and UID to gid/uid, in that order - group must be dropped before user,
// since once the UID is dropped the process typically no longer has
// permission to change its GID. It verifies the drop actually took effect
// and returns an error (rather than continuing to run privileged) if it
// did not.
func DropPrivileges(uid, gid int) error {
	if err := syscall.Setgroups([]int{}); err != nil {
		return fmt.Errorf("service: clearing supplementary groups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("service: dropping to gid %d: %w", gid, err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("service: dropping to uid %d: %w", uid, err)
	}

	if got := os.Getuid(); got != uid {
		return fmt.Errorf("service: privilege drop did not take effect: uid is %d, want %d", got, uid)
	}
	if got := os.Getgid(); got != gid {
		return fmt.Errorf("service: privilege drop did not take effect: gid is %d, want %d", got, gid)
	}
	if os.Geteuid() == 0 {
		return fmt.Errorf("service: privilege drop did not take effect: still effectively root")
	}
	return nil
}
