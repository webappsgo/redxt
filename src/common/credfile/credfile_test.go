package credfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckPermsAcceptsPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := CheckPerms(path); err != nil {
		t.Errorf("CheckPerms() on 0600 file = %v, want nil", err)
	}
}

func TestCheckPermsRejectsGroupOrWorldAccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	cases := []os.FileMode{0o644, 0o666, 0o640, 0o604}
	for _, mode := range cases {
		path := filepath.Join(dir, mode.String())
		if err := os.WriteFile(path, []byte("secret"), mode); err != nil {
			t.Fatalf("write fixture (mode %o): %v", mode, err)
		}
		if err := CheckPerms(path); err == nil {
			t.Errorf("CheckPerms() on mode %04o file = nil, want error", mode)
		}
	}
}

func TestCheckPermsSkippedOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only behavior")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret"), 0o666); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := CheckPerms(path); err != nil {
		t.Errorf("CheckPerms() on Windows = %v, want nil (no POSIX bit enforcement)", err)
	}
}

func TestCheckPermsMissingFile(t *testing.T) {
	dir := t.TempDir()
	if err := CheckPerms(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("CheckPerms() expected error for missing file")
	}
}
