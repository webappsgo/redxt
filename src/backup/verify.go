package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/webappsgo/redxt/src/database"

	// modernc.org/sqlite registers the "sqlite" database/sql driver used by
	// the database-integrity check below. redxt already depends on it via
	// src/database; blank-importing it here too keeps this package
	// independently buildable and testable.
	_ "modernc.org/sqlite"
)

// VerifyReport is the outcome of the AI.md PART 22 verification suite. Every
// check it lists is Fatal, so a non-nil error on Verify already means the
// backup is unusable; Report is returned alongside for logging which check
// passed how far.
type VerifyReport struct {
	// FailedCheck names the first check that failed, or "" if all passed.
	FailedCheck string
	Manifest    Manifest
	SizeBytes   int64
}

// checkFail is a small helper so each verification step can return a
// consistently formatted, wrapped error.
func checkFail(check string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrVerificationFailed, check, err)
}

// Verify runs every check from AI.md PART 22's verification table against
// the backup file at path, in order: file exists, size > 0, decrypt test
// (if the name ends .enc), checksum valid, manifest readable, content
// extraction, database integrity. It returns as soon as one check fails.
//
// password is ignored for an unencrypted backup and required for one whose
// name ends in ".enc".
func Verify(path, password string) (VerifyReport, error) {
	report := VerifyReport{FailedCheck: "file_exists"}

	info, err := os.Stat(path)
	if err != nil {
		return report, checkFail("file_exists", err)
	}
	report.FailedCheck = "size_nonzero"
	if info.Size() == 0 {
		return report, checkFail("size_nonzero", fmt.Errorf("backup file is empty"))
	}
	report.SizeBytes = info.Size()

	raw, err := os.ReadFile(path)
	if err != nil {
		return report, checkFail("file_exists", err)
	}

	encrypted := strings.HasSuffix(path, ".enc")
	archive := raw
	report.FailedCheck = "decrypt_test"
	if encrypted {
		if password == "" {
			return report, checkFail("decrypt_test", ErrPasswordRequired)
		}
		archive, err = decryptArchive(raw, password)
		if err != nil {
			return report, checkFail("decrypt_test", err)
		}
	}

	report.FailedCheck = "content_extraction"
	tmpDir, err := os.MkdirTemp("", "redxt-backup-verify-*")
	if err != nil {
		return report, checkFail("content_extraction", err)
	}
	defer os.RemoveAll(tmpDir)

	manifestJSON, entries, err := extractArchive(archive, tmpDir)
	if err != nil {
		return report, checkFail("content_extraction", err)
	}

	report.FailedCheck = "manifest_readable"
	if manifestJSON == nil {
		return report, checkFail("manifest_readable", fmt.Errorf("manifest.json missing from archive"))
	}
	manifest, err := parseManifest(manifestJSON)
	if err != nil {
		return report, checkFail("manifest_readable", err)
	}
	report.Manifest = manifest

	report.FailedCheck = "checksum_valid"
	if got := contentChecksum(entries); got != manifest.Checksum {
		return report, checkFail("checksum_valid", fmt.Errorf("computed %s, manifest says %s", got, manifest.Checksum))
	}

	report.FailedCheck = "database_integrity"
	for _, name := range []string{database.ServerDBFile, database.UsersDBFile} {
		if !hasEntry(entries, name) {
			continue
		}
		if err := checkSQLiteIntegrity(filepath.Join(tmpDir, name)); err != nil {
			return report, checkFail("database_integrity", fmt.Errorf("%s: %w", name, err))
		}
	}

	report.FailedCheck = ""
	return report, nil
}

func hasEntry(entries []fileEntry, name string) bool {
	for _, e := range entries {
		if e.Name == name {
			return true
		}
	}
	return false
}

// checkSQLiteIntegrity opens path with the pure-Go sqlite driver and runs
// PRAGMA integrity_check, the "Database integrity" Fatal check from AI.md
// PART 22's verification table.
func checkSQLiteIntegrity(path string) error {
	db, err := sql.Open(database.DriverSQLite, path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	row := db.QueryRow("PRAGMA integrity_check")
	var result string
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("integrity_check query: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check reported %q", result)
	}
	return nil
}
