package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/redxt/src/config"
)

func TestCreateManualVerifyRestoreUnencrypted(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{Project: "redxt", Paths: p}

	res, err := svc.CreateManual(CreateOptions{CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if res.Encrypted {
		t.Fatalf("expected unencrypted result")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}

	report, err := Verify(res.Path, "")
	if err != nil {
		t.Fatalf("Verify: %v (failed check %s)", err, report.FailedCheck)
	}

	restoreDir := t.TempDir()
	restoreP := p
	restoreP.Config = filepath.Join(restoreDir, "config")
	restoreP.ConfigFile = filepath.Join(restoreP.Config, "server.yml")
	restoreP.DB = filepath.Join(restoreDir, "db")
	restoreP.SSL = filepath.Join(restoreDir, "ssl")
	restoreP.Data = filepath.Join(restoreDir, "data")
	restoreSvc := &Service{Project: "redxt", Paths: restoreP}

	result, err := restoreSvc.Restore(RestoreOptions{
		BackupPath: res.Path,
		Auth:       RestoreAuthContext{DatabaseEmpty: true},
		RestoredBy: "tester",
	})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !result.NewServer {
		t.Fatalf("expected NewServer true for DatabaseEmpty restore")
	}
	if result.SetupToken == "" {
		t.Fatalf("expected a setup token on new-server restore")
	}
	got, err := os.ReadFile(restoreP.ConfigFile)
	if err != nil {
		t.Fatalf("restored server.yml missing: %v", err)
	}
	if string(got) != "port: 8080\n" {
		t.Fatalf("restored server.yml content = %q", got)
	}
}

func TestCreateManualEncryptedRoundTrip(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{Project: "redxt", Paths: p}

	res, err := svc.CreateManual(CreateOptions{Password: "correct-horse", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if !res.Encrypted {
		t.Fatalf("expected encrypted result")
	}

	if _, err := Verify(res.Path, "correct-horse"); err != nil {
		t.Fatalf("Verify with correct password: %v", err)
	}
}

func TestVerifyWrongPasswordFails(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{Project: "redxt", Paths: p}

	res, err := svc.CreateManual(CreateOptions{Password: "correct-horse", CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	report, err := Verify(res.Path, "wrong-password")
	if err == nil {
		t.Fatalf("expected an error for wrong password")
	}
	if report.FailedCheck != "decrypt_test" {
		t.Fatalf("FailedCheck = %q, want decrypt_test", report.FailedCheck)
	}
}

func TestCreateManualComplianceWithoutPasswordBlocked(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{
		Project: "redxt",
		Paths:   p,
		Config:  config.Backup{Compliance: config.BackupCompliance{Enabled: true}},
	}

	_, err := svc.CreateManual(CreateOptions{CreatedBy: "tester"})
	if err != ErrComplianceNoPassword {
		t.Fatalf("err = %v, want ErrComplianceNoPassword", err)
	}
	entries, err := os.ReadDir(p.Backup)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no backup file to be written, found %d", len(entries))
	}
}

func TestVerifyChecksumMismatchDetected(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{Project: "redxt", Paths: p}

	res, err := svc.CreateManual(CreateOptions{CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	raw, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	tmpDir := t.TempDir()
	_, entries, err := extractArchive(raw, tmpDir)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	manifest, err := parseManifest(mustReadManifest(t, raw))
	if err != nil {
		t.Fatalf("parseManifest: %v", err)
	}
	manifest.Checksum = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	badManifest, err := marshalManifest(manifest)
	if err != nil {
		t.Fatalf("marshalManifest: %v", err)
	}
	tampered, err := buildArchive(entries, badManifest)
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}
	if err := os.WriteFile(res.Path, tampered, 0o640); err != nil {
		t.Fatalf("write tampered archive: %v", err)
	}

	report, err := Verify(res.Path, "")
	if err == nil {
		t.Fatalf("expected checksum mismatch error")
	}
	if report.FailedCheck != "checksum_valid" {
		t.Fatalf("FailedCheck = %q, want checksum_valid", report.FailedCheck)
	}
}

// mustReadManifest re-extracts manifest.json from a known-good archive so
// TestVerifyChecksumMismatchDetected can build a tampered manifest from it.
func mustReadManifest(t *testing.T, archive []byte) []byte {
	t.Helper()
	manifestJSON, _, err := extractArchive(archive, t.TempDir())
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	return manifestJSON
}

func TestRunDailyProducesFullAndDailyFiles(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{Project: "redxt", Paths: p}

	result, err := svc.RunDaily(RunOptions{CreatedBy: "tester"})
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if result.Full == nil || result.DailyIncremental == nil {
		t.Fatalf("expected both Full and DailyIncremental results, got %+v", result)
	}
	if !result.RetentionApplied {
		t.Fatalf("expected RetentionApplied when both backups verify")
	}
}

func TestCheckDiskSpaceAbortsWhenRecentBackupTooLarge(t *testing.T) {
	p := newTestPaths(t)
	svc := &Service{Project: "redxt", Paths: p}

	ok, _, _, err := svc.checkDiskSpace(1 << 62)
	if err != nil {
		t.Fatalf("checkDiskSpace: %v", err)
	}
	if ok {
		t.Fatalf("expected checkDiskSpace to report not-ok for an implausibly large recent-backup size")
	}
}

func TestRunSetupClearsAdminCredentials(t *testing.T) {
	p := newTestPaths(t)
	admin := &fakeAdminRecovery{}
	svc := &Service{Project: "redxt", Paths: p, Admin: admin}

	token, err := svc.RunSetup(SetupAuthContext{IsRoot: true}, "operator")
	if err != nil {
		t.Fatalf("RunSetup: %v", err)
	}
	if token == "" {
		t.Fatalf("expected a non-empty setup token")
	}
	if !admin.cleared {
		t.Fatalf("expected ClearAdminCredentials to be called")
	}
}

func TestRunSetupDeniedWithoutAuthorization(t *testing.T) {
	p := newTestPaths(t)
	admin := &fakeAdminRecovery{}
	svc := &Service{Project: "redxt", Paths: p, Admin: admin}

	_, err := svc.RunSetup(SetupAuthContext{}, "operator")
	if err != ErrNotAuthorized {
		t.Fatalf("err = %v, want ErrNotAuthorized", err)
	}
	if admin.cleared {
		t.Fatalf("ClearAdminCredentials must not run when unauthorized")
	}
}

type fakeAdminRecovery struct {
	cleared bool
}

func (f *fakeAdminRecovery) ClearAdminCredentials() error {
	f.cleared = true
	return nil
}
