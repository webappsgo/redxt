package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
)

// DefaultDiskThreshold is the disk-usage percentage a scheduled backup
// aborts above when server.backup.disk_threshold is left at zero.
const DefaultDiskThreshold = 90

// defaultCreatedBy names a scheduled backup's actor when the caller leaves
// CreateOptions/RunOptions.CreatedBy empty.
const defaultCreatedBy = "system"

// Service is the entry point for every operation this package exposes. The
// zero value is usable once Paths, Config, and Retention are set; Logger,
// Audit, Recorder, and Now are optional.
type Service struct {
	// Project is the app name used in every filename this package builds.
	// Empty resolves to paths.ProjectName().
	Project string
	Paths   paths.Paths
	Config  config.Backup
	// Retention is server.scheduler.tasks.backup_daily.retention, per
	// AI.md PART 22.
	Retention config.BackupRetention
	Logger    Logger
	Audit     AuditRecorder
	Recorder  BackupRecorder
	// Admin performs the credential clear RunSetup needs. Required only
	// for RunSetup; every other method ignores it.
	Admin AdminRecovery
	// Now overrides time.Now for tests. Nil uses the real clock.
	Now func() time.Time
}

func (s *Service) logger() Logger {
	if s.Logger == nil {
		return nopLogger{}
	}
	return s.Logger
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) project() string {
	if s.Project != "" {
		return s.Project
	}
	return paths.ProjectName()
}

func actorOr(actor string) string {
	if actor == "" {
		return defaultCreatedBy
	}
	return actor
}

// CreateOptions drives Service.CreateManual, the `--maintenance backup
// [filename]` command.
type CreateOptions struct {
	// Filename overrides the generated timestamped name. Empty uses
	// ManualFilename.
	Filename    string
	Password    string
	IncludeSSL  bool
	IncludeData bool
	CreatedBy   string
}

// RunOptions drives the two scheduler-facing entry points, RunDaily and
// RunHourly.
type RunOptions struct {
	Password    string
	IncludeSSL  bool
	IncludeData bool
	CreatedBy   string
}

// CreateManual implements `{project_name} --maintenance backup [filename]`:
// one verified, retention-counted backup file.
func (s *Service) CreateManual(opts CreateOptions) (*Result, error) {
	if s.Config.Compliance.Enabled && opts.Password == "" {
		s.audit(EventFailed, LevelError, actorOr(opts.CreatedBy), map[string]any{"error": ErrComplianceNoPassword.Error()})
		return nil, ErrComplianceNoPassword
	}

	encrypted := opts.Password != ""
	filename := opts.Filename
	if filename == "" {
		filename = ManualFilename(s.project(), s.now(), encrypted)
	}

	res, err := s.createAndVerify(filename, opts.Password, CollectOptions{
		IncludeSSL:  opts.IncludeSSL,
		IncludeData: opts.IncludeData,
	}, actorOr(opts.CreatedBy))
	if err != nil {
		return nil, err
	}

	if _, rerr := s.applyRetentionSweep(actorOr(opts.CreatedBy)); rerr != nil {
		s.logger().Warnf("backup: retention sweep after manual backup: %v", rerr)
	}
	return res, nil
}

// RunDaily implements the backup_daily task's 02:00 flow from AI.md PART
// 22 "Backup Creation Flow" in its exact numbered order.
func (s *Service) RunDaily(opts RunOptions) (*DailyResult, error) {
	actor := actorOr(opts.CreatedBy)
	collect := CollectOptions{IncludeSSL: opts.IncludeSSL, IncludeData: opts.IncludeData}

	// Step 1: retention sweep on the cached backup dir.
	deleted, err := s.applyRetentionSweep(actor)
	if err != nil {
		s.logger().Warnf("backup: startup retention sweep: %v", err)
	}

	// Step 2: disk-pressure guard.
	mostRecentSize := s.mostRecentFullSize()
	ok, free, usedPct, err := s.checkDiskSpace(mostRecentSize)
	if err != nil {
		s.logger().Warnf("backup: disk usage check: %v", err)
	} else if !ok {
		threshold := s.diskThreshold()
		s.audit(EventSkippedDiskFull, LevelError, actor, map[string]any{
			"free_bytes":   free,
			"used_percent": usedPct,
			"threshold":    threshold,
		})
		return nil, ErrDiskFull
	}

	result := &DailyResult{Deleted: deleted}

	// Steps 3-4: full backup.
	fullName := FullFilename(s.project(), s.now(), opts.Password != "")
	full, fullErr := s.createAndVerify(fullName, opts.Password, collect, actor)
	result.Full = full

	// Steps 5-6: daily incremental.
	dailyName := DailyIncrementalFilename(s.project(), opts.Password != "")
	daily, dailyErr := s.createAndVerify(dailyName, opts.Password, collect, actor)
	result.DailyIncremental = daily
	if dailyErr == nil && daily != nil {
		s.audit(EventDailyUpdated, LevelInfo, actor, map[string]any{"filename": daily.Filename})
	}

	// Step 7/8: only apply retention when both verifications passed.
	if fullErr == nil && dailyErr == nil {
		more, rerr := s.applyRetentionSweep(actor)
		if rerr != nil {
			s.logger().Warnf("backup: post-backup retention sweep: %v", rerr)
		}
		result.RetentionApplied = true
		result.Deleted = append(result.Deleted, more...)
		return result, nil
	}
	if fullErr != nil {
		return result, fullErr
	}
	return result, dailyErr
}

// RunHourly implements the optional backup_hourly task: create and verify
// the hourly incremental, then run the same retention/disk-pressure sweep
// AI.md PART 22 says runs "after every backup".
func (s *Service) RunHourly(opts RunOptions) (*Result, error) {
	actor := actorOr(opts.CreatedBy)
	collect := CollectOptions{IncludeSSL: opts.IncludeSSL, IncludeData: opts.IncludeData}

	name := HourlyIncrementalFilename(s.project(), opts.Password != "")
	res, err := s.createAndVerify(name, opts.Password, collect, actor)
	if err != nil {
		return nil, err
	}
	if _, rerr := s.applyRetentionSweep(actor); rerr != nil {
		s.logger().Warnf("backup: retention sweep after hourly backup: %v", rerr)
	}
	return res, nil
}

// create builds, optionally encrypts, and writes one backup file. It does
// not verify or record — createAndVerify wraps this with both.
func (s *Service) create(filename, password string, opts CollectOptions, createdBy string) (*Result, error) {
	if s.Config.Compliance.Enabled && password == "" {
		return nil, ErrComplianceNoPassword
	}

	entries, contents, err := collectEntries(s.Paths, opts)
	if err != nil {
		return nil, err
	}

	now := s.now()
	encrypted := password != ""
	manifest := Manifest{
		Version:    ManifestVersion,
		CreatedAt:  now,
		CreatedBy:  createdBy,
		AppVersion: version.Version(),
		Contents:   contents,
		Encrypted:  encrypted,
		Checksum:   contentChecksum(entries),
	}
	if encrypted {
		manifest.EncryptionMethod = EncryptionMethod
	}

	manifestJSON, err := marshalManifest(manifest)
	if err != nil {
		return nil, err
	}

	archive, err := buildArchive(entries, manifestJSON)
	if err != nil {
		return nil, err
	}

	final := archive
	if encrypted {
		final, err = encryptArchive(archive, password)
		if err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(s.Paths.Backup, 0o750); err != nil {
		return nil, fmt.Errorf("backup: create backup dir: %w", err)
	}
	dest := filepath.Join(s.Paths.Backup, filename)
	if err := os.WriteFile(dest, final, 0o640); err != nil {
		return nil, fmt.Errorf("backup: write %s: %w", filename, err)
	}

	return &Result{
		Filename:  filename,
		Path:      dest,
		SizeBytes: int64(len(final)),
		Checksum:  manifest.Checksum,
		Encrypted: encrypted,
		CreatedAt: now,
	}, nil
}

// createAndVerify runs create, then Verify, deleting the file and auditing
// backup.verification_failed on any check failure — never leaving a broken
// file behind and never deleting an existing valid backup as a side effect.
func (s *Service) createAndVerify(filename, password string, opts CollectOptions, createdBy string) (*Result, error) {
	res, err := s.create(filename, password, opts, createdBy)
	if err != nil {
		s.audit(EventFailed, LevelError, createdBy, map[string]any{"error": err.Error(), "filename": filename})
		return nil, err
	}

	report, verr := Verify(res.Path, password)
	if verr != nil {
		_ = os.Remove(res.Path)
		s.audit(EventVerificationFailed, LevelError, createdBy, map[string]any{
			"filename": res.Filename,
			"check":    report.FailedCheck,
		})
		return nil, verr
	}

	if s.Recorder != nil {
		if err := s.Recorder.RecordBackup(BackupRecord{
			Filename:    res.Filename,
			Destination: "local",
			SizeBytes:   res.SizeBytes,
			Checksum:    res.Checksum,
			Encrypted:   res.Encrypted,
			Status:      "complete",
			CreatedAt:   res.CreatedAt,
		}); err != nil {
			s.logger().Warnf("backup: record backup row for %s: %v", res.Filename, err)
		}
	}

	s.audit(EventCreated, LevelInfo, createdBy, map[string]any{
		"filename":   res.Filename,
		"size_bytes": res.SizeBytes,
		"encrypted":  res.Encrypted,
		"verified":   true,
		"created_by": createdBy,
	})
	return res, nil
}

// diskThreshold resolves server.backup.disk_threshold, defaulting to 90.
func (s *Service) diskThreshold() int {
	if s.Config.DiskThreshold <= 0 {
		return DefaultDiskThreshold
	}
	return s.Config.DiskThreshold
}

// mostRecentFullSize returns the size in bytes of the newest full or manual
// backup on disk, or 0 if none exists yet.
func (s *Service) mostRecentFullSize() int64 {
	files, err := ScanDir(s.Paths.Backup, s.project())
	if err != nil {
		return 0
	}
	var newest File
	var found bool
	for _, f := range files {
		if f.Class == classIncremental {
			continue
		}
		if !found || f.Date.After(newest.Date) {
			newest = f
			found = true
		}
	}
	if !found {
		return 0
	}
	return newest.Size
}

// checkDiskSpace implements AI.md PART 22 step 2: abort if free space is
// under 2x the most recent backup's size, or disk usage exceeds the
// configured threshold.
func (s *Service) checkDiskSpace(mostRecentSize int64) (ok bool, freeBytes uint64, usedPercent float64, err error) {
	total, free, err := diskUsage(s.Paths.Backup)
	if err != nil {
		return true, 0, 0, err
	}
	if total == 0 {
		return true, free, 0, nil
	}
	usedPercent = 100 * (1 - float64(free)/float64(total))
	threshold := float64(s.diskThreshold())

	if free < uint64(2*mostRecentSize) {
		return false, free, usedPercent, nil
	}
	if usedPercent > threshold {
		return false, free, usedPercent, nil
	}
	return true, free, usedPercent, nil
}

// applyRetentionSweep scans the backup dir, deletes whatever
// SelectForDeletion selects, and audits backup.deleted per file plus one
// backup.retention_cleanup summary if anything was removed.
func (s *Service) applyRetentionSweep(actor string) ([]string, error) {
	total, _, err := diskUsage(s.Paths.Backup)
	if err != nil {
		total = 0
	}

	files, err := ScanDir(s.Paths.Backup, s.project())
	if err != nil {
		return nil, err
	}

	capBytes, capEnabled, err := ParseSizeCap(s.Retention.MaxTotalSize, total)
	if err != nil {
		s.logger().Warnf("backup: %v", err)
	}

	toDelete := SelectForDeletion(files, s.Retention, capBytes, capEnabled)
	if len(toDelete) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(toDelete))
	for _, f := range toDelete {
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			s.logger().Warnf("backup: delete %s: %v", f.Name, err)
			continue
		}
		names = append(names, f.Name)
		s.audit(EventDeleted, LevelInfo, actor, map[string]any{"filename": f.Name})
	}

	remaining := len(files) - len(names)
	s.audit(EventRetentionCleanup, LevelInfo, actor, map[string]any{
		"deleted":         names,
		"reason":          "retention_policy",
		"remaining_count": remaining,
	})
	return names, nil
}
