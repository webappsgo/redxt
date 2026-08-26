package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// Challenge is a single-use, hashed, expiring token: a password reset
// or an email confirmation. Both tables have the same shape, so both
// are read through this one type.
type Challenge struct {
	ID     int64
	UserID int64
	// Email is set only for an email confirmation, where the address
	// being confirmed may differ from the one currently on the account.
	Email     string
	UsedAt    time.Time
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Usable reports whether the challenge may still be redeemed.
func (c Challenge) Usable(at time.Time) bool {
	if !c.UsedAt.IsZero() {
		return false
	}
	return c.ExpiresAt.IsZero() || at.Before(c.ExpiresAt)
}

// CreatePasswordReset stores the hash of a reset token.
func (s *Store) CreatePasswordReset(ctx context.Context, userID int64, hash string, expires time.Time) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO password_resets (user_id, token_hash, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		userID, hash, database.FormatTime(now()),
		database.FormatTime(expires.UTC()))
	return err
}

// PasswordReset reads a reset challenge by token hash.
func (s *Store) PasswordReset(ctx context.Context, hash string) (Challenge, error) {
	var (
		c                  Challenge
		used, created, exp any
	)
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			return row.Scan(&c.ID, &c.UserID, &used, &created, &exp)
		},
		`SELECT id, user_id, used_at, created_at, expires_at
		 FROM password_resets WHERE token_hash = ?`, hash)
	if err != nil {
		return Challenge{}, notFound(err)
	}
	c.UsedAt = database.ScanTime(used)
	c.CreatedAt = database.ScanTime(created)
	c.ExpiresAt = database.ScanTime(exp)
	return c, nil
}

// ConsumePasswordReset marks a reset token used.
//
// The used_at IS NULL guard makes the update itself the single-use
// check: two requests racing with the same token both run this
// statement, and only the first affects a row.
func (s *Store) ConsumePasswordReset(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE password_resets SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		database.FormatTime(now()), id)
	return affected(res, err)
}

// DeleteUserPasswordResets invalidates every outstanding reset for a
// user, which a completed reset or a password change must do.
func (s *Store) DeleteUserPasswordResets(ctx context.Context, userID int64) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM password_resets WHERE user_id = ?`, userID)
	return err
}

// CreateEmailVerification stores the hash of a confirmation token for
// the given address.
func (s *Store) CreateEmailVerification(ctx context.Context, userID int64, email, hash string, expires time.Time) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO email_verifications (user_id, email, token_hash, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, email, hash, database.FormatTime(now()),
		database.FormatTime(expires.UTC()))
	return err
}

// EmailVerification reads a confirmation challenge by token hash.
func (s *Store) EmailVerification(ctx context.Context, hash string) (Challenge, error) {
	var (
		c                  Challenge
		used, created, exp any
	)
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			return row.Scan(&c.ID, &c.UserID, &c.Email, &used, &created, &exp)
		},
		`SELECT id, user_id, email, used_at, created_at, expires_at
		 FROM email_verifications WHERE token_hash = ?`, hash)
	if err != nil {
		return Challenge{}, notFound(err)
	}
	c.UsedAt = database.ScanTime(used)
	c.CreatedAt = database.ScanTime(created)
	c.ExpiresAt = database.ScanTime(exp)
	return c, nil
}

// ConsumeEmailVerification marks a confirmation token used, guarded the
// same way as a password reset.
func (s *Store) ConsumeEmailVerification(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE email_verifications SET used_at = ? WHERE id = ? AND used_at IS NULL`,
		database.FormatTime(now()), id)
	return affected(res, err)
}

// PurgeExpiredChallenges deletes spent and expired reset and
// confirmation rows, reporting how many went away.
func (s *Store) PurgeExpiredChallenges(ctx context.Context, cutoff time.Time) (int64, error) {
	stamp := database.FormatTime(cutoff.UTC())

	var total int64
	for _, table := range []string{"password_resets", "email_verifications"} {
		res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
			`DELETE FROM `+table+` WHERE expires_at <= ?`, stamp)
		if err != nil {
			return total, err
		}
		if n, rowsErr := res.RowsAffected(); rowsErr == nil {
			total += n
		}
	}
	return total, nil
}
