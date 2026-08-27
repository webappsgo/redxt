// Package credfile provides a shared permission gate for reading
// credential files (client cli.yml/token files, agent agent.yml/token
// files). AI.md 52847 requires os.Stat() on read and refusal when
// info.Mode().Perm()&0o077 != 0 — a world/group readable credential
// file must never be trusted. Skipped on Windows, which has no POSIX
// permission bits and relies on ACL inheritance instead.
package credfile

import (
	"fmt"
	"os"
	"runtime"
)

// CheckPerms stats path and returns an error if the file is group- or
// other-accessible (any of read/write/execute), per AI.md 52847. It is
// a no-op on Windows. Callers should invoke this before trusting the
// contents of any on-disk credential file (config with an embedded
// token, or a separate token file).
func CheckPerms(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential file %s has permissive mode %04o; expected 0600 (chmod 600 %s)", path, info.Mode().Perm(), path)
	}
	return nil
}
