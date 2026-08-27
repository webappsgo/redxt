package startup

import (
	"context"
	"encoding/json"
	"time"

	"github.com/webappsgo/redxt/src/backup"
	"github.com/webappsgo/redxt/src/database"
)

// startBackup builds the AI.md PART 22 service. It runs after the
// databases are open, because both the backups table recorder and the
// audit_log recorder need a handle, and before the scheduler, whose
// backup_daily and backup_hourly tasks drive it.
func (s *Server) startBackup() {
	s.Backup = &backup.Service{
		Paths:     s.Paths,
		Config:    s.Config.Server.Backup,
		Retention: s.Config.Server.Scheduler.Tasks["backup_daily"].Retention,
		Logger:    s.Log,
		Audit:     &dbAuditRecorder{db: s.ServerDB},
		Recorder:  &dbBackupRecorder{db: s.ServerDB},
	}
}

// runBackupDaily is the scheduler handler for PART 19's backup_daily
// task: a full backup, a replaced daily incremental, and the retention
// sweep, all under the configured encryption password.
func (s *Server) runBackupDaily(context.Context) error {
	if s.Backup == nil {
		return nil
	}
	_, err := s.Backup.RunDaily(s.backupRunOptions())
	return err
}

// runBackupHourly is the scheduler handler for PART 19's backup_hourly
// task: one always-replaced hourly incremental.
func (s *Server) runBackupHourly(context.Context) error {
	if s.Backup == nil {
		return nil
	}
	_, err := s.Backup.RunHourly(s.backupRunOptions())
	return err
}

// backupRunOptions builds the options both scheduled tasks share. A
// scheduled run cannot prompt, so the configured password is its only
// source; when compliance is on and none is set, the service refuses the
// run and audits the refusal rather than writing a plaintext archive.
func (s *Server) backupRunOptions() backup.RunOptions {
	return backup.RunOptions{
		Password:    s.Config.Server.Backup.Encryption.Password,
		IncludeSSL:  true,
		IncludeData: true,
		CreatedBy:   "scheduler",
	}
}

// dbBackupRecorder writes one row of the backups table for every file
// the service creates, which is what makes a backup visible to the admin
// panel instead of only existing on disk.
type dbBackupRecorder struct {
	db *database.DB
}

// RecordBackup implements backup.BackupRecorder.
func (r *dbBackupRecorder) RecordBackup(rec backup.BackupRecord) error {
	_, err := database.ExecContext(context.Background(), r.db, database.TimeoutWrite,
		`INSERT INTO backups (filename, destination, size_bytes, checksum, encrypted, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.Filename, rec.Destination, rec.SizeBytes, rec.Checksum,
		rec.Encrypted, rec.Status, rec.CreatedAt)
	return err
}

// dbAuditRecorder appends the backup service's events to audit_log, the
// append-only trail PART 22 requires so a skipped or failed backup can
// never pass unnoticed.
type dbAuditRecorder struct {
	db *database.DB
}

// RecordAudit implements backup.AuditRecorder. The details column is
// JSON, and an unmarshalable payload degrades to an empty object rather
// than losing the event itself.
func (r *dbAuditRecorder) RecordAudit(event, level, actor string, data map[string]any) error {
	details := []byte("{}")
	if len(data) > 0 {
		if encoded, err := json.Marshal(data); err == nil {
			details = encoded
		}
	}

	payload := map[string]any{"level": level, "data": json.RawMessage(details)}
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = details
	}

	_, err = database.ExecContext(context.Background(), r.db, database.TimeoutWrite,
		`INSERT INTO audit_log (event, actor_type, actor_id, target_type, details, created_at)
		 VALUES (?, 'system', ?, 'backup', ?, ?)`,
		event, actor, string(encoded), time.Now())
	return err
}
