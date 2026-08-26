// Package pidfile implements PID file creation, staleness detection, and
// removal, with container-aware skipping and cross-platform process checks.
package pidfile

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/webappsgo/redxt/src/paths"
)

// BinaryName is the project binary name used for process-identity checks.
// An exact base-name match is required - substring matching would also
// match redxt-cli and redxt-agent.
const BinaryName = "redxt"

// isProcessRunning reports whether a process with the given PID exists.
// It is a package-level variable assigned from the platform-specific
// implementation so tests can substitute it.
var isProcessRunning = isProcessRunningPlatform

// isOurProcess reports whether the process with the given PID is our binary.
// It is a package-level variable assigned from the platform-specific
// implementation so tests can substitute it.
var isOurProcess = isOurProcessPlatform

// isContainerFunc detects whether the current process is running inside a
// container. It is a package-level variable so tests can force both branches.
var isContainerFunc = detectContainer

// IsContainer reports whether the process is running inside a container.
func IsContainer() bool {
	return isContainerFunc()
}

// containerMarkerFiles are the filesystem markers container runtimes drop
// into the image root. It is a package-level variable so tests can isolate
// the environment-variable branch: the build toolchain itself runs inside a
// container, where these markers are genuinely present.
var containerMarkerFiles = []string{"/.dockerenv", "/run/.containerenv"}

// detectContainer performs the actual container detection.
func detectContainer() bool {
	for _, marker := range containerMarkerFiles {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	if os.Getenv("CONTAINER") != "" {
		return true
	}
	if os.Getenv("container") != "" {
		return true
	}
	return false
}

// Check checks if a PID file exists and if the process it names is still
// running and is our binary. It removes stale or corrupt PID files.
// Returns: (isRunning bool, pid int, err error)
func Check(pidPath string) (bool, int, error) {
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("reading pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(pidPath)
		return false, 0, nil
	}

	if !isProcessRunning(pid) {
		os.Remove(pidPath)
		return false, 0, nil
	}

	if !isOurProcess(pid) {
		os.Remove(pidPath)
		return false, 0, nil
	}

	return true, pid, nil
}

// Write writes the current process PID to the given path.
// Containers are skipped entirely - the runtime supervises the process, and
// a namespace-local PID in a PID file is wrong when read across namespaces.
func Write(pidPath string) error {
	if IsContainer() {
		return nil
	}

	running, existingPID, err := Check(pidPath)
	if err != nil {
		return err
	}
	if running {
		return fmt.Errorf("already running (pid %d)", existingPID)
	}

	// The mode follows the locked-in privilege level rather than the
	// live EUID, so a PID file written after the startup privilege drop
	// still gets the system mode the service tooling expects.
	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), paths.PIDFileMode())
}

// Remove removes the PID file on shutdown. It is a no-op inside a
// container, and returns nil if the file is already gone.
func Remove(pidPath string) error {
	if IsContainer() {
		return nil
	}

	err := os.Remove(pidPath)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
