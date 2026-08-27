package service

import (
	"fmt"
	"os/user"
	"strconv"
)

// reservedIDs are UIDs/GIDs used by well-known services across distros.
// NEVER use these even if they appear available on the current system.
// Verbatim per AI.md PART 24 "Go Implementation".
var reservedIDs = map[int]bool{
	// nobody
	65534: true,
	// systemd-*, docker
	999: true, 998: true, 997: true, 996: true, 995: true,
	// systemd-*, kvm
	994: true, 993: true, 992: true, 991: true, 990: true,
	// sgx, pipewire, colord
	989: true, 988: true, 987: true, 986: true, 985: true,
	// avahi, rtkit, saned
	984: true, 983: true, 982: true, 981: true, 980: true,
	// Database and common services (101-110, 170-179)
	101: true, 102: true, 103: true, 104: true, 105: true,
	106: true, 107: true, 108: true, 109: true, 110: true,
	170: true, 171: true, 172: true, 173: true, 174: true,
	175: true, 176: true, 177: true, 178: true, 179: true,
}

// IDLookup resolves whether a UID or GID is already taken. Production code
// uses osIDLookup (os/user); tests inject a fake so no real /etc/passwd or
// /etc/group lookup ever happens.
type IDLookup interface {
	LookupUID(id int) (taken bool)
	LookupGID(id int) (taken bool)
}

// osIDLookup is the production IDLookup backed by os/user.
type osIDLookup struct{}

// NewOSIDLookup returns the production IDLookup backed by os/user.
func NewOSIDLookup() IDLookup { return osIDLookup{} }

func (osIDLookup) LookupUID(id int) bool {
	_, err := user.LookupId(strconv.Itoa(id))
	return err == nil
}

func (osIDLookup) LookupGID(id int) bool {
	_, err := user.LookupGroupId(strconv.Itoa(id))
	return err == nil
}

// LinuxIDRangeTop and LinuxIDRangeBottom bound the Linux/BSD safe system ID
// range (AI.md PART 24 "Safe range recommendation: 200-899").
const (
	LinuxIDRangeTop    = 899
	LinuxIDRangeBottom = 200
)

// DarwinIDRangeTop and DarwinIDRangeBottom bound the macOS safe system ID
// range (AI.md PART 24 "Safe range recommendation: 200-399").
const (
	DarwinIDRangeTop    = 399
	DarwinIDRangeBottom = 200
)

// FindAvailableSystemID finds an unused ID in [bottom, top], scanning from
// top down to bottom, skipping reservedIDs and any ID where either the UID
// or the GID already resolves to an existing account. AI.md PART 24
// "UID/GID Selection Logic" / "Go Implementation".
func FindAvailableSystemID(lookup IDLookup, top, bottom int) (int, error) {
	for id := top; id >= bottom; id-- {
		if reservedIDs[id] {
			continue
		}
		if lookup.LookupUID(id) {
			continue
		}
		if lookup.LookupGID(id) {
			continue
		}
		return id, nil
	}
	return 0, fmt.Errorf("service: no available UID/GID in safe range %d-%d", bottom, top)
}

// FindAvailableLinuxSystemID finds an available ID in the Linux/BSD safe
// range (200-899).
func FindAvailableLinuxSystemID(lookup IDLookup) (int, error) {
	return FindAvailableSystemID(lookup, LinuxIDRangeTop, LinuxIDRangeBottom)
}

// FindAvailableMacOSSystemID finds an available ID in the macOS safe range
// (200-399).
func FindAvailableMacOSSystemID(lookup IDLookup) (int, error) {
	return FindAvailableSystemID(lookup, DarwinIDRangeTop, DarwinIDRangeBottom)
}

// FileLookup reports whether a path exists on disk. Production code uses
// osFileLookup (os.Stat); tests inject a fake in-memory set.
type FileLookup interface {
	Exists(path string) bool
}

// osFileLookup is the production FileLookup backed by os.Stat.
type osFileLookup struct{}

// NewOSFileLookup returns the production FileLookup backed by os.Stat.
func NewOSFileLookup() FileLookup { return osFileLookup{} }

func (osFileLookup) Exists(path string) bool {
	return statExists(path)
}

// NologinShell resolves the nologin shell to use for the dedicated Linux
// service account: /sbin/nologin when present, otherwise
// /usr/sbin/nologin. AI.md PART 24 "System User Requirements" documents
// both forms; the literal useradd example uses /sbin/nologin, which is
// what this returns whenever it exists.
func NologinShell(fl FileLookup) string {
	if fl.Exists("/sbin/nologin") {
		return "/sbin/nologin"
	}
	if fl.Exists("/usr/sbin/nologin") {
		return "/usr/sbin/nologin"
	}
	return "/sbin/nologin"
}

// LinuxCreateCommands returns the groupadd/useradd invocations that create
// the dedicated system group and user with matching UID/GID. Verbatim per
// AI.md PART 24 "Platform-Specific Commands" > Linux, with the shell
// resolved via NologinShell.
func LinuxCreateCommands(name string, id int, homeDir, shell string) [][]string {
	return [][]string{
		{"groupadd", "--system", "--gid", strconv.Itoa(id), name},
		{
			"useradd", "--system",
			"--uid", strconv.Itoa(id),
			"--gid", strconv.Itoa(id),
			"--home-dir", homeDir,
			"--shell", shell,
			"--comment", name + " service account",
			name,
		},
	}
}

// DarwinCreateCommands returns the dscl invocations that create the
// dedicated group and hidden system user. Verbatim per AI.md PART 24
// "Go Implementation (macOS)".
func DarwinCreateCommands(name string, id int, homeDir string) [][]string {
	return [][]string{
		{"dscl", ".", "-create", "/Groups/" + name},
		{"dscl", ".", "-create", "/Groups/" + name, "PrimaryGroupID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Groups/" + name, "Password", "*"},
		{"dscl", ".", "-create", "/Users/" + name},
		{"dscl", ".", "-create", "/Users/" + name, "UniqueID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Users/" + name, "PrimaryGroupID", strconv.Itoa(id)},
		{"dscl", ".", "-create", "/Users/" + name, "UserShell", "/usr/bin/false"},
		{"dscl", ".", "-create", "/Users/" + name, "RealName", name + " service account"},
		{"dscl", ".", "-create", "/Users/" + name, "NFSHomeDirectory", homeDir},
		{"dscl", ".", "-create", "/Users/" + name, "Password", "*"},
		{"dscl", ".", "-create", "/Users/" + name, "IsHidden", "1"},
	}
}

// FreeBSDCreateCommands returns the pw invocations that create the
// dedicated group and user with matching UID/GID. Verbatim per AI.md
// PART 24 "Platform-Specific Commands" > FreeBSD.
func FreeBSDCreateCommands(name string, id int, homeDir string) [][]string {
	return [][]string{
		{"pw", "groupadd", "-n", name, "-g", strconv.Itoa(id)},
		{
			"pw", "useradd",
			"-n", name,
			"-u", strconv.Itoa(id),
			"-g", strconv.Itoa(id),
			"-d", homeDir,
			"-s", "/usr/sbin/nologin",
			"-c", name + " service account",
		},
	}
}

// CreateCommands dispatches to the per-OS command builder for goos. Windows
// returns (nil, nil): Windows services run as the auto-managed Virtual
// Service Account (NT SERVICE\{internal_name}), so no account is created.
func CreateCommands(goos, name string, id int, homeDir string, fl FileLookup) ([][]string, error) {
	switch goos {
	case "linux":
		return LinuxCreateCommands(name, id, homeDir, NologinShell(fl)), nil
	case "darwin":
		return DarwinCreateCommands(name, id, homeDir), nil
	case "freebsd", "openbsd", "netbsd":
		return FreeBSDCreateCommands(name, id, homeDir), nil
	case "windows":
		return nil, nil
	default:
		return nil, fmt.Errorf("service: unsupported GOOS %q for system user creation", goos)
	}
}

// RunAll executes every command in commands via r, stopping and returning
// the first error encountered.
func RunAll(r Runner, commands [][]string) error {
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		if err := r.Run(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}
	return nil
}
