//go:build linux || darwin || freebsd

package backup

import "syscall"

// diskUsage returns the total and free byte capacity of the filesystem
// holding path, via the POSIX statfs(2) syscall stdlib exposes on every
// unix target redxt builds for.
func diskUsage(path string) (totalBytes, freeBytes uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	bsize := uint64(stat.Bsize)
	return bsize * uint64(stat.Blocks), bsize * uint64(stat.Bavail), nil
}
