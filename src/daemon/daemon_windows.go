//go:build windows

package daemon

import (
	"os"
	"syscall"
)

// daemonSysProcAttr returns the process attributes used to launch the
// daemon child on Windows. Windows has no setsid equivalent; HideWindow
// keeps the child from opening a visible console window.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow: true,
	}
}

// defaultWorkDir returns the default working directory for the daemon
// child on Windows: the Windows system root, falling back to C:\Windows
// when SystemRoot is not set in the environment.
func defaultWorkDir() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return root
	}
	return `C:\Windows`
}
