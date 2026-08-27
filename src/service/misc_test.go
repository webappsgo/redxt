package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecRunnerRunSuccessAndFailure(t *testing.T) {
	r := NewExecRunner()
	if err := r.Run("true"); err != nil {
		t.Errorf("Run(true) = %v, want nil", err)
	}

	err := r.Run("false")
	if err == nil {
		t.Fatal("Run(false) = nil, want an error")
	}
	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("Run(false) error is %T, want *RunError", err)
	}
	if runErr.Name != "false" {
		t.Errorf("RunError.Name = %q, want %q", runErr.Name, "false")
	}
	if runErr.Unwrap() == nil {
		t.Error("RunError.Unwrap() = nil, want the underlying exec error")
	}
	if runErr.Error() == "" {
		t.Error("RunError.Error() must not be empty")
	}
}

func TestExecRunnerOutputSuccessAndFailure(t *testing.T) {
	r := NewExecRunner()
	out, err := r.Output("echo", "hello")
	if err != nil {
		t.Fatalf("Output(echo): %v", err)
	}
	if got := string(out); got != "hello\n" {
		t.Errorf("Output(echo) = %q, want %q", got, "hello\n")
	}

	if _, err := r.Output("sh", "-c", "exit 1"); err == nil {
		t.Fatal("Output(exit 1) = nil error, want an error")
	}
}

func TestStatExistsAndOSFileLookup(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "present")
	absent := filepath.Join(dir, "absent")
	if err := os.WriteFile(present, []byte("x"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !statExists(present) {
		t.Error("statExists(present) = false, want true")
	}
	if statExists(absent) {
		t.Error("statExists(absent) = true, want false")
	}

	fl := NewOSFileLookup()
	if !fl.Exists(present) {
		t.Error("OSFileLookup.Exists(present) = false, want true")
	}
	if fl.Exists(absent) {
		t.Error("OSFileLookup.Exists(absent) = true, want false")
	}
}

func TestOSIDLookupResolvesRootAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os/user UID/GID lookups are POSIX-specific")
	}
	lu := NewOSIDLookup()
	if !lu.LookupUID(0) {
		t.Error("LookupUID(0) = false, want true: uid 0 (root) always exists")
	}
	if !lu.LookupGID(0) {
		t.Error("LookupGID(0) = false, want true: gid 0 (root) always exists")
	}
	if lu.LookupUID(999999) {
		t.Error("LookupUID(999999) = true, want false: implausible uid")
	}
}

func TestOSPathLookupResolvesShellBinary(t *testing.T) {
	lu := NewOSPathLookup()
	if _, err := lu.LookPath("sh"); err != nil {
		t.Errorf("LookPath(sh): %v", err)
	}
	if _, err := lu.LookPath("redxt-service-binary-that-does-not-exist"); err == nil {
		t.Error("LookPath(nonexistent) = nil error, want an error")
	}
}

func TestRunAllStopsAtFirstError(t *testing.T) {
	r := newFakeRunner()
	r.failOn[r.key("b", "x")] = true

	err := RunAll(r, [][]string{
		{"a"},
		{"b", "x"},
		{"c"},
	})
	if err == nil {
		t.Fatal("expected an error from the failing command")
	}
	if len(r.calls) != 2 {
		t.Fatalf("expected RunAll to stop after the failing command, got %d calls: %v", len(r.calls), r.calls)
	}
}

func TestRunAllSkipsEmptyCommands(t *testing.T) {
	r := newFakeRunner()
	if err := RunAll(r, [][]string{nil, {}, {"a"}}); err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected only the non-empty command to run, got %v", r.calls)
	}
}

func TestCreateCommandsLinux(t *testing.T) {
	got, err := CreateCommands("linux", "redxt", 512, "/etc/webappsgo/redxt", newFakeFileLookup("/sbin/nologin"))
	if err != nil {
		t.Fatalf("CreateCommands: %v", err)
	}
	want := LinuxCreateCommands("redxt", 512, "/etc/webappsgo/redxt", "/sbin/nologin")
	assertCommandsEqual(t, got, want)
}

func TestCreateCommandsDarwin(t *testing.T) {
	got, err := CreateCommands("darwin", "redxt", 300, "/usr/local/var/webappsgo/redxt", newFakeFileLookup())
	if err != nil {
		t.Fatalf("CreateCommands: %v", err)
	}
	want := DarwinCreateCommands("redxt", 300, "/usr/local/var/webappsgo/redxt")
	assertCommandsEqual(t, got, want)
}

func TestCreateCommandsFreeBSDFamily(t *testing.T) {
	for _, goos := range []string{"freebsd", "openbsd", "netbsd"} {
		t.Run(goos, func(t *testing.T) {
			got, err := CreateCommands(goos, "redxt", 250, "/var/lib/webappsgo/redxt", newFakeFileLookup())
			if err != nil {
				t.Fatalf("CreateCommands(%s): %v", goos, err)
			}
			want := FreeBSDCreateCommands("redxt", 250, "/var/lib/webappsgo/redxt")
			assertCommandsEqual(t, got, want)
		})
	}
}

func TestDropPrivilegesRefusesToStayRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("DropPrivileges is unix-only")
	}
	if os.Geteuid() != 0 {
		t.Skip("DropPrivileges needs root to exercise setuid/setgid")
	}
	// Dropping to uid/gid 0 is a deliberate degenerate case: the syscalls
	// succeed trivially (already that uid/gid), but the final
	// still-effectively-root guard must still catch it and refuse.
	err := DropPrivileges(0, 0)
	if err == nil {
		t.Fatal("DropPrivileges(0, 0) = nil, want the still-effectively-root guard error")
	}
}
