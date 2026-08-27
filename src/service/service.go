// Package service implements AI.md PART 24 (Privilege Escalation & Service)
// and PART 25 (Service Support): detecting how the current user can
// escalate privileges, creating the dedicated {internal_name} system user
// and group, detecting the host init system, rendering that init system's
// unit file, and driving install/uninstall/disable/start/stop/restart/
// reload plus privilege drop after a privileged port is bound.
//
// IDEA.md does not justify redxt running permanently as root/Administrator,
// so this package always implements the default behavior: a dedicated
// non-root system user/group is created and the process drops to it after
// binding privileged ports.
package service

import (
	"os/exec"
)

// Logger is the subset of *logging.Logger this package needs. A narrow
// local interface (matching src/scheduler's convention) avoids importing
// src/logging directly.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Runner executes external commands. Production code uses execRunner;
// tests inject a fake so no user/group is ever created, no file is ever
// written under /etc, and no init-system command is ever actually invoked.
type Runner interface {
	// Run executes name with args, discarding stdout/stderr on success and
	// returning combined output on failure.
	Run(name string, args ...string) error
	// Output executes name with args and returns its trimmed stdout.
	Output(name string, args ...string) ([]byte, error)
}

// execRunner is the production Runner backed by os/exec.
type execRunner struct{}

// NewExecRunner returns the production Runner that actually invokes
// external commands via os/exec.
func NewExecRunner() Runner { return execRunner{} }

func (execRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return &RunError{Name: name, Args: args, Output: out, Err: err}
	}
	return nil
}

func (execRunner) Output(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, &RunError{Name: name, Args: args, Output: out, Err: err}
	}
	return out, nil
}

// RunError wraps a failed external command with enough context for callers
// to log or display it without re-running the command.
type RunError struct {
	Name   string
	Args   []string
	Output []byte
	Err    error
}

func (e *RunError) Error() string {
	return e.Name + ": " + e.Err.Error()
}

func (e *RunError) Unwrap() error { return e.Err }

// Context carries every value a rendered unit/service file or user-creation
// command needs. Callers (src/cli, src/startup) build one from resolved
// paths.Paths, config, and the frozen {internal_org}/{internal_name}
// identifiers; this package never hardcodes them so it stays reusable.
type Context struct {
	// ProjectName is the public binary name ({project_name}), e.g. "redxt".
	ProjectName string
	// ProjectOrg is {project_org}, used in Documentation= URLs.
	ProjectOrg string
	// InternalOrg is the frozen {internal_org} path-namespace identifier.
	InternalOrg string
	// InternalName is the frozen {internal_name} identifier - also the
	// dedicated system user/group name and the init-system service name.
	InternalName string
	// AppName is the human-facing description used in OpenRC/SysVinit
	// comments. Defaults to ProjectName when empty.
	AppName string
	// BinaryPath is the absolute path the unit file execs.
	BinaryPath string
	// ConfigDir, DataDir, CacheDir, LogDir, BackupDir are the resolved
	// privileged-mode directories (paths.Paths fields for a privileged run).
	ConfigDir string
	DataDir   string
	CacheDir  string
	LogDir    string
	BackupDir string
	// PIDFile is the resolved privileged-mode PID file path.
	PIDFile string
}

// appName returns c.AppName, falling back to c.ProjectName when unset.
func (c Context) appName() string {
	if c.AppName != "" {
		return c.AppName
	}
	return c.ProjectName
}

// plistName returns the reverse-DNS style launchd label derived from
// InternalOrg/InternalName, e.g. "org.webappsgo.redxt". AI.md PART 24/25
// reference {plist_name} without dictating its exact format; reverse-DNS
// is the documented launchd convention.
func (c Context) plistName() string {
	return "org." + c.InternalOrg + "." + c.InternalName
}

// Manager drives service install/uninstall/status for the detected
// platform and init system. All filesystem writes are rooted under Root
// (defaults to "/" in production; tests set it to t.TempDir()) so tests
// never touch the real host.
type Manager struct {
	Ctx    Context
	Runner Runner
	Log    Logger
	GOOS   string
	Root   string
	IDLU   IDLookup
	PathLU PathLookup
	FileLU FileLookup
	// Elevated reports whether the current process is already
	// root/Administrator (paths.IsPrivileged()).
	Elevated bool
	Confirm  func(prompt string) bool
}
