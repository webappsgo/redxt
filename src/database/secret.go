package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrSecretNotFound reports that no row exists for the requested secret name.
var ErrSecretNotFound = errors.New("database: secret not found")

// ErrEmptySecret reports that a generator produced an empty value. An empty
// secret is never stored: it would silently disable whatever HMAC depends on
// it.
var ErrEmptySecret = errors.New("database: generated secret is empty")

// This file is the app_secrets ROW layer described in PART 11 "Cryptographic
// Keys": it reads and writes rows and nothing else. Decoding, HMAC, HKDF
// derivation, fingerprinting, and the rotation schedules are the security
// package's job; the split keeps the raw secret confined to the smallest
// possible surface.
//
// SECURITY, applying to every function here: a secret VALUE is never placed in
// an error message, never formatted into a log line, and never returned by an
// API. Errors below name the secret's row name only. A caller must observe the
// same rule with the value it receives.

// GetSecret returns the current version of a project-level secret.
//
// "Current" is the highest version present. During a rotation grace window the
// previous version is still in the table with an expires_at in the future;
// this returns the new one. A caller that must also accept the previous
// version reads it explicitly.
//
// The returned expiresAt is set only for a superseded row, so on the current
// version it is normally invalid.
//
// A missing name returns ErrSecretNotFound.
func GetSecret(ctx context.Context, db *DB, name string) (value string, version int, expiresAt sql.NullTime, err error) {
	const q = `SELECT value, version, expires_at
	           FROM app_secrets
	           WHERE name = ?
	           ORDER BY version DESC
	           LIMIT 1`
	// expires_at is scanned into an any and converted, because a driver may
	// return either the stored text or a parsed time.Time for a TIMESTAMP
	// column; scanning directly into a sql.NullTime works against only one of
	// those.
	var rawExpires any
	err = QueryRowContext(ctx, db, TimeoutSimple, func(row *sql.Row) error {
		return row.Scan(&value, &version, &rawExpires)
	}, q, name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", 0, sql.NullTime{}, fmt.Errorf("%w: %s", ErrSecretNotFound, name)
	case err != nil:
		return "", 0, sql.NullTime{}, fmt.Errorf("database: read secret %s: %w", name, err)
	}
	return value, version, nullTimeFrom(rawExpires), nil
}

// EnsureSecret returns the stored secret for name, generating and storing one
// on first use.
//
// RACE, and why the insert is written this way: PART 11 requires these secrets
// to exist before any user-visible operation, so every node runs this during
// startup. In a cluster sharing one database, several nodes can reach the
// "not present" branch at the same moment and each generate a different
// candidate value. If they each wrote unconditionally, the last writer would
// win and the earlier nodes would keep serving with a value the database no
// longer holds, invalidating every cookie and CSRF token they had already
// signed.
//
// The INSERT ... ON CONFLICT DO NOTHING plus re-read makes that impossible.
// The first writer's row stands, every later writer's insert is a silent
// no-op, and every node then re-reads and converges on the ONE stored value —
// including the node that generated the candidate that lost. The generated
// loser is discarded without ever being used.
//
// The generated value is written to the database and nothing else; it is not
// logged, and a generator error is wrapped without its output.
func EnsureSecret(ctx context.Context, db *DB, name string, generate func() (string, error)) (string, int, error) {
	value, version, _, err := GetSecret(ctx, db, name)
	switch {
	case err == nil:
		return value, version, nil
	case !errors.Is(err, ErrSecretNotFound):
		return "", 0, err
	}

	candidate, err := generate()
	if err != nil {
		return "", 0, fmt.Errorf("database: generate secret %s: %w", name, err)
	}
	if candidate == "" {
		return "", 0, fmt.Errorf("%w: %s", ErrEmptySecret, name)
	}

	const insert = `INSERT INTO app_secrets (name, version, value, created_at)
	                VALUES (?, 1, ?, ?)
	                ON CONFLICT (name, version) DO NOTHING`
	if _, err := ExecContext(ctx, db, TimeoutWrite, insert, name, candidate, FormatTime(time.Now())); err != nil {
		return "", 0, fmt.Errorf("database: store secret %s: %w", name, err)
	}

	value, version, _, err = GetSecret(ctx, db, name)
	if err != nil {
		return "", 0, err
	}
	return value, version, nil
}

// RotateSecret stores the next version of a secret and starts the grace window
// on the version it replaces.
//
// PART 11 rotation is add-only. The new row is inserted at version+1 and the
// previous row is left in place with expires_at set to now plus grace, so an
// HMAC that was in flight when the rotation happened still validates until the
// window closes. Nothing is deleted here, and nothing may delete a superseded
// row before its expires_at has passed.
//
// The read of the current version and both writes share one transaction, so
// two nodes rotating at the same moment cannot both claim version+1: the
// second one's insert conflicts on the primary key and the rotation fails
// rather than silently overwriting. That is the database-level half of the
// PART 10 anti-split-brain flow, whose other half — the advisory lock and the
// quorum check against cluster_nodes — belongs to the caller performing the
// rotation.
//
// A non-positive grace expires the previous version immediately, which is
// correct only for a compromise-driven rotation where the old value must stop
// being accepted at once.
func RotateSecret(ctx context.Context, db *DB, name string, generate func() (string, error), grace time.Duration) error {
	candidate, err := generate()
	if err != nil {
		return fmt.Errorf("database: generate secret %s: %w", name, err)
	}
	if candidate == "" {
		return fmt.Errorf("%w: %s", ErrEmptySecret, name)
	}

	now := time.Now()
	expiry := now.Add(grace)

	return WithTransaction(ctx, db, func(tx *sql.Tx) error {
		var current int
		err := tx.QueryRowContext(ctx,
			`SELECT version FROM app_secrets WHERE name = ? ORDER BY version DESC LIMIT 1`,
			name).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrSecretNotFound, name)
		}
		if err != nil {
			return fmt.Errorf("database: read secret %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO app_secrets (name, version, value, created_at) VALUES (?, ?, ?, ?)`,
			name, current+1, candidate, FormatTime(now)); err != nil {
			return fmt.Errorf("database: rotate secret %s: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE app_secrets SET expires_at = ? WHERE name = ? AND version = ?`,
			FormatTime(expiry), name, current); err != nil {
			return fmt.Errorf("database: expire secret %s: %w", name, err)
		}

		return nil
	})
}

// PruneSecrets deletes superseded secret versions whose grace window closed
// before now.
//
// This is the one deletion the app_secrets table permits, and it is
// deliberately narrow: a row is removed only when it is not the newest version
// for its name AND it carries an expires_at that has already passed. The
// current version has no expires_at and can never match, so a rotation that is
// still inside its window is untouched.
func PruneSecrets(ctx context.Context, db *DB, now time.Time) (int64, error) {
	const q = `DELETE FROM app_secrets
	           WHERE expires_at IS NOT NULL
	             AND expires_at < ?
	             AND version < (SELECT MAX(v.version) FROM app_secrets v WHERE v.name = app_secrets.name)`
	res, err := ExecContext(ctx, db, TimeoutWrite, q, FormatTime(now))
	if err != nil {
		return 0, fmt.Errorf("database: prune secrets: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("database: prune secrets: %w", err)
	}
	return n, nil
}
