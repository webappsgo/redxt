// Package daemon implements Unix-style daemonization via a re-exec approach.
//
// The parent process re-execs itself with an environment marker set. The
// child process detects the marker, detaches from the controlling terminal
// (setsid on Unix), and continues normal startup. The parent prints the
// child PID and exits. Windows does not support this approach; see
// daemon_windows.go for the platform-specific stub.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// daemonChildEnvVar is the environment variable used to mark the re-exec'd
// child process so it does not attempt to daemonize again.
const daemonChildEnvVar = "_DAEMON_CHILD"

// Options configures a call to Daemonize.
type Options struct {
	// Args are the arguments passed to the re-exec'd child, excluding the
	// executable name. When nil, os.Args[1:] is used.
	Args []string

	// Env is the environment passed to the child. When nil, os.Environ()
	// is used. The daemon child marker is appended automatically.
	Env []string

	// WorkDir is the working directory of the child process. When empty,
	// the platform default working directory is used.
	WorkDir string

	// LogFile, when non-empty, receives the child's stdout and stderr.
	// When empty, both are redirected to os.DevNull.
	LogFile string
}

// IsChild reports whether the current process is the re-exec'd daemon
// child, detected via the environment marker set by Daemonize.
func IsChild() bool {
	return os.Getenv(daemonChildEnvVar) != ""
}

// FilterDaemonFlag returns a copy of args with every form of the daemon
// flag removed: --daemon, -daemon, --daemon=true, --daemon=false, and the
// single-dash equivalents. It does not remove unrelated arguments that
// merely contain the substring "daemon", and it does not consume the
// argument following the flag, since the flag is boolean.
func FilterDaemonFlag(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if isDaemonFlag(arg) {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

// isDaemonFlag reports whether arg is exactly the daemon flag, in either
// its bare form or its --daemon=value / -daemon=value form.
func isDaemonFlag(arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	switch name {
	case "--daemon", "-daemon":
		return true
	default:
		return false
	}
}

// Daemonize re-execs the current executable, detached from the controlling
// terminal, and returns the child's PID. It must not be called from within
// the daemon child itself; callers should check IsChild() first. Calling it
// from the child returns an error.
func Daemonize(opts Options) (pid int, err error) {
	if IsChild() {
		return 0, errors.New("daemon: already running as the daemon child")
	}

	execPath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("daemon: resolving executable path: %w", err)
	}

	args := opts.Args
	if args == nil {
		args = os.Args[1:]
	}
	args = FilterDaemonFlag(args)

	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	env = append(append([]string{}, env...), daemonChildEnvVar+"=1")

	workDir := opts.WorkDir
	if workDir == "" {
		workDir = defaultWorkDir()
	}

	cmd := exec.Command(execPath, args...)
	cmd.Env = env
	cmd.Dir = workDir
	cmd.SysProcAttr = daemonSysProcAttr()

	var closers []*os.File
	defer func() {
		for _, f := range closers {
			f.Close()
		}
	}()

	if opts.LogFile != "" {
		logFile, openErr := os.OpenFile(opts.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr != nil {
			return 0, fmt.Errorf("daemon: opening log file: %w", openErr)
		}
		closers = append(closers, logFile)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else {
		devNull, openErr := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if openErr != nil {
			return 0, fmt.Errorf("daemon: opening %s: %w", os.DevNull, openErr)
		}
		closers = append(closers, devNull)
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}

	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("daemon: starting child: %w", err)
	}

	return cmd.Process.Pid, nil
}
