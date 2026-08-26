package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// TOTP is one totp_secrets row. The seed is held encrypted because a
// second factor must be recoverable to be checked, unlike a password,
// which only ever needs to be verified.
type TOTP struct {
	UserID int64
	// SecretEncrypted is the AES-256-GCM ciphertext of the base32 seed,
	// tagged with the key version that produced it.
	SecretEncrypted string
	KeyVersion      int
	// Confirmed reports whether the user proved the enrollment by
	// entering a code. An unconfirmed row must not gate a sign-in.
	Confirmed bool
	// RecoveryCodes is a JSON array of hashed single-use codes.
	RecoveryCodes string
	CreatedAt     time.Time
}

// SaveTOTP writes an enrollment, replacing any earlier one. Update runs
// first so the primary key is not violated and no driver-specific upsert
// syntax is needed.
func (s *Store) SaveTOTP(ctx context.Context, t TOTP) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE totp_secrets SET secret_encrypted = ?, key_version = ?,
			confirmed = ?, recovery_codes = ?
		 WHERE user_id = ?`,
		t.SecretEncrypted, t.KeyVersion, boolInt(t.Confirmed),
		t.RecoveryCodes, t.UserID)
	if err != nil {
		return err
	}
	if n, rowsErr := res.RowsAffected(); rowsErr == nil && n > 0 {
		return nil
	}

	_, err = database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO totp_secrets (user_id, secret_encrypted, key_version,
			confirmed, recovery_codes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		t.UserID, t.SecretEncrypted, t.KeyVersion, boolInt(t.Confirmed),
		t.RecoveryCodes, database.FormatTime(now()))
	return err
}

// TOTPForUser reads a user's enrollment.
func (s *Store) TOTPForUser(ctx context.Context, userID int64) (TOTP, error) {
	var (
		t         TOTP
		confirmed int
		created   any
	)
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			return row.Scan(&t.UserID, &t.SecretEncrypted, &t.KeyVersion,
				&confirmed, &t.RecoveryCodes, &created)
		},
		`SELECT user_id, secret_encrypted, key_version, confirmed,
			recovery_codes, created_at
		 FROM totp_secrets WHERE user_id = ?`, userID)
	if err != nil {
		return TOTP{}, notFound(err)
	}
	t.Confirmed = confirmed != 0
	t.CreatedAt = database.ScanTime(created)
	return t, nil
}

// ConfirmTOTP marks an enrollment proven.
func (s *Store) ConfirmTOTP(ctx context.Context, userID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE totp_secrets SET confirmed = 1 WHERE user_id = ?`, userID)
	return affected(res, err)
}

// SetRecoveryCodes replaces the stored recovery code set, which
// consuming one code must do.
func (s *Store) SetRecoveryCodes(ctx context.Context, userID int64, codes string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE totp_secrets SET recovery_codes = ? WHERE user_id = ?`,
		codes, userID)
	return affected(res, err)
}

// DeleteTOTP removes an enrollment, disabling the second factor.
func (s *Store) DeleteTOTP(ctx context.Context, userID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM totp_secrets WHERE user_id = ?`, userID)
	return affected(res, err)
}
