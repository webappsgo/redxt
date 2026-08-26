package admin

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// Store reads and writes the users.db admins and admin_sessions tables.
type Store struct {
	db *database.DB
}

// NewStore returns a Store backed by an open users.db handle.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

var (
	// ErrNotFound reports that no row matched.
	ErrNotFound = errors.New("admin: not found")
	// ErrConflict reports a uniqueness violation on username or email.
	ErrConflict = errors.New("admin: already exists")
)

// notFound maps a driver's no-rows error onto ErrNotFound and leaves
// every other error untouched.
func notFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) || database.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// now returns the current UTC time truncated to the second, which is
// the resolution the TIMESTAMP columns store.
func now() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

// boolInt converts a Go bool to the INTEGER the schema stores.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation reports whether err is a uniqueness failure. The
// supported drivers word this differently, so the check is on the
// message rather than on a driver-specific error type, which would
// otherwise force a build tag per driver.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unique constraint"):
		return true
	case strings.Contains(msg, "duplicate key"):
		return true
	case strings.Contains(msg, "duplicate entry"):
		return true
	case strings.Contains(msg, "constraint failed: unique"):
		return true
	case strings.Contains(msg, "violation of unique"):
		return true
	}
	return false
}

// adminColumns is the column list every admin read shares.
const adminColumns = `id, username, email, password_hash, must_change_pw,
	disabled, last_login_at, created_at, updated_at`

// scanAdmin reads one row in adminColumns order. The timestamp columns
// are scanned into any because the supported drivers disagree on
// whether a TIMESTAMP arrives as a string or a time.Time.
func scanAdmin(scan func(...any) error) (Admin, error) {
	var (
		a                       Admin
		mustChange, disabled    int
		lastLogin, created, upd any
	)
	err := scan(&a.ID, &a.Username, &a.Email, &a.PasswordHash,
		&mustChange, &disabled, &lastLogin, &created, &upd)
	if err != nil {
		return Admin{}, err
	}
	a.MustChangePassword = mustChange != 0
	a.Disabled = disabled != 0
	a.LastLoginAt = database.ScanTime(lastLogin)
	a.CreatedAt = database.ScanTime(created)
	a.UpdatedAt = database.ScanTime(upd)
	return a, nil
}

// CreateAdmin inserts a Server Admin and returns the stored row.
//
// The caller supplies an already-hashed password; the store never sees
// a plaintext credential and never hashes one, so there is exactly one
// hashing path in the codebase.
func (s *Store) CreateAdmin(ctx context.Context, a Admin) (Admin, error) {
	ts := now()
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO admins (username, email, password_hash, must_change_pw,
			disabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.Username, a.Email, a.PasswordHash, boolInt(a.MustChangePassword),
		boolInt(a.Disabled), database.FormatTime(ts), database.FormatTime(ts))
	if err != nil {
		if isUniqueViolation(err) {
			return Admin{}, ErrConflict
		}
		return Admin{}, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return s.AdminByUsername(ctx, a.Username)
	}
	return s.AdminByID(ctx, id)
}

// AdminByID reads one admin by primary key.
func (s *Store) AdminByID(ctx context.Context, id int64) (Admin, error) {
	var a Admin
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			a, scanErr = scanAdmin(row.Scan)
			return scanErr
		},
		`SELECT `+adminColumns+` FROM admins WHERE id = ?`, id)
	return a, notFound(err)
}

// AdminByUsername reads one admin by username.
func (s *Store) AdminByUsername(ctx context.Context, username string) (Admin, error) {
	var a Admin
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			a, scanErr = scanAdmin(row.Scan)
			return scanErr
		},
		`SELECT `+adminColumns+` FROM admins WHERE username = ?`, username)
	return a, notFound(err)
}

// CountAdmins returns the total number of Server Admin accounts. The
// setup wizard uses it to decide whether first-run setup is still
// pending: any count greater than zero means setup already happened.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM admins`)
	return n, err
}

// RecordLoginSuccess stamps the login time. Brute-force protection on
// admin login happens at the PART 11 per-IP rate-limit middleware, so
// the admins table carries no per-account lockout counters.
func (s *Store) RecordLoginSuccess(ctx context.Context, adminID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE admins SET last_login_at = ?, updated_at = ? WHERE id = ?`,
		database.FormatTime(now()), database.FormatTime(now()), adminID)
	return affected(res, err)
}

// affected turns a write that matched no row into ErrNotFound.
func affected(res sql.Result, err error) error {
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// sessionColumns is the column list shared by every session read.
const sessionColumns = `id, session_hash, admin_id, ip_address, user_agent,
	two_factor_ok, created_at, last_active_at, expires_at`

// scanSession reads one row in sessionColumns order.
func scanSession(scan func(...any) error) (Session, error) {
	var (
		sess                     Session
		twoFactor                int
		created, lastActive, exp any
	)
	err := scan(&sess.ID, &sess.Hash, &sess.AdminID, &sess.IP, &sess.UserAgent,
		&twoFactor, &created, &lastActive, &exp)
	if err != nil {
		return Session{}, err
	}
	sess.TwoFactorOK = twoFactor != 0
	sess.CreatedAt = database.ScanTime(created)
	sess.LastActive = database.ScanTime(lastActive)
	sess.ExpiresAt = database.ScanTime(exp)
	return sess, nil
}

// CreateSession stores a session keyed by the hash of the identifier
// handed to the browser. The identifier itself is never persisted, so a
// database read cannot be replayed as a logged-in browser.
func (s *Store) CreateSession(ctx context.Context, sess Session) (Session, error) {
	ts := now()
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO admin_sessions (session_hash, admin_id, ip_address,
			user_agent, two_factor_ok, created_at, last_active_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.Hash, sess.AdminID, sess.IP, sess.UserAgent,
		boolInt(sess.TwoFactorOK), database.FormatTime(ts),
		database.FormatTime(ts), database.FormatTime(sess.ExpiresAt))
	if err != nil {
		return Session{}, err
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		sess.ID = id
	}
	sess.CreatedAt = ts
	sess.LastActive = ts
	return sess, nil
}

// SessionByHash reads a session by the hash of its identifier.
func (s *Store) SessionByHash(ctx context.Context, hash string) (Session, error) {
	var sess Session
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			sess, scanErr = scanSession(row.Scan)
			return scanErr
		},
		`SELECT `+sessionColumns+` FROM admin_sessions WHERE session_hash = ?`, hash)
	return sess, notFound(err)
}

// TouchSession stamps the last-active time on a live session.
func (s *Store) TouchSession(ctx context.Context, hash string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE admin_sessions SET last_active_at = ? WHERE session_hash = ?`,
		database.FormatTime(now()), hash)
	return affected(res, err)
}

// DeleteSession ends one session.
func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM admin_sessions WHERE session_hash = ?`, hash)
	return affected(res, err)
}

// PurgeExpiredSessions removes sessions past their absolute lifetime and
// reports how many rows went away. The scheduler calls it; nothing
// depends on it having run, because every read also checks the expiry.
func (s *Store) PurgeExpiredSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM admin_sessions WHERE expires_at <= ?`,
		database.FormatTime(cutoff.UTC()))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return n, nil
}
