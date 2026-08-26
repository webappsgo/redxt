//go:build !windows
// +build !windows

package daemon

import "syscall"

// daemonSysProcAttr returns the process attributes used to detach the
// daemon child from the controlling terminal by creating a new session.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}

// defaultWorkDir returns the default working directory for the daemon
// child on Unix, which is the filesystem root.
func defaultWorkDir() string {
	return "/"
}
