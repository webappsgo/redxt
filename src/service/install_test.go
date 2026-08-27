package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestManager builds a Manager rooted at t.TempDir(), with every OS-facing
// dependency (Runner, IDLU, PathLU, FileLU) faked, so tests never touch the
// real init system, filesystem outside the tempdir, or user database. For
// goos "linux" it pre-seeds FileLU/PathLU so systemd is the detected init
// system by default; tests exercising a different init system or a
// detection failure reassign m.FileLU/m.PathLU/m.GOOS directly.
func newTestManager(t *testing.T, goos string) (*Manager, *fakeRunner) {
	t.Helper()
	root := t.TempDir()
	runner := newFakeRunner()

	fileLU := newFakeFileLookup()
	if goos == "linux" {
		fileLU = newFakeFileLookup("/run/systemd/system")
	}
	pathLU := newFakePathLookup("systemctl", "sv", "runsvdir", "update-rc.d", "chkconfig")

	return &Manager{
		Ctx: Context{
			ProjectName:  "redxt",
			ProjectOrg:   "webappsgo",
			InternalOrg:  "webappsgo",
			InternalName: "redxt",
			BinaryPath:   filepath.Join(root, "bin", "redxt"),
			ConfigDir:    filepath.Join(root, "config"),
			DataDir:      filepath.Join(root, "data"),
			CacheDir:     filepath.Join(root, "cache"),
			LogDir:       filepath.Join(root, "log"),
			BackupDir:    filepath.Join(root, "backup"),
			PIDFile:      filepath.Join(root, "redxt.pid"),
		},
		Runner:  runner,
		Log:     noopLogger{},
		GOOS:    goos,
		Root:    root,
		IDLU:    newFakeIDLookup(),
		PathLU:  pathLU,
		FileLU:  fileLU,
		Confirm: func(string) bool { return true },
	}, runner
}

func TestManagerInstallSystemd(t *testing.T) {
	m, runner := newTestManager(t, "linux")

	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	unitPath := m.Root + "/etc/systemd/system/redxt.service"
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("unit file not written: %v", err)
	}
	wantCalls := [][]string{
		{"systemctl", "enable", "redxt"},
		{"systemctl", "start", "redxt"},
	}
	assertCommandsEqual(t, runner.calls, wantCalls)
}

func TestManagerInstallNoInitDetected(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	m.FileLU = newFakeFileLookup()
	if err := m.Install(); err == nil {
		t.Fatal("expected an error when no init system is detectable")
	}
}

func TestManagerInstallWindowsSCM(t *testing.T) {
	m, _ := newTestManager(t, "windows")
	err := m.Install()
	if err == nil || !strings.Contains(err.Error(), "GOOS=windows") {
		t.Fatalf("Install: got %v, want the win-stub GOOS=windows error", err)
	}
}

func TestManagerUninstallAborted(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	m.Confirm = func(prompt string) bool {
		if prompt != uninstallConfirmPrompt {
			t.Errorf("unexpected prompt: %q", prompt)
		}
		return false
	}
	_, err := m.Uninstall()
	if err != ErrUninstallAborted {
		t.Fatalf("got %v, want ErrUninstallAborted", err)
	}
}

func TestManagerUninstallSystemd(t *testing.T) {
	m, runner := newTestManager(t, "linux")

	// Seed everything Uninstall is expected to remove.
	for _, dir := range []string{m.Ctx.ConfigDir, m.Ctx.DataDir, m.Ctx.CacheDir, m.Ctx.LogDir, m.Ctx.BackupDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(m.Ctx.PIDFile, []byte("123"), 0o640); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}
	unitPath := m.Root + "/etc/systemd/system/redxt.service"
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(unitPath, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	result, err := m.Uninstall()
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !strings.Contains(result.Message, m.Ctx.BinaryPath) {
		t.Errorf("Uninstall message %q does not mention binary path", result.Message)
	}
	for _, dir := range []string{m.Ctx.ConfigDir, m.Ctx.DataDir, m.Ctx.CacheDir, m.Ctx.LogDir, m.Ctx.BackupDir} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", dir)
		}
	}
	if _, err := os.Stat(m.Ctx.PIDFile); !os.IsNotExist(err) {
		t.Errorf("pidfile should have been removed")
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("unit file should have been removed")
	}
	wantCalls := [][]string{
		{"systemctl", "stop", "redxt"},
		{"systemctl", "disable", "redxt"},
	}
	assertCommandsEqual(t, runner.calls, wantCalls)
}

func TestManagerUninstallWindowsSCM(t *testing.T) {
	m, _ := newTestManager(t, "windows")
	_, err := m.Uninstall()
	if err == nil || !strings.Contains(err.Error(), "GOOS=windows") {
		t.Fatalf("Uninstall: got %v, want the win-stub GOOS=windows error", err)
	}
}

func TestManagerDisable(t *testing.T) {
	m, runner := newTestManager(t, "linux")
	if err := m.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	wantCalls := [][]string{
		{"systemctl", "stop", "redxt"},
		{"systemctl", "disable", "redxt"},
	}
	assertCommandsEqual(t, runner.calls, wantCalls)
}

func TestManagerDisableWindowsSCM(t *testing.T) {
	m, _ := newTestManager(t, "windows")
	err := m.Disable()
	if err == nil || !strings.Contains(err.Error(), "GOOS=windows") {
		t.Fatalf("Disable: got %v, want the win-stub GOOS=windows error", err)
	}
}

func TestManagerDisableNoInitDetected(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	m.FileLU = newFakeFileLookup()
	if err := m.Disable(); err == nil {
		t.Fatal("expected an error when no init system is detectable")
	}
}

func TestManagerLifecycleSystemd(t *testing.T) {
	cases := []struct {
		action string
		call   func(*Manager) error
		want   []string
	}{
		{"start", (*Manager).Start, []string{"systemctl", "start", "redxt"}},
		{"stop", (*Manager).Stop, []string{"systemctl", "stop", "redxt"}},
		{"restart", (*Manager).Restart, []string{"systemctl", "restart", "redxt"}},
		{"reload", (*Manager).Reload, []string{"systemctl", "reload", "redxt"}},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			m, runner := newTestManager(t, "linux")
			if err := tc.call(m); err != nil {
				t.Fatalf("%s: %v", tc.action, err)
			}
			assertCommandsEqual(t, runner.calls, [][]string{tc.want})
		})
	}
}

func TestManagerLifecycleLaunchdFallsBackOnRestartAndReload(t *testing.T) {
	cases := []struct {
		action string
		call   func(*Manager) error
	}{
		{"restart", (*Manager).Restart},
		{"reload", (*Manager).Reload},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			m, runner := newTestManager(t, "darwin")
			plist := m.Root + "/Library/LaunchDaemons/org.webappsgo.redxt.plist"
			want := [][]string{
				{"launchctl", "unload", plist},
				{"launchctl", "load", plist},
			}
			if err := tc.call(m); err != nil {
				t.Fatalf("%s: %v", tc.action, err)
			}
			assertCommandsEqual(t, runner.calls, want)
		})
	}
}

func TestManagerLifecycleUnknownAction(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	if err := m.runLifecycle("bogus"); err == nil {
		t.Fatal("expected an error for an unknown lifecycle action")
	}
}

func TestManagerLifecycleNoInitDetected(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	m.FileLU = newFakeFileLookup()
	if err := m.Start(); err == nil {
		t.Fatal("expected an error when no init system is detectable")
	}
}

func TestManagerLifecycleWindowsSCM(t *testing.T) {
	cases := []func(*Manager) error{
		(*Manager).Start, (*Manager).Stop, (*Manager).Restart, (*Manager).Reload,
	}
	for _, call := range cases {
		m, _ := newTestManager(t, "windows")
		if err := call(m); err == nil || !strings.Contains(err.Error(), "GOOS=windows") {
			t.Fatalf("got %v, want the win-stub GOOS=windows error", err)
		}
	}
}

func TestManagerStatusInstalledAndEnabled(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	unitPath := m.Root + "/etc/systemd/system/redxt.service"
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(unitPath, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}
	// Status probes FileLU.Exists for the unit path, so the fake must know
	// about it independently of the file actually existing on disk.
	m.FileLU = newFakeFileLookup("/run/systemd/system", unitPath)
	if err := os.WriteFile(m.Ctx.PIDFile, []byte("4242"), 0o640); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}

	st, err := m.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true")
	}
	if !st.AutoStart {
		t.Error("expected AutoStart=true (systemctl is-enabled fake succeeds)")
	}
}

func TestManagerStatusNotInstalled(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	st, err := m.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false: unit file was never written")
	}
	if st.AutoStart {
		t.Error("expected AutoStart=false when not installed")
	}
}

func TestManagerStatusWindowsSCM(t *testing.T) {
	m, _ := newTestManager(t, "windows")
	_, err := m.Status()
	if err == nil || !strings.Contains(err.Error(), "GOOS=windows") {
		t.Fatalf("Status: got %v, want the win-stub GOOS=windows error", err)
	}
}

func TestManagerStatusNoInitDetected(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	m.FileLU = newFakeFileLookup()
	if _, err := m.Status(); err == nil {
		t.Fatal("expected an error when no init system is detectable")
	}
}

func TestManagerDeleteSystemUserWindowsIsNoop(t *testing.T) {
	m, runner := newTestManager(t, "windows")
	if err := m.deleteSystemUser(); err != nil {
		t.Fatalf("deleteSystemUser: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no commands run, got %v", runner.calls)
	}
}

func TestManagerDeleteSystemUserAbsentAccount(t *testing.T) {
	m, runner := newTestManager(t, "linux")
	// "redxt-service-account-that-does-not-exist" is never a real account
	// on the test host, so user.Lookup fails and deleteSystemUser returns
	// nil without running any command.
	m.Ctx.InternalName = "redxt-service-account-that-does-not-exist"
	if err := m.deleteSystemUser(); err != nil {
		t.Fatalf("deleteSystemUser: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no commands run for an absent account, got %v", runner.calls)
	}
}

func TestManagerDeleteSystemUserUnsupportedGOOS(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	// user.Lookup("root") succeeds on every unix test host, taking the
	// deleteSystemUser code path into the goos switch.
	m.Ctx.InternalName = "root"
	m.GOOS = "plan9"
	if err := m.deleteSystemUser(); err == nil {
		t.Fatal("expected an error for an unsupported GOOS")
	}
}
