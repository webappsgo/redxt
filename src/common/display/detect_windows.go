//go:build windows

package display

import (
	"os"
	"strings"
)

// detectDisplay reports the native display available on Windows; services run
// in session 0 with no interactive desktop, and RDP sessions are remote.
func detectDisplay() (bool, string) {
	session := os.Getenv("SESSIONNAME")
	if session == "" {
		return false, "session0"
	}
	if strings.HasPrefix(session, "RDP") {
		return true, "rdp"
	}
	return true, "win32"
}
