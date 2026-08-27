package backup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
)

// RestoreAuthContext is the caller-determined state the AI.md PART 22
// "Restore Authorization" matrix decides on. The caller is responsible for
// determining DatabaseEmpty, IsRoot, and IsServiceUser from the running
// process; AdminAuthenticated is true only once the caller has itself
// verified the supplied admin credentials.
type RestoreAuthContext struct {
	// DatabaseEmpty is true on a first-run server with nothing to protect.
	DatabaseEmpty bool
	// IsRoot is true when the calling process has root/Administrator
	// privileges.
	IsRoot bool
	// IsServiceUser is true when the calling process runs as the
	// dedicated service account (not root, not a random user).
	IsServiceUser bool
	// AdminAuthenticated is true once the caller has independently
	// verified the admin credentials a service-user restore requires.
	AdminAuthenticated bool
}

// AuthorizeRestore implements the AI.md PART 22 "Restore Authorization"
// table exactly: empty database or root is always allowed; a service user
// needs verified admin credentials; anyone else is denied.
func AuthorizeRestore(ctx RestoreAuthContext) error {
	switch {
	case ctx.DatabaseEmpty:
		return nil
	case ctx.IsRoot:
		return nil
	case ctx.IsServiceUser && ctx.AdminAuthenticated:
		return nil
	default:
		return ErrNotAuthorized
	}
}

// RestoreOptions drives Service.Restore.
type RestoreOptions struct {
	BackupPath string
	// Password is required when BackupPath ends in ".enc".
	Password   string
	Auth       RestoreAuthContext
	RestoredBy string
}

// RestoreResult reports what a successful restore did, including whether
// the Primary Admin must re-authenticate per AI.md PART 22 "Primary Admin
// Re-Setup on Restore" — true exactly when the restore happened onto an
// empty (i.e. new) server.
type RestoreResult struct {
	Manifest   Manifest
	NewServer  bool
	SetupToken string
}

// Restore implements `{project_name} --maintenance restore <backup-file>`.
// It runs the full AI.md PART 22 verification suite before touching
// anything — restore proceeds only if every check passes — then extracts
// each archived file to its real on-disk destination.
func (s *Service) Restore(opts RestoreOptions) (*RestoreResult, error) {
	if err := AuthorizeRestore(opts.Auth); err != nil {
		return nil, err
	}

	report, err := Verify(opts.BackupPath, opts.Password)
	if err != nil {
		return nil, err
	}

	appVer := version.Version()
	if report.Manifest.AppVersion != "" && report.Manifest.AppVersion != appVer {
		s.logger().Warnf("backup: restoring a backup created by app version %s into %s", report.Manifest.AppVersion, appVer)
	}

	raw, err := os.ReadFile(opts.BackupPath)
	if err != nil {
		return nil, err
	}
	archive := raw
	if strings.HasSuffix(opts.BackupPath, ".enc") {
		archive, err = decryptArchive(raw, opts.Password)
		if err != nil {
			return nil, err
		}
	}

	tmpDir, err := os.MkdirTemp("", "redxt-restore-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	_, entries, err := extractArchive(archive, tmpDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		dest := s.restoreDestination(e.Name)
		if dest == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dest, e.Data, 0o640); err != nil {
			return nil, err
		}
	}

	result := &RestoreResult{Manifest: report.Manifest, NewServer: opts.Auth.DatabaseEmpty}
	if result.NewServer {
		token, err := security.GenerateSetupToken()
		if err != nil {
			return nil, err
		}
		result.SetupToken = token
	}

	s.audit(EventRestored, LevelInfo, actorOr(opts.RestoredBy), map[string]any{
		"filename":    filepath.Base(opts.BackupPath),
		"restored_by": actorOr(opts.RestoredBy),
	})
	return result, nil
}

// restoreDestination maps one archive entry name back to its real
// filesystem path, mirroring collectEntries' naming exactly. An entry name
// this package never writes returns "" and is skipped rather than trusted.
func (s *Service) restoreDestination(name string) string {
	switch {
	case name == "server.yml":
		return s.Paths.ConfigFile
	case name == database.ServerDBFile:
		return dbPath(s.Paths, database.ServerDBFile)
	case name == database.UsersDBFile:
		return dbPath(s.Paths, database.UsersDBFile)
	case strings.HasPrefix(name, "template/"):
		return filepath.Join(s.Paths.Config, name)
	case strings.HasPrefix(name, "theme/"):
		return filepath.Join(s.Paths.Config, name)
	case strings.HasPrefix(name, "ssl/"):
		return filepath.Join(s.Paths.SSL, strings.TrimPrefix(name, "ssl/"))
	case strings.HasPrefix(name, "data/"):
		return filepath.Join(s.Paths.Data, strings.TrimPrefix(name, "data/"))
	default:
		return ""
	}
}
