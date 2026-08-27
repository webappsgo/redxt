//go:build windows

package service

// DropPrivileges is a no-op on Windows: the service already runs as the
// minimal-privilege Virtual Service Account (NT SERVICE\{internal_name}),
// per AI.md PART 24/25, so there is nothing to drop after port binding.
func DropPrivileges(uid, gid int) error {
	return nil
}
