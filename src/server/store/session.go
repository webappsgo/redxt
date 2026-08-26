package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// sessionColumns is the column list shared by every session read.
const sessionColumns = `id, session_hash, user_id, ip_address, user_agent,
	two_factor_ok, created_at, last_active_at, expires_at`

// scanSession reads one row in sessionColumns order.
func scanSession(scan func(...any) error) (model.Session, error) {
	var (
		s                        model.Session
		twoFactor                int
		created, lastActive, exp any
	)
	err := scan(&s.ID, &s.Hash, &s.UserID, &s.IP, &s.UserAgent,
		&twoFactor, &created, &lastActive, &exp)
	if err != nil {
		return model.Session{}, err
	}
	s.TwoFactorOK = twoFactor != 0
	s.CreatedAt = database.ScanTime(created)
	s.LastActive = database.ScanTime(lastActive)
	s.ExpiresAt = database.ScanTime(exp)
	return s, nil
}

// CreateSession stores a session keyed by the hash of the identifier
// handed to the browser. The identifier itself is never persisted, so a
// database read cannot be replayed as a logged-in browser.
func (s *Store) CreateSession(ctx context.Context, sess model.Session) (model.Session, error) {
	ts := now()
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO user_sessions (session_hash, user_id, ip_address,
			user_agent, two_factor_ok, created_at, last_active_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.Hash, sess.UserID, sess.IP, sess.UserAgent,
		boolInt(sess.TwoFactorOK), database.FormatTime(ts),
		database.FormatTime(ts), database.FormatTime(sess.ExpiresAt))
	if err != nil {
		return model.Session{}, err
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		sess.ID = id
	}
	sess.CreatedAt = ts
	sess.LastActive = ts
	return sess, nil
}

// SessionByHash reads a session by the hash of its identifier.
func (s *Store) SessionByHash(ctx context.Context, hash string) (model.Session, error) {
	var sess model.Session
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			sess, scanErr = scanSession(row.Scan)
			return scanErr
		},
		`SELECT `+sessionColumns+` FROM user_sessions WHERE session_hash = ?`, hash)
	return sess, notFound(err)
}

// ListSessions returns a user's live sessions, newest first, so the
// profile page can show where the account is signed in.
func (s *Store) ListSessions(ctx context.Context, userID int64) ([]model.Session, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutSimple,
		`SELECT `+sessionColumns+` FROM user_sessions
		 WHERE user_id = ? ORDER BY last_active_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Session
	for rows.Next() {
		sess, scanErr := scanSession(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// TouchSession stamps the last-active time on a live session.
func (s *Store) TouchSession(ctx context.Context, hash string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE user_sessions SET last_active_at = ? WHERE session_hash = ?`,
		database.FormatTime(now()), hash)
	return affected(res, err)
}

// MarkSessionTwoFactor records that the session has satisfied the
// second factor.
func (s *Store) MarkSessionTwoFactor(ctx context.Context, hash string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE user_sessions SET two_factor_ok = 1 WHERE session_hash = ?`, hash)
	return affected(res, err)
}

// DeleteSession ends one session.
func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM user_sessions WHERE session_hash = ?`, hash)
	return affected(res, err)
}

// DeleteUserSessions ends every session belonging to one user, which is
// what a password change and an account suspension both require.
func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM user_sessions WHERE user_id = ?`, userID)
	return err
}

// PurgeExpiredSessions removes sessions past their absolute lifetime
// and reports how many rows went away. The scheduler calls it; nothing
// depends on it having run, because every read also checks the expiry.
func (s *Store) PurgeExpiredSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM user_sessions WHERE expires_at <= ?`,
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
