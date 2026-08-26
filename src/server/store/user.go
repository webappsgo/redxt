package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// userColumns is the column list every user read shares, so a scan
// helper can be written once and reused by each lookup.
const userColumns = `id, username, email, email_verified, notification_email,
	password_hash, display_name, bio, location, website, avatar_url,
	timezone, language, role, visibility, org_visibility, status,
	failed_logins, locked_until, last_login_at, created_at, updated_at`

// scanUser reads one row in userColumns order. The timestamp columns are
// scanned into any because the supported drivers disagree on whether a
// TIMESTAMP arrives as a string or a time.Time.
func scanUser(scan func(...any) error) (model.User, error) {
	var (
		u                                     model.User
		emailVerified, orgVisibility, failedN int
		lockedUntil, lastLogin, created, upd  any
	)
	err := scan(
		&u.ID, &u.Username, &u.Email, &emailVerified, &u.NotificationEmail,
		&u.PasswordHash, &u.DisplayName, &u.Bio, &u.Location, &u.Website,
		&u.AvatarURL, &u.Timezone, &u.Language, &u.Role, &u.Visibility,
		&orgVisibility, &u.Status, &failedN, &lockedUntil, &lastLogin,
		&created, &upd,
	)
	if err != nil {
		return model.User{}, err
	}
	u.EmailVerified = emailVerified != 0
	u.OrgVisibility = orgVisibility != 0
	u.FailedLogins = failedN
	u.LockedUntil = database.ScanTime(lockedUntil)
	u.LastLoginAt = database.ScanTime(lastLogin)
	u.CreatedAt = database.ScanTime(created)
	u.UpdatedAt = database.ScanTime(upd)
	return u, nil
}

// CreateUser inserts a Regular User and returns the stored row.
//
// The caller supplies an already-hashed password; the store never sees
// a plaintext credential and never hashes one, so there is exactly one
// hashing path in the codebase.
func (s *Store) CreateUser(ctx context.Context, u model.User) (model.User, error) {
	ts := now()
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO users (username, email, email_verified, notification_email,
			password_hash, display_name, bio, location, website, avatar_url,
			timezone, language, role, visibility, org_visibility, status,
			created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, u.Email, boolInt(u.EmailVerified), u.NotificationEmail,
		u.PasswordHash, u.DisplayName, u.Bio, u.Location, u.Website,
		u.AvatarURL, u.Timezone, u.Language, u.Role, u.Visibility,
		boolInt(u.OrgVisibility), u.Status,
		database.FormatTime(ts), database.FormatTime(ts))
	if err != nil {
		if isUniqueViolation(err) {
			return model.User{}, ErrConflict
		}
		return model.User{}, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return s.UserByUsername(ctx, u.Username)
	}
	return s.UserByID(ctx, id)
}

// UserByID reads one user by primary key.
func (s *Store) UserByID(ctx context.Context, id int64) (model.User, error) {
	var u model.User
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			u, scanErr = scanUser(row.Scan)
			return scanErr
		},
		`SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	return u, notFound(err)
}

// UserByUsername reads one user by username.
func (s *Store) UserByUsername(ctx context.Context, username string) (model.User, error) {
	var u model.User
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			u, scanErr = scanUser(row.Scan)
			return scanErr
		},
		`SELECT `+userColumns+` FROM users WHERE username = ?`,
		strings.ToLower(strings.TrimSpace(username)))
	return u, notFound(err)
}

// UserByEmail reads one user by email address.
func (s *Store) UserByEmail(ctx context.Context, email string) (model.User, error) {
	var u model.User
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			u, scanErr = scanUser(row.Scan)
			return scanErr
		},
		`SELECT `+userColumns+` FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email)))
	return u, notFound(err)
}

// ListUsers returns a page of users ordered by id, for the admin panel.
func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]model.User, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutSimple,
		`SELECT `+userColumns+` FROM users ORDER BY id LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.User
	for rows.Next() {
		u, scanErr := scanUser(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers returns the total number of Regular User accounts.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM users`)
	return n, err
}

// UpdateProfile writes the editable profile and privacy fields.
func (s *Store) UpdateProfile(ctx context.Context, u model.User) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE users SET display_name = ?, bio = ?, location = ?, website = ?,
			avatar_url = ?, timezone = ?, language = ?, notification_email = ?,
			visibility = ?, org_visibility = ?, updated_at = ?
		 WHERE id = ?`,
		u.DisplayName, u.Bio, u.Location, u.Website, u.AvatarURL,
		u.Timezone, u.Language, u.NotificationEmail, u.Visibility,
		boolInt(u.OrgVisibility), database.FormatTime(now()), u.ID)
	return affected(res, err)
}

// UpdatePassword replaces the stored Argon2id hash and clears the
// lockout counters, because a successful password change is proof the
// account is back under its owner's control.
func (s *Store) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE users SET password_hash = ?, failed_logins = 0,
			locked_until = NULL, updated_at = ?
		 WHERE id = ?`,
		hash, database.FormatTime(now()), userID)
	return affected(res, err)
}

// UpdateStatus sets the account lifecycle status.
func (s *Store) UpdateStatus(ctx context.Context, userID int64, status string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE users SET status = ?, updated_at = ? WHERE id = ?`,
		status, database.FormatTime(now()), userID)
	return affected(res, err)
}

// MarkEmailVerified records a confirmed address.
func (s *Store) MarkEmailVerified(ctx context.Context, userID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?`,
		database.FormatTime(now()), userID)
	return affected(res, err)
}

// RecordLoginSuccess clears the failure counters and stamps the login
// time.
func (s *Store) RecordLoginSuccess(ctx context.Context, userID int64) error {
	ts := database.FormatTime(now())
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE users SET failed_logins = 0, locked_until = NULL,
			last_login_at = ?, updated_at = ?
		 WHERE id = ?`, ts, ts, userID)
	return affected(res, err)
}

// RecordLoginFailure increments the failure counter and applies a
// lockout once the configured threshold is reached.
func (s *Store) RecordLoginFailure(ctx context.Context, userID int64, max int, lockFor time.Duration) error {
	u, err := s.UserByID(ctx, userID)
	if err != nil {
		return err
	}

	failed := u.FailedLogins + 1
	locked := any(nil)
	if max > 0 && failed >= max {
		locked = database.FormatTime(now().Add(lockFor))
	}

	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE users SET failed_logins = ?, locked_until = ?, updated_at = ?
		 WHERE id = ?`,
		failed, locked, database.FormatTime(now()), userID)
	return affected(res, err)
}

// DeleteUser removes an account. The foreign keys cascade to sessions,
// preferences, memberships, and credentials, so no orphan row survives.
func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM users WHERE id = ?`, userID)
	return affected(res, err)
}

// affected turns a write that matched no row into ErrNotFound, so a
// caller can tell "updated nothing" from "updated something".
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
