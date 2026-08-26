package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReopenWritesToTheNewInode reproduces what an external rotator
// does: it renames the live file away, then signals the process. After
// Reopen the writes must land in a freshly created file, not in the
// renamed one.
func TestReopenWritesToTheNewInode(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})

	logger.Infof("before rotation")
	live := filepath.Join(dir, "server.log")
	moved := filepath.Join(dir, "server.log.moved")
	if err := os.Rename(live, moved); err != nil {
		t.Fatalf("rename live log: %v", err)
	}

	if err := logger.Reopen(); err != nil {
		t.Fatalf("Reopen() error = %v", err)
	}
	logger.Infof("after rotation")

	movedData, err := os.ReadFile(moved)
	if err != nil {
		t.Fatalf("read moved log: %v", err)
	}
	if !strings.Contains(string(movedData), "before rotation") {
		t.Fatalf("moved log lost its content: %q", movedData)
	}
	if strings.Contains(string(movedData), "after rotation") {
		t.Fatalf("post-reopen write landed in the moved file: %q", movedData)
	}

	liveData, err := os.ReadFile(live)
	if err != nil {
		t.Fatalf("read reopened log: %v", err)
	}
	if !strings.Contains(string(liveData), "after rotation") {
		t.Fatalf("reopened log missing the new line: %q", liveData)
	}
	if strings.Contains(string(liveData), "before rotation") {
		t.Fatalf("reopened log should start empty: %q", liveData)
	}
}

// TestReopenAfterCloseIsNoop guards the shutdown ordering: a signal
// arriving after Close must not resurrect the log files.
func TestReopenAfterCloseIsNoop(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "server.log")); err != nil {
		t.Fatalf("remove log: %v", err)
	}

	if err := logger.Reopen(); err != nil {
		t.Fatalf("Reopen() after Close error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "server.log")); !os.IsNotExist(err) {
		t.Fatalf("Reopen recreated a log file after Close")
	}
}
