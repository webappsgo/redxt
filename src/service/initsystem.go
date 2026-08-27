package service

import "fmt"

// InitSystem identifies a host's service supervisor.
type InitSystem string

// Supported init systems, per AI.md PART 25 "Service Templates".
const (
	InitSystemd    InitSystem = "systemd"
	InitOpenRC     InitSystem = "openrc"
	InitSysVinit   InitSystem = "sysvinit"
	InitRunit      InitSystem = "runit"
	InitRcd        InitSystem = "rcd"
	InitLaunchd    InitSystem = "launchd"
	InitWindowsSCM InitSystem = "windowsscm"
)

// DetectInit identifies the init system to target on goos, using fl to
// probe well-known marker files/binaries and pl to probe PATH tools.
// OpenRC and SysVinit share /etc/init.d/{name} - only one is ever
// installed, decided by the detection order below.
func DetectInit(goos string, fl FileLookup, pl PathLookup) (InitSystem, error) {
	switch goos {
	case "linux":
		return detectLinuxInit(fl, pl)
	case "darwin":
		return InitLaunchd, nil
	case "freebsd", "openbsd", "netbsd":
		return InitRcd, nil
	case "windows":
		return InitWindowsSCM, nil
	default:
		return "", fmt.Errorf("service: unsupported GOOS %q for init-system detection", goos)
	}
}

// detectLinuxInit implements the Linux detection order: systemd, then
// OpenRC, then runit, then SysVinit - per AI.md PART 25's SysVinit
// detection rule: "the binary picks SysVinit only when /sbin/openrc-run
// is absent, systemctl is absent, and /etc/init.d/ exists with a working
// update-rc.d or chkconfig."
func detectLinuxInit(fl FileLookup, pl PathLookup) (InitSystem, error) {
	if available(pl, "systemctl") && fl.Exists("/run/systemd/system") {
		return InitSystemd, nil
	}
	if fl.Exists("/sbin/openrc-run") {
		return InitOpenRC, nil
	}
	if fl.Exists("/etc/sv") && (available(pl, "sv") || available(pl, "runsvdir")) {
		return InitRunit, nil
	}
	if !fl.Exists("/sbin/openrc-run") && !available(pl, "systemctl") && fl.Exists("/etc/init.d") &&
		(available(pl, "update-rc.d") || available(pl, "chkconfig")) {
		return InitSysVinit, nil
	}
	return "", fmt.Errorf("service: no supported init system detected " +
		"(checked systemd, OpenRC, runit, SysVinit)")
}

// UnitPath returns the absolute install path(s) for name under init, or
// nil for init systems (runit) that install multiple files - see
// RunitPaths for that case. root is prepended for test isolation; pass ""
// in production so the path is absolute on the real filesystem.
func UnitPath(init InitSystem, root, name, plistName string) string {
	switch init {
	case InitSystemd:
		return root + "/etc/systemd/system/" + name + ".service"
	case InitOpenRC, InitSysVinit:
		return root + "/etc/init.d/" + name
	case InitRcd:
		return root + "/usr/local/etc/rc.d/" + name
	case InitLaunchd:
		return root + "/Library/LaunchDaemons/" + plistName + ".plist"
	default:
		return ""
	}
}

// RunitPaths returns the run script and log/run script paths for the
// runit service directory /etc/sv/{name}/.
func RunitPaths(root, name string) (runScript, logRunScript string) {
	dir := root + "/etc/sv/" + name
	return dir + "/run", dir + "/log/run"
}
