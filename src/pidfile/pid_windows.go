//go:build windows
// +build windows

package pidfile

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The stdlib golang.org/x/sys/windows package is not a dependency of this
// module, so process identity is checked via the tasklist command instead
// of the Windows API - this avoids CGO and any extra module dependency.

// isProcessRunningPlatform checks if a process with the given PID exists (Windows).
func isProcessRunningPlatform(pid int) bool {
	if _, err := os.FindProcess(pid); err != nil {
		return false
	}
	image, ok := tasklistImageName(pid)
	return ok && image != ""
}

// isOurProcessPlatform verifies the process is actually our binary (Windows).
func isOurProcessPlatform(pid int) bool {
	image, ok := tasklistImageName(pid)
	if !ok {
		return false
	}
	// Exact match (case-insensitive) - substring matching would also match redxt-cli.exe
	return strings.EqualFold(image, BinaryName+".exe") || strings.EqualFold(image, BinaryName)
}

// tasklistImageName runs tasklist filtered to the given PID and returns the
// image name from the first CSV field, or ok=false if the PID was not found.
func tasklistImageName(pid int) (string, bool) {
	filter := fmt.Sprintf("PID eq %d", pid)
	cmd := exec.Command("tasklist", "/FI", filter, "/NH", "/FO", "CSV")
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return "", false
	}
	// Only the first line of output is relevant - a matching PID produces
	// exactly one CSV row
	line = strings.SplitN(line, "\n", 2)[0]

	fields := strings.Split(line, ",")
	if len(fields) == 0 {
		return "", false
	}
	image := strings.Trim(strings.TrimSpace(fields[0]), "\"")
	if image == "" {
		return "", false
	}

	// Confirm the row actually names the requested PID, not an unrelated
	// "INFO: No tasks..." message
	if len(fields) > 1 {
		pidField := strings.Trim(strings.TrimSpace(fields[1]), "\"")
		if reportedPID, err := strconv.Atoi(pidField); err != nil || reportedPID != pid {
			return "", false
		}
	}

	return image, true
}
