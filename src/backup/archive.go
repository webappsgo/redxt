package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fileEntry is one file this package writes into or reads out of a backup
// archive: a name relative to the archive root, plus its content.
type fileEntry struct {
	Name string
	Data []byte
	Mode int64
}

// manifestName is the fixed path manifest.json occupies inside every
// archive this package writes.
const manifestName = "manifest.json"

// buildArchive tars and gzips entries plus manifest.json (built from
// manifestJSON) into a single in-memory ".tar.gz" payload. Nothing is
// written to disk here — the caller decides whether the returned bytes are
// the final file or get sealed by encryptArchive first, satisfying AI.md
// PART 22's "unencrypted archive never touches disk" requirement.
func buildArchive(entries []fileEntry, manifestJSON []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	all := make([]fileEntry, 0, len(entries)+1)
	all = append(all, entries...)
	all = append(all, fileEntry{Name: manifestName, Data: manifestJSON, Mode: 0o640})

	for _, e := range all {
		mode := e.Mode
		if mode == 0 {
			mode = 0o640
		}
		hdr := &tar.Header{
			Name: e.Name,
			Mode: mode,
			Size: int64(len(e.Data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("backup: write tar header %s: %w", e.Name, err)
		}
		if _, err := tw.Write(e.Data); err != nil {
			return nil, fmt.Errorf("backup: write tar content %s: %w", e.Name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("backup: close tar writer: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("backup: close gzip writer: %w", err)
	}
	return buf.Bytes(), nil
}

// extractArchive extracts a ".tar.gz" payload (already gzip+tar, never
// encrypted — callers decrypt first) into destDir, which must already
// exist. It returns the manifest.json bytes separately from the other
// entries, since verification needs to parse it apart from the checksum
// computation.
func extractArchive(archive []byte, destDir string) (manifestJSON []byte, entries []fileEntry, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, nil, fmt.Errorf("backup: open gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("backup: read tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		cleaned, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return nil, nil, err
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, fmt.Errorf("backup: read tar content %s: %w", hdr.Name, err)
		}
		if err := os.MkdirAll(filepath.Dir(cleaned), 0o750); err != nil {
			return nil, nil, fmt.Errorf("backup: create extract dir: %w", err)
		}
		if err := os.WriteFile(cleaned, data, 0o640); err != nil {
			return nil, nil, fmt.Errorf("backup: write extracted file %s: %w", hdr.Name, err)
		}
		if hdr.Name == manifestName {
			manifestJSON = data
			continue
		}
		entries = append(entries, fileEntry{Name: hdr.Name, Data: data})
	}
	return manifestJSON, entries, nil
}

// safeJoin joins base and name, rejecting any name that would escape base
// via ".." or an absolute path — a defense against a crafted archive
// performing path traversal on extraction.
func safeJoin(base, name string) (string, error) {
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("backup: unsafe archive entry path %q", name)
	}
	joined := filepath.Join(base, filepath.FromSlash(name))
	rel, err := filepath.Rel(base, joined)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("backup: unsafe archive entry path %q", name)
	}
	return joined, nil
}

// contentChecksum hashes entries in a deterministic, name-sorted order so
// the same file set always yields the same checksum regardless of the
// order they were collected in. This is the value stored as manifest.json's
// "checksum" field and the value verification recomputes from the
// extracted archive to detect corruption or tampering.
func contentChecksum(entries []fileEntry) string {
	sorted := make([]fileEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	h := sha256.New()
	for _, e := range sorted {
		h.Write([]byte(e.Name))
		h.Write([]byte{0})
		h.Write(e.Data)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// collectDir walks dir and returns one fileEntry per regular file found,
// with Name set to prefix + the path relative to dir (forward-slash
// separated, matching tar convention). It returns (nil, nil) if dir does
// not exist — the caller treats a missing optional directory as "not
// included" rather than an error, per AI.md PART 22's "if exists" columns.
func collectDir(dir, prefix string) ([]fileEntry, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("backup: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("backup: %s is not a directory", dir)
	}

	var entries []fileEntry
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, fileEntry{Name: prefix + filepath.ToSlash(rel), Data: data})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("backup: collect %s: %w", dir, walkErr)
	}
	return entries, nil
}
