package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/paths"
)

// Manifest is the exact manifest.json shape from AI.md PART 22.
type Manifest struct {
	Version          string    `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedBy        string    `json:"created_by"`
	AppVersion       string    `json:"app_version"`
	Contents         []string  `json:"contents"`
	Encrypted        bool      `json:"encrypted"`
	EncryptionMethod string    `json:"encryption_method,omitempty"`
	Checksum         string    `json:"checksum"`
}

// CollectOptions selects which optional content groups AI.md PART 22's
// "Backup Contents" table marks as flag-gated.
type CollectOptions struct {
	IncludeSSL  bool
	IncludeData bool
}

// collectEntries reads every file the AI.md PART 22 "Backup Contents" table
// lists into memory: server.yml and the two databases always, template/ and
// theme/ if present, ssl/ and data/ only when opted in. It returns the
// entries to archive plus the top-level "contents" summary manifest.json
// records (directory groups collapsed to one "name/" entry, matching the
// spec's example).
func collectEntries(p paths.Paths, opts CollectOptions) (entries []fileEntry, contents []string, err error) {
	cfgBytes, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		return nil, nil, fmt.Errorf("backup: read %s: %w", p.ConfigFile, err)
	}
	entries = append(entries, fileEntry{Name: "server.yml", Data: cfgBytes})
	contents = append(contents, "server.yml")

	serverDB, err := os.ReadFile(dbPath(p, database.ServerDBFile))
	if err != nil {
		return nil, nil, fmt.Errorf("backup: read %s: %w", database.ServerDBFile, err)
	}
	entries = append(entries, fileEntry{Name: database.ServerDBFile, Data: serverDB})
	contents = append(contents, database.ServerDBFile)

	usersDB, err := os.ReadFile(dbPath(p, database.UsersDBFile))
	if err != nil {
		return nil, nil, fmt.Errorf("backup: read %s: %w", database.UsersDBFile, err)
	}
	entries = append(entries, fileEntry{Name: database.UsersDBFile, Data: usersDB})
	contents = append(contents, database.UsersDBFile)

	templateDir := filepath.Join(p.Config, "template")
	tpl, err := collectDir(templateDir, "template/")
	if err != nil {
		return nil, nil, err
	}
	if len(tpl) > 0 {
		entries = append(entries, tpl...)
		contents = append(contents, "template/")
	}

	themeDir := filepath.Join(p.Config, "theme")
	theme, err := collectDir(themeDir, "theme/")
	if err != nil {
		return nil, nil, err
	}
	if len(theme) > 0 {
		entries = append(entries, theme...)
		contents = append(contents, "theme/")
	}

	if opts.IncludeSSL {
		ssl, err := collectDir(p.SSL, "ssl/")
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, ssl...)
		contents = append(contents, "ssl/")
	}

	if opts.IncludeData {
		data, err := collectDir(p.Data, "data/")
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, data...)
		contents = append(contents, "data/")
	}

	return entries, contents, nil
}

// dbPath resolves the on-disk path for one of the SQLite database files
// under paths.Paths.DB.
func dbPath(p paths.Paths, file string) string {
	if p.DB == "" {
		return file
	}
	return filepath.Join(p.DB, file)
}

// marshalManifest renders m as the exact indented JSON shape AI.md PART 22
// documents, with a trailing newline so the file inside the archive ends
// cleanly.
func marshalManifest(m Manifest) ([]byte, error) {
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("backup: marshal manifest: %w", err)
	}
	return append(out, '\n'), nil
}

// parseManifest parses manifest.json bytes extracted from an archive.
func parseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: unparsable manifest.json: %v", ErrVerificationFailed, err)
	}
	return m, nil
}
