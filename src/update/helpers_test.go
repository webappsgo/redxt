package update

import (
	"os"
	"testing"
)

// writeFile writes contents to path, failing the test on error.
func writeFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readFile reads path, returning the error for the caller to assert on.
func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}
