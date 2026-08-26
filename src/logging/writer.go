package logging

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// File permissions per the PART 11 "File Permissions" rule: the log
// directory is owner and group traversable, log files are owner
// writable and group readable only.
const (
	logDirMode  os.FileMode = 0o750
	logFileMode os.FileMode = 0o640
)

// ErrClosed is returned by Write after the file has been closed.
var ErrClosed = errors.New("logging: file is closed")

// File is an append-only log file with the built-in rotation and
// retention PART 11 requires, so no external logrotate is needed. It
// is safe for concurrent use.
//
// A rotated file keeps its extension and gains a UTC timestamp before
// it: "access.log" becomes "access.20250115-000000.log", and its
// compressed form is "access.20250115-000000.log.gz". Rotated names
// therefore sort chronologically and stay recognizable by extension.
type File struct {
	mu sync.Mutex

	dir  string
	name string
	base string
	ext  string

	rotate   policy
	keep     string
	compress bool

	handle   *os.File
	size     int64
	openedAt time.Time
	closed   bool
}

// Open opens (creating it if needed) the log file name inside dir,
// applying the given rotation and retention policies. The directory is
// created with mode 0750 and the file with mode 0640.
func Open(dir, name, rotate, keep string) (*File, error) {
	return OpenCompressed(dir, name, rotate, keep, false)
}

// OpenCompressed is Open with the option to gzip each rotated file
// before retention is applied, which backs the audit log's compress
// setting.
func OpenCompressed(dir, name, rotate, keep string, compress bool) (*File, error) {
	if name == "" {
		return nil, errors.New("logging: empty log file name")
	}
	if err := os.MkdirAll(dir, logDirMode); err != nil {
		return nil, fmt.Errorf("logging: create log dir %s: %w", dir, err)
	}

	ext := filepath.Ext(name)
	f := &File{
		dir:      dir,
		name:     name,
		base:     strings.TrimSuffix(name, ext),
		ext:      ext,
		rotate:   parseRotate(rotate),
		keep:     keep,
		compress: compress,
	}
	if err := f.open(); err != nil {
		return nil, err
	}
	return f, nil
}

// Path returns the full path of the live log file.
func (f *File) Path() string {
	return filepath.Join(f.dir, f.name)
}

// Write appends p to the log file, rotating first when the rotation
// policy calls for it. It never writes a partial record: the rotation
// decision is made before the write, so a record always lands whole in
// one file.
func (f *File) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return 0, ErrClosed
	}
	if f.needsRotate(len(p), time.Now()) {
		if err := f.rotateLocked(); err != nil {
			return 0, err
		}
	}

	n, err := f.handle.Write(p)
	f.size += int64(n)
	if err != nil {
		return n, fmt.Errorf("logging: write %s: %w", f.Path(), err)
	}
	return n, nil
}

// Rotate rotates the file immediately, regardless of policy, then
// applies compression and retention. It exists so a caller (and the
// package's own tests) can force a rotation.
func (f *File) Rotate() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return ErrClosed
	}
	return f.rotateLocked()
}

// Reopen closes and reopens the live file without rotating it. It backs
// the SIGUSR1 handler, so an external rotator (logrotate) can move the
// file aside and have the process write to the new inode.
func (f *File) Reopen() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return ErrClosed
	}
	if f.handle != nil {
		if err := f.handle.Close(); err != nil {
			return fmt.Errorf("logging: close %s: %w", f.Path(), err)
		}
		f.handle = nil
	}
	return f.open()
}

// Close closes the live file. It is idempotent: closing an already
// closed file is not an error.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}
	f.closed = true
	if f.handle == nil {
		return nil
	}
	err := f.handle.Close()
	f.handle = nil
	if err != nil {
		return fmt.Errorf("logging: close %s: %w", f.Path(), err)
	}
	return nil
}

// open opens the live file in append mode and records its current size
// and period start. The period start comes from the file's
// modification time when it already holds data, so a restart inside an
// existing period does not reset the rotation boundary.
func (f *File) open() error {
	handle, err := os.OpenFile(f.Path(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, logFileMode)
	if err != nil {
		return fmt.Errorf("logging: open %s: %w", f.Path(), err)
	}
	f.handle = handle
	f.size = 0
	f.openedAt = time.Now()
	if info, err := handle.Stat(); err == nil {
		f.size = info.Size()
		if f.size > 0 {
			f.openedAt = info.ModTime()
		}
	}
	return nil
}

// needsRotate reports whether writing n more bytes at now must be
// preceded by a rotation. A combined policy rotates on whichever of
// the time and size components fires first.
func (f *File) needsRotate(n int, now time.Time) bool {
	if shouldRotateTime(f.openedAt, now, f.rotate.interval) {
		return true
	}
	if f.rotate.maxSize > 0 && f.size > 0 && f.size+int64(n) > f.rotate.maxSize {
		return true
	}
	return false
}

// rotateLocked performs the rotation itself: close, rename with a UTC
// stamp, reopen a fresh file, compress the rotated file when
// configured, then prune according to the retention policy. The caller
// holds the mutex.
func (f *File) rotateLocked() error {
	if f.handle != nil {
		if err := f.handle.Close(); err != nil {
			return fmt.Errorf("logging: close %s: %w", f.Path(), err)
		}
		f.handle = nil
	}

	rotated := f.rotatedPath(time.Now().UTC())
	if err := os.Rename(f.Path(), rotated); err != nil && !os.IsNotExist(err) {
		// Reopen so logging survives a failed rename rather than
		// losing every later record.
		_ = f.open()
		return fmt.Errorf("logging: rotate %s: %w", f.Path(), err)
	}

	if err := f.open(); err != nil {
		return err
	}

	if f.compress {
		if err := compressFile(rotated); err != nil {
			return err
		}
	}
	return f.prune(time.Now())
}

// rotatedPath returns the destination name for a rotation at stamp. A
// second rotation inside the same second is stamped one second later
// rather than given a numeric suffix, so rotated names stay unique and
// still sort chronologically.
func (f *File) rotatedPath(stamp time.Time) string {
	for i := 0; ; i++ {
		candidate := stamp.Add(time.Duration(i) * time.Second)
		path := filepath.Join(f.dir, f.base+"."+candidate.Format(stampLayout)+f.ext)
		if !pathExists(path) && !pathExists(path+".gz") {
			return path
		}
	}
}

// pathExists reports whether a filesystem entry exists at path.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// prune deletes the rotated files the retention policy no longer
// keeps. Compression has already run, so a compressed file is subject
// to exactly the same retention as an uncompressed one.
func (f *File) prune(now time.Time) error {
	files, err := f.rotatedFiles()
	if err != nil {
		return err
	}
	var errs []error
	for _, victim := range pruneList(files, f.keep, now) {
		if err := os.Remove(victim.Path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("logging: remove %s: %w", victim.Path, err))
		}
	}
	return errors.Join(errs...)
}

// rotatedFiles lists the rotated siblings of the live file, parsing
// the rotation stamp out of each name. A file whose name does not
// carry a parseable stamp is ignored, so unrelated files in the log
// directory are never deleted.
func (f *File) rotatedFiles() ([]rotatedFile, error) {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, fmt.Errorf("logging: read log dir %s: %w", f.dir, err)
	}

	prefix := f.base + "."
	var out []rotatedFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == f.name || !strings.HasPrefix(name, prefix) {
			continue
		}
		stamp, ok := parseStamp(name, prefix, f.ext)
		if !ok {
			continue
		}
		out = append(out, rotatedFile{
			Name:  name,
			Path:  filepath.Join(f.dir, name),
			Stamp: stamp,
		})
	}
	return out, nil
}

// parseStamp extracts the rotation timestamp from a rotated file name,
// accepting both the plain and the gzipped form.
func parseStamp(name, prefix, ext string) (time.Time, bool) {
	rest := strings.TrimPrefix(name, prefix)
	rest = strings.TrimSuffix(rest, ".gz")
	rest = strings.TrimSuffix(rest, ext)
	if len(rest) < len(stampLayout) {
		return time.Time{}, false
	}
	stamp, err := time.ParseInLocation(stampLayout, rest[:len(stampLayout)], time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return stamp, true
}

// compressFile gzips path to path+".gz" and removes the original. A
// missing source is not an error: the file was already pruned.
func compressFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("logging: open %s: %w", path, err)
	}
	defer func() {
		_ = src.Close()
	}()

	dstPath := path + ".gz"
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFileMode)
	if err != nil {
		return fmt.Errorf("logging: create %s: %w", dstPath, err)
	}

	zw := gzip.NewWriter(dst)
	if _, err := io.Copy(zw, src); err != nil {
		_ = zw.Close()
		_ = dst.Close()
		return fmt.Errorf("logging: compress %s: %w", path, err)
	}
	if err := zw.Close(); err != nil {
		_ = dst.Close()
		return fmt.Errorf("logging: compress %s: %w", path, err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("logging: close %s: %w", dstPath, err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("logging: remove %s: %w", path, err)
	}
	return nil
}
