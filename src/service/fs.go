package service

import "os"

// statExists reports whether path exists on disk, backing the production
// FileLookup implementation.
func statExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
