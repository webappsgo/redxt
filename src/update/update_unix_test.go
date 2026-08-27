//go:build !windows

package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceBinaryUnix(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "redxt")
	next := filepath.Join(dir, "redxt-new")

	writeFile(t, current, []byte("old contents"))
	if err := os.Chmod(current, 0o750); err != nil {
		t.Fatalf("chmod current: %v", err)
	}
	writeFile(t, next, []byte("new contents"))

	if err := replaceBinary(current, next); err != nil {
		t.Fatalf("replaceBinary() error = %v", err)
	}

	got, err := readFile(t, current)
	if err != nil {
		t.Fatalf("read current after replace: %v", err)
	}
	if string(got) != "new contents" {
		t.Fatalf("current contents = %q, want %q", got, "new contents")
	}

	if _, err := os.Stat(next); !os.IsNotExist(err) {
		t.Fatalf("new binary path still exists after rename: err = %v", err)
	}

	info, err := os.Stat(current)
	if err != nil {
		t.Fatalf("stat current after replace: %v", err)
	}
	if info.Mode().Perm() != 0o750 {
		t.Fatalf("current mode = %v, want the original 0750 restored", info.Mode().Perm())
	}
}

func TestReplaceBinaryUnixMissingCurrent(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "does-not-exist")
	next := filepath.Join(dir, "redxt-new")
	writeFile(t, next, []byte("new contents"))

	if err := replaceBinary(current, next); err == nil {
		t.Fatal("replaceBinary() error = nil, want an error when the current binary does not exist")
	}
}
