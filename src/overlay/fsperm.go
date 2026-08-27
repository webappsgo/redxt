package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ensureDir creates dir if missing and enforces 0700 permissions and
// current-user ownership on every call, even when the directory already
// existed. Chmod/chown are skipped on Windows, which has no POSIX
// permission bits and relies on inherited ACLs from the user profile.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("chmod dir %s: %w", dir, err)
	}
	if err := os.Chown(dir, os.Getuid(), os.Getgid()); err != nil {
		return fmt.Errorf("chown dir %s: %w", dir, err)
	}
	return nil
}

// writeSecureFile (over)writes path with content, creating the parent
// directory first and enforcing 0600 permissions and current-user ownership
// on every call. Used for torrc, tunnels.conf, and any persisted key
// material - all of it private, all of it regenerated or replaced as a whole
// file rather than patched in place.
func writeSecureFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod file: %w", err)
	}
	if err := os.Chown(path, os.Getuid(), os.Getgid()); err != nil {
		return fmt.Errorf("chown file: %w", err)
	}
	return nil
}
