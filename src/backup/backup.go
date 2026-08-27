// Package backup implements AI.md PART 22: creating, verifying, retaining,
// and restoring redxt backups, plus the "--maintenance setup" admin recovery
// flow. The package is self-contained — it never imports src/scheduler and
// never registers scheduled tasks; the caller wires RunDaily/RunHourly into
// the scheduler and supplies the optional AuditRecorder/BackupRecorder to
// persist events into server.db.
package backup

import (
	"errors"
	"time"
)

// ManifestVersion is the manifest.json "version" field this package writes
// and the highest version it can restore.
const ManifestVersion = "1.0.0"

// EncryptionMethod is the manifest.json "encryption_method" field for every
// encrypted backup this package produces.
const EncryptionMethod = "AES-256-GCM"

var (
	// ErrComplianceNoPassword means server.backup.compliance.enabled is true
	// and no password was supplied, so the backup is blocked outright.
	ErrComplianceNoPassword = errors.New("backup: compliance mode requires an encryption password")
	// ErrPasswordRequired means the backup file is encrypted and no password
	// was supplied to open it.
	ErrPasswordRequired = errors.New("backup: encrypted backup requires a password")
	// ErrInvalidPassword means the supplied password could not decrypt the
	// backup — either it is wrong or the file is corrupt.
	ErrInvalidPassword = errors.New("backup: invalid backup password")
	// ErrVerificationFailed wraps the specific check that failed; see the
	// error's message for which one.
	ErrVerificationFailed = errors.New("backup: verification failed")
	// ErrDiskFull means the scheduled backup was skipped because free space
	// or disk usage crossed the configured threshold.
	ErrDiskFull = errors.New("backup: skipped, insufficient free space")
	// ErrNotAuthorized means the caller's restore/setup authorization did
	// not satisfy the PART 5 sensitive-operations matrix.
	ErrNotAuthorized = errors.New("backup: operation not authorized")
	// ErrBackupNotFound means the named backup file does not exist.
	ErrBackupNotFound = errors.New("backup: file not found")
)

// Logger is the narrow logging surface this package needs, matching the
// interface src/scheduler takes so the same adapter serves both.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// nopLogger discards everything. It is the zero-value fallback so a Service
// built without a Logger never nil-derefs.
type nopLogger struct{}

func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// AuditRecorder persists one audit_log row. Implementations typically wrap
// the caller's server.db handle. A Service with no AuditRecorder still logs
// every event through its Logger; it just never reaches audit_log.
type AuditRecorder interface {
	RecordAudit(event, level, actor string, data map[string]any) error
}

// BackupRecord is one row of the "backups" table schema already defined in
// src/database/schema_server.go.
type BackupRecord struct {
	Filename    string
	Destination string
	SizeBytes   int64
	Checksum    string
	Encrypted   bool
	Status      string
	CreatedAt   time.Time
}

// BackupRecorder persists one BackupRecord. Implementations typically wrap
// the caller's server.db handle and INSERT into the "backups" table.
type BackupRecorder interface {
	RecordBackup(rec BackupRecord) error
}

// Kind distinguishes the four backup shapes AI.md PART 22 names.
type Kind string

const (
	// KindManual is a `--maintenance backup [filename]` timestamped backup.
	KindManual Kind = "manual"
	// KindFull is the 02:00 backup_daily task's full backup.
	KindFull Kind = "full"
	// KindDailyIncremental is backup_daily's always-replaced second file.
	KindDailyIncremental Kind = "daily_incremental"
	// KindHourlyIncremental is backup_hourly's always-replaced file.
	KindHourlyIncremental Kind = "hourly_incremental"
)

// Result describes one backup file this package created and verified.
type Result struct {
	Filename  string
	Path      string
	SizeBytes int64
	Checksum  string
	Encrypted bool
	CreatedAt time.Time
}

// DailyResult is RunDaily's outcome: the full backup, the daily incremental,
// and whether retention actually ran (only true when both verified clean).
type DailyResult struct {
	Full             *Result
	DailyIncremental *Result
	RetentionApplied bool
	Deleted          []string
}
