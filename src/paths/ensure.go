package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Directory and file permissions. Root-owned system directories are
// world-readable; user-owned directories are private. See AI.md PART 8
// "Directory Validation Rules".
const (
	// DirModeSystem is the directory mode used when started elevated.
	DirModeSystem os.FileMode = 0o755
	// DirModeUser is the directory mode used when started unprivileged.
	DirModeUser os.FileMode = 0o700
	// DirModeSensitive is the mode for directories holding key material.
	DirModeSensitive os.FileMode = 0o700
	// PIDFileModeSystem is the mode of the PID file when started
	// elevated, where the file lives in a shared runtime directory and
	// service tooling has to read it.
	PIDFileModeSystem os.FileMode = 0o644
	// PIDFileModeUser is the mode of the PID file when started
	// unprivileged, where nothing outside the account may read it.
	PIDFileModeUser os.FileMode = 0o600
)

// startedElevated is captured ONCE at process start, before any
// privilege drop, and never re-evaluated. After the startup sequence
// drops privileges the effective UID changes, but the directory mode
// (system versus user) must not.
var startedElevated = IsPrivileged()

// StartedElevated reports whether the process began life with elevated
// privileges. Directory layout decisions must use this, never a live
// IsPrivileged call, because the answer changes after the privilege
// drop in startup step 8h.
func StartedElevated() bool {
	return startedElevated
}

// Overrides holds the directory values supplied on the command line.
// They take priority over environment variables and over the OS
// defaults. An empty field means "not supplied".
type Overrides struct {
	Config  string
	Data    string
	Cache   string
	Logs    string
	Backup  string
	PIDFile string
}

// ResolveWith returns the resolved paths with command-line overrides
// applied on top. Priority is CLI flag, then environment variable, then
// the OS default, which is the order AI.md PART 8 prescribes.
func ResolveWith(o Overrides) Paths {
	p := Resolve()
	if o.Config != "" {
		p.Config = o.Config
		p.ConfigFile = filepath.Join(o.Config, "server.yml")
		p.SSL = filepath.Join(o.Config, "ssl")
	}
	if o.Data != "" {
		p.Data = o.Data
		p.Security = filepath.Join(o.Data, "security")
		if os.Getenv("DATABASE_DIR") == "" {
			p.DB = filepath.Join(o.Data, "db")
		}
	}
	if o.Cache != "" {
		p.Cache = o.Cache
	}
	if o.Logs != "" {
		p.Logs = o.Logs
		p.LogFile = filepath.Join(o.Logs, "server.log")
	}
	if o.PIDFile != "" {
		p.PIDFile = o.PIDFile
	}
	p.Backup = ResolveBackupDir(o.Backup, p.Data)
	return p
}

// ResolveBackupDir picks the backup directory. The system backup
// location wins whenever it is writable. When it is not, an elevated
// process falls back inside the data directory and never to a
// $HOME-derived path: a service account's HOME points at the data
// directory, so a $HOME fallback would nest user-style directories
// inside it.
func ResolveBackupDir(flagValue, dataDir string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("BACKUP_DIR"); v != "" {
		return v
	}
	if sys := SystemBackupDir(); isWritable(sys) {
		return sys
	}
	if startedElevated {
		return filepath.Join(dataDir, "backup")
	}
	return UserBackupDir()
}

// SystemBackupDir returns the system-level backup directory for this
// platform.
func SystemBackupDir() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join("/Library/Backups", internalOrg, internalName)
	case "windows":
		return filepath.Join(os.Getenv("ProgramData"), "Backups", internalOrg, internalName)
	case "freebsd", "openbsd", "netbsd":
		return filepath.Join("/var/backups", internalOrg, internalName)
	default:
		return filepath.Join("/mnt/Backups", internalOrg, internalName)
	}
}

// UserBackupDir returns the per-user backup directory. It must never be
// called once the process has dropped privileges from an elevated
// start, because $HOME then belongs to the service account.
func UserBackupDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Backups", internalOrg, internalName)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Backups", internalOrg, internalName)
	default:
		return filepath.Join(home, ".local", "share", "Backups", internalOrg, internalName)
	}
}

// isWritable reports whether a directory can be created at path, by
// probing its parent with a uniquely named temporary file.
func isWritable(path string) bool {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return false
	}
	probe := filepath.Join(parent, ".write_test_"+strconv.FormatInt(time.Now().UnixNano(), 36))
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}

// DirMode returns the directory mode for the locked-in privilege level.
func DirMode() os.FileMode {
	if startedElevated {
		return DirModeSystem
	}
	return DirModeUser
}

// PIDFileMode returns the PID file mode for the locked-in privilege
// level, per the AI.md PART 8 "Directory Validation Rules" table.
func PIDFileMode() os.FileMode {
	if startedElevated {
		return PIDFileModeSystem
	}
	return PIDFileModeUser
}

// EnsureDir creates a directory, including any missing parents, and
// verifies that it is writable. A directory that already exists keeps
// its current mode; only newly created directories are given perm.
func EnsureDir(path string, perm os.FileMode) error {
	if path == "" {
		return fmt.Errorf("paths: refusing to create a directory with an empty path")
	}
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("paths: creating %s: %w", path, err)
	}
	probe := filepath.Join(path, ".write-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return fmt.Errorf("paths: %s is not writable: %w", path, err)
	}
	os.Remove(probe)
	return nil
}

// EnsurePIDFile creates the directory that will hold the PID file.
func EnsurePIDFile(path string, perm os.FileMode) error {
	if path == "" {
		return fmt.Errorf("paths: refusing to create a PID directory with an empty path")
	}
	return EnsureDir(filepath.Dir(path), perm)
}

// EnsureAll creates every directory the server needs. General
// directories follow the locked-in privilege level; directories that
// hold key material or private data are always 0700.
func EnsureAll(p Paths) error {
	perm := DirMode()
	general := []string{p.Config, p.Data, p.Cache, p.Logs, p.Backup, p.DB}
	for _, dir := range general {
		if dir == "" {
			continue
		}
		if err := EnsureDir(dir, perm); err != nil {
			return err
		}
	}
	sensitive := []string{p.SSL, p.Security}
	for _, dir := range sensitive {
		if dir == "" {
			continue
		}
		if err := EnsureDir(dir, DirModeSensitive); err != nil {
			return err
		}
	}
	return nil
}
