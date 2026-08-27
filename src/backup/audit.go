package backup

// Audit event names from AI.md PART 22 "Audit Events". Every RecordAudit
// call this package makes uses one of these exact strings.
const (
	// EventCreated fires once a backup file is written and passes every
	// verification check.
	EventCreated = "backup.created"
	// EventRestored fires after a successful restore.
	EventRestored = "backup.restored"
	// EventDeleted fires for each backup file retention removes.
	EventDeleted = "backup.deleted"
	// EventFailed fires when backup creation itself errors out (not a
	// verification failure — see EventVerificationFailed for that).
	EventFailed = "backup.failed"
	// EventRetentionCleanup fires once per retention sweep that deletes at
	// least one file.
	EventRetentionCleanup = "backup.retention_cleanup"
	// EventVerificationFailed fires when a post-creation verification check
	// fails and the failed backup file is deleted.
	EventVerificationFailed = "backup.verification_failed"
	// EventDailyUpdated fires when the daily incremental is replaced.
	EventDailyUpdated = "backup.daily_updated"
	// EventSkippedDiskFull fires when a scheduled backup aborts because
	// free space or disk usage crossed the configured threshold.
	EventSkippedDiskFull = "backup.skipped_disk_full"
)

// Audit levels used with the events above.
const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// audit records one event through both the Logger and, if configured, the
// AuditRecorder. A RecordAudit failure is logged but never propagated — a
// broken audit sink must not abort a backup that otherwise succeeded.
func (s *Service) audit(event, level, actor string, data map[string]any) {
	switch level {
	case LevelWarn:
		s.logger().Warnf("backup: %s %v", event, data)
	case LevelError:
		s.logger().Errorf("backup: %s %v", event, data)
	default:
		s.logger().Infof("backup: %s %v", event, data)
	}
	if s.Audit == nil {
		return
	}
	if err := s.Audit.RecordAudit(event, level, actor, data); err != nil {
		s.logger().Warnf("backup: record audit event %s: %v", event, err)
	}
}
