package pidfile

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// withProcessFuncs substitutes the injectable process-check variables for
// the duration of a test and restores them afterward.
func withProcessFuncs(t *testing.T, running, ours func(pid int) bool) {
	t.Helper()
	origRunning := isProcessRunning
	origOurs := isOurProcess
	isProcessRunning = running
	isOurProcess = ours
	t.Cleanup(func() {
		isProcessRunning = origRunning
		isOurProcess = origOurs
	})
}

// withContainer substitutes the injectable container-detection variable for
// the duration of a test and restores it afterward.
func withContainer(t *testing.T, isContainer bool) {
	t.Helper()
	orig := isContainerFunc
	isContainerFunc = func() bool { return isContainer }
	t.Cleanup(func() {
		isContainerFunc = orig
	})
}

func alwaysTrue(int) bool  { return true }
func alwaysFalse(int) bool { return false }

func TestCheck_MissingFile(t *testing.T) {
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.pid")

	running, pid, err := Check(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Fatalf("expected not running")
	}
	if pid != 0 {
		t.Fatalf("expected pid 0, got %d", pid)
	}
}

func TestCheck_CorruptContents(t *testing.T) {
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.pid")
	if err := os.WriteFile(path, []byte("not-a-pid"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	running, pid, err := Check(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running || pid != 0 {
		t.Fatalf("expected (false, 0), got (%v, %d)", running, pid)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected corrupt pid file to be removed")
	}
}

func TestCheck_StalePID(t *testing.T) {
	withProcessFuncs(t, alwaysFalse, alwaysTrue)
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.pid")
	if err := os.WriteFile(path, []byte("12345"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	running, pid, err := Check(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running || pid != 0 {
		t.Fatalf("expected (false, 0), got (%v, %d)", running, pid)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected stale pid file to be removed")
	}
}

func TestCheck_DifferentBinary(t *testing.T) {
	withProcessFuncs(t, alwaysTrue, alwaysFalse)
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "other.pid")
	if err := os.WriteFile(path, []byte("54321"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	running, pid, err := Check(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running || pid != 0 {
		t.Fatalf("expected (false, 0), got (%v, %d)", running, pid)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected pid file for other binary to be removed")
	}
}

func TestCheck_LiveMatchingPID(t *testing.T) {
	withProcessFuncs(t, alwaysTrue, alwaysTrue)
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "live.pid")
	if err := os.WriteFile(path, []byte("99999"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	running, pid, err := Check(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !running {
		t.Fatalf("expected running")
	}
	if pid != 99999 {
		t.Fatalf("expected pid 99999, got %d", pid)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected live pid file to remain: %v", statErr)
	}
}

// TestCheck_UnreadableFile uses a directory in place of the PID file so
// the read fails with EISDIR. A mode-based unreadable file would not
// work here because the test suite runs as root in the build container.
func TestCheck_UnreadableFile(t *testing.T) {
	withContainer(t, false)
	path := filepath.Join(t.TempDir(), "unreadable.pid")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("creating directory in place of pid file: %v", err)
	}

	_, _, err := Check(path)
	if err == nil {
		t.Fatalf("expected error for unreadable path")
	}
}

func TestWrite_Succeeds(t *testing.T) {
	withProcessFuncs(t, alwaysFalse, alwaysFalse)
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "write.pid")

	if err := Write(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written pid file: %v", err)
	}
	got, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("pid file contents not numeric: %q", data)
	}
	if got != os.Getpid() {
		t.Fatalf("expected pid %d, got %d", os.Getpid(), got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat pid file: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("expected mode 0644, got %v", info.Mode().Perm())
	}
}

func TestWrite_FailsWhenAlreadyRunning(t *testing.T) {
	withProcessFuncs(t, alwaysTrue, alwaysTrue)
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "running.pid")
	if err := os.WriteFile(path, []byte("42"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	err := Write(path)
	if err == nil {
		t.Fatalf("expected error when already running")
	}
}

func TestWrite_NoopInContainer(t *testing.T) {
	withContainer(t, true)
	dir := t.TempDir()
	path := filepath.Join(dir, "container.pid")

	if err := Write(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected no pid file to be written in a container")
	}
}

func TestRemove_Idempotent(t *testing.T) {
	withContainer(t, false)
	dir := t.TempDir()
	path := filepath.Join(dir, "remove.pid")
	if err := os.WriteFile(path, []byte("1"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("unexpected error removing existing file: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected pid file to be removed")
	}

	if err := Remove(path); err != nil {
		t.Fatalf("unexpected error removing already-gone file: %v", err)
	}
}

func TestRemove_NoopInContainer(t *testing.T) {
	withContainer(t, true)
	dir := t.TempDir()
	path := filepath.Join(dir, "container-remove.pid")
	if err := os.WriteFile(path, []byte("1"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	if err := Remove(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected pid file to remain untouched in a container: %v", statErr)
	}
}

// TestDetectContainer_EnvVar exercises the environment-variable branch with
// the filesystem markers pointed at a temporary directory, because the test
// suite itself runs inside the build container where the real markers exist.
func TestDetectContainer_EnvVar(t *testing.T) {
	origMarkers := containerMarkerFiles
	containerMarkerFiles = []string{filepath.Join(t.TempDir(), "absent-marker")}
	t.Cleanup(func() {
		containerMarkerFiles = origMarkers
	})

	tests := []struct {
		name      string
		container string
		lowercase string
		want      bool
	}{
		{name: "no markers", want: false},
		{name: "CONTAINER set", container: "docker", want: true},
		{name: "container set", lowercase: "podman", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CONTAINER", tt.container)
			t.Setenv("container", tt.lowercase)
			if got := detectContainer(); got != tt.want {
				t.Fatalf("detectContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectContainer_MarkerFile covers the filesystem-marker branch with
// both environment variables cleared.
func TestDetectContainer_MarkerFile(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "dockerenv")
	if err := os.WriteFile(marker, nil, 0644); err != nil {
		t.Fatalf("creating marker file: %v", err)
	}
	origMarkers := containerMarkerFiles
	containerMarkerFiles = []string{marker}
	t.Cleanup(func() {
		containerMarkerFiles = origMarkers
	})
	t.Setenv("CONTAINER", "")
	t.Setenv("container", "")

	if !detectContainer() {
		t.Fatalf("expected detectContainer to report true when a marker file exists")
	}
}
