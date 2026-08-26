// Package paths resolves OS-specific filesystem locations for redxt.
//
// Resolution follows AI.md PART 4 (OS-Specific Paths): privileged vs
// non-privileged user, per-OS base directories, and the Docker-only
// /config and /data convention. All paths are namespaced under
// {internal_org}/{internal_name} (webappsgo/redxt) except the Docker
// container paths, which are namespaced under {project_name} only.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// internalOrg and internalName are frozen per IDEA.md Project variables.
const (
	internalOrg  = "webappsgo"
	internalName = "redxt"
	projectName  = "redxt"
)

// Paths holds every resolved filesystem location the server needs.
type Paths struct {
	Binary     string
	Config     string
	ConfigFile string
	Data       string
	Cache      string
	Logs       string
	LogFile    string
	Backup     string
	PIDFile    string
	SSL        string
	Security   string
	DB         string
}

// IsDockerContainer reports whether the process is running inside a
// container built from docker/Dockerfile (detected via /.dockerenv or
// the CONTAINER env var set by the entrypoint script).
func IsDockerContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return os.Getenv("CONTAINER") != ""
}

// IsPrivileged reports whether the current process has elevated
// (root/Administrator) privileges.
func IsPrivileged() bool {
	if runtime.GOOS == "windows" {
		// Windows privilege detection is handled by the service
		// installer (PART 24); default to non-privileged here.
		return false
	}
	return os.Geteuid() == 0
}

// Resolve returns the full set of resolved paths for the current
// platform and privilege level, honoring CONFIG_DIR/DATA_DIR/LOG_DIR/
// DATABASE_DIR/BACKUP_DIR init-only environment variable overrides.
func Resolve() Paths {
	var p Paths

	switch {
	case IsDockerContainer():
		p = dockerPaths()
	case runtime.GOOS == "windows":
		p = windowsPaths(IsPrivileged())
	case runtime.GOOS == "darwin":
		p = darwinPaths(IsPrivileged())
	case isBSD():
		p = bsdPaths(IsPrivileged())
	default:
		p = linuxPaths(IsPrivileged())
	}

	applyInitOverrides(&p)
	return p
}

func isBSD() bool {
	switch runtime.GOOS {
	case "freebsd", "openbsd", "netbsd":
		return true
	default:
		return false
	}
}

// applyInitOverrides applies the init-only environment variables
// (CONFIG_DIR, DATA_DIR, CACHE_DIR, LOG_DIR, PID_FILE, DATABASE_DIR,
// BACKUP_DIR) documented
// in AI.md PART 5. These are consulted only on first run by the config
// package; paths.Resolve exposes the override so config can decide.
func applyInitOverrides(p *Paths) {
	if v := os.Getenv("CONFIG_DIR"); v != "" {
		p.Config = v
		p.ConfigFile = filepath.Join(v, "server.yml")
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		p.Data = v
	}
	if v := os.Getenv("LOG_DIR"); v != "" {
		p.Logs = v
		p.LogFile = filepath.Join(v, "server.log")
	}
	if v := os.Getenv("CACHE_DIR"); v != "" {
		p.Cache = v
	}
	if v := os.Getenv("PID_FILE"); v != "" {
		p.PIDFile = v
	}
	if v := os.Getenv("DATABASE_DIR"); v != "" {
		p.DB = v
	}
	if v := os.Getenv("BACKUP_DIR"); v != "" {
		p.Backup = v
	}
}

func linuxPaths(privileged bool) Paths {
	if privileged {
		return Paths{
			Binary:     "/usr/local/bin/" + projectName,
			Config:     "/etc/" + internalOrg + "/" + internalName + "/",
			ConfigFile: "/etc/" + internalOrg + "/" + internalName + "/server.yml",
			Data:       "/var/lib/" + internalOrg + "/" + internalName + "/",
			Cache:      "/var/cache/" + internalOrg + "/" + internalName + "/",
			Logs:       "/var/log/" + internalOrg + "/" + internalName + "/",
			LogFile:    "/var/log/" + internalOrg + "/" + internalName + "/server.log",
			Backup:     "/mnt/Backups/" + internalOrg + "/" + internalName + "/",
			PIDFile:    "/var/run/" + internalOrg + "/" + internalName + ".pid",
			SSL:        "/etc/" + internalOrg + "/" + internalName + "/ssl/",
			Security:   "/var/lib/" + internalOrg + "/" + internalName + "/security/",
			DB:         "/var/lib/" + internalOrg + "/" + internalName + "/db/",
		}
	}
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".config", internalOrg, internalName)
	data := filepath.Join(home, ".local", "share", internalOrg, internalName)
	return Paths{
		Binary:     filepath.Join(home, ".local", "bin", projectName),
		Config:     base + "/",
		ConfigFile: filepath.Join(base, "server.yml"),
		Data:       data + "/",
		Cache:      filepath.Join(home, ".cache", internalOrg, internalName) + "/",
		Logs:       filepath.Join(home, ".local", "log", internalOrg, internalName) + "/",
		LogFile:    filepath.Join(home, ".local", "log", internalOrg, internalName, "server.log"),
		Backup:     filepath.Join(home, ".local", "share", "Backups", internalOrg, internalName) + "/",
		PIDFile:    filepath.Join(data, internalName+".pid"),
		SSL:        filepath.Join(base, "ssl") + "/",
		Security:   filepath.Join(data, "security") + "/",
		DB:         filepath.Join(data, "db") + "/",
	}
}

func darwinPaths(privileged bool) Paths {
	if privileged {
		base := "/Library/Application Support/" + internalOrg + "/" + internalName
		return Paths{
			Binary:     "/usr/local/bin/" + projectName,
			Config:     base + "/",
			ConfigFile: base + "/server.yml",
			Data:       base + "/data/",
			Cache:      "/Library/Caches/" + internalOrg + "/" + internalName + "/",
			Logs:       "/Library/Logs/" + internalOrg + "/" + internalName + "/",
			LogFile:    "/Library/Logs/" + internalOrg + "/" + internalName + "/server.log",
			Backup:     "/Library/Backups/" + internalOrg + "/" + internalName + "/",
			PIDFile:    "/var/run/" + internalOrg + "/" + internalName + ".pid",
			SSL:        base + "/ssl/",
			Security:   base + "/data/security/",
			DB:         base + "/db/",
		}
	}
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "Library", "Application Support", internalOrg, internalName)
	return Paths{
		Binary:     filepath.Join(home, "bin", projectName),
		Config:     base + "/",
		ConfigFile: filepath.Join(base, "server.yml"),
		Data:       base + "/",
		Cache:      filepath.Join(home, "Library", "Caches", internalOrg, internalName) + "/",
		Logs:       filepath.Join(home, "Library", "Logs", internalOrg, internalName) + "/",
		LogFile:    filepath.Join(home, "Library", "Logs", internalOrg, internalName, "server.log"),
		Backup:     filepath.Join(home, "Library", "Backups", internalOrg, internalName) + "/",
		PIDFile:    filepath.Join(base, internalName+".pid"),
		SSL:        filepath.Join(base, "ssl") + "/",
		Security:   filepath.Join(base, "data", "security") + "/",
		DB:         filepath.Join(base, "db") + "/",
	}
}

func bsdPaths(privileged bool) Paths {
	if privileged {
		base := "/usr/local/etc/" + internalOrg + "/" + internalName
		return Paths{
			Binary:     "/usr/local/bin/" + projectName,
			Config:     base + "/",
			ConfigFile: base + "/server.yml",
			Data:       "/var/db/" + internalOrg + "/" + internalName + "/",
			Cache:      "/var/cache/" + internalOrg + "/" + internalName + "/",
			Logs:       "/var/log/" + internalOrg + "/" + internalName + "/",
			LogFile:    "/var/log/" + internalOrg + "/" + internalName + "/server.log",
			Backup:     "/var/backups/" + internalOrg + "/" + internalName + "/",
			PIDFile:    "/var/run/" + internalOrg + "/" + internalName + ".pid",
			SSL:        base + "/ssl/",
			Security:   "/var/db/" + internalOrg + "/" + internalName + "/security/",
			DB:         "/var/db/" + internalOrg + "/" + internalName + "/db/",
		}
	}
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".config", internalOrg, internalName)
	data := filepath.Join(home, ".local", "share", internalOrg, internalName)
	return Paths{
		Binary:     filepath.Join(home, ".local", "bin", projectName),
		Config:     base + "/",
		ConfigFile: filepath.Join(base, "server.yml"),
		Data:       data + "/",
		Cache:      filepath.Join(home, ".cache", internalOrg, internalName) + "/",
		Logs:       filepath.Join(home, ".local", "log", internalOrg, internalName) + "/",
		LogFile:    filepath.Join(home, ".local", "log", internalOrg, internalName, "server.log"),
		Backup:     filepath.Join(home, ".local", "share", "Backups", internalOrg, internalName) + "/",
		PIDFile:    filepath.Join(data, internalName+".pid"),
		SSL:        filepath.Join(base, "ssl") + "/",
		Security:   filepath.Join(data, "security") + "/",
		DB:         filepath.Join(data, "db") + "/",
	}
}

func windowsPaths(privileged bool) Paths {
	if privileged {
		base := `C:\ProgramData\` + internalOrg + `\` + internalName
		return Paths{
			Binary:     `C:\Program Files\` + internalOrg + `\` + internalName + `\` + projectName + ".exe",
			Config:     base + `\`,
			ConfigFile: base + `\server.yml`,
			Data:       base + `\data\`,
			Cache:      base + `\cache\`,
			Logs:       base + `\logs\`,
			LogFile:    base + `\logs\server.log`,
			Backup:     `C:\ProgramData\Backups\` + internalOrg + `\` + internalName + `\`,
			SSL:        base + `\ssl\`,
			Security:   base + `\data\security\`,
			DB:         base + `\db\`,
		}
	}
	appData := os.Getenv("AppData")
	localAppData := os.Getenv("LocalAppData")
	base := filepath.Join(appData, internalOrg, internalName)
	localBase := filepath.Join(localAppData, internalOrg, internalName)
	return Paths{
		Binary:     filepath.Join(localAppData, internalOrg, internalName, projectName+".exe"),
		Config:     base + `\`,
		ConfigFile: filepath.Join(base, "server.yml"),
		Data:       localBase + `\`,
		Cache:      filepath.Join(localBase, "cache") + `\`,
		Logs:       filepath.Join(localBase, "logs") + `\`,
		LogFile:    filepath.Join(localBase, "logs", "server.log"),
		Backup:     filepath.Join(localAppData, "Backups", internalOrg, internalName) + `\`,
		SSL:        filepath.Join(base, "ssl") + `\`,
		Security:   filepath.Join(localBase, "security") + `\`,
		DB:         filepath.Join(localBase, "db") + `\`,
	}
}

// dockerPaths returns the simplified /config and /data layout used ONLY
// inside the project's own Docker image (see AI.md PART 4, Docker/Container).
func dockerPaths() Paths {
	return Paths{
		Binary:     "/usr/local/bin/" + projectName,
		Config:     "/config/" + projectName + "/",
		ConfigFile: "/config/" + projectName + "/server.yml",
		Data:       "/data/" + projectName + "/",
		Cache:      "/data/" + projectName + "/cache/",
		Logs:       "/data/log/" + projectName + "/",
		LogFile:    "/data/log/" + projectName + "/server.log",
		Backup:     "/data/backups/" + projectName + "/",
		SSL:        "/config/" + projectName + "/ssl/",
		Security:   "/data/" + projectName + "/security/",
		DB:         "/data/db/sqlite/",
	}
}
