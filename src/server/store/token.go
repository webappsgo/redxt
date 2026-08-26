package store

import (
	"context"
	"database/sql"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
)

// tokenColumns is the column list shared by every token read. The token
// secret is not among them, because it is not stored.
const tokenColumns = `id, owner_type, owner_id, name, token_hash, token_prefix,
	scope, role, org_id, zone_id, capability, last_used_at, revoked_at,
	expires_at, created_at`

// scanToken reads one row in tokenColumns order.
func scanToken(scan func(...any) error) (model.Token, error) {
	var (
		t                              model.Token
		lastUsed, revoked, exp, create any
	)
	err := scan(&t.ID, &t.OwnerType, &t.OwnerID, &t.Name, &t.Hash, &t.Prefix,
		&t.Scope, &t.Role, &t.OrgID, &t.ZoneID, &t.Capability,
		&lastUsed, &revoked, &exp, &create)
	if err != nil {
		return model.Token{}, err
	}
	t.LastUsedAt = database.ScanTime(lastUsed)
	t.RevokedAt = database.ScanTime(revoked)
	t.ExpiresAt = database.ScanTime(exp)
	t.CreatedAt = database.ScanTime(create)
	return t, nil
}

// CreateToken stores an API token by hash and returns the stored row.
func (s *Store) CreateToken(ctx context.Context, t model.Token) (model.Token, error) {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO api_tokens (owner_type, owner_id, name, token_hash,
			token_prefix, scope, role, org_id, zone_id, capability,
			expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.OwnerType, t.OwnerID, t.Name, t.Hash, t.Prefix, t.Scope, t.Role,
		t.OrgID, t.ZoneID, t.Capability, nullTime(t.ExpiresAt),
		database.FormatTime(now()))
	if err != nil {
		if isUniqueViolation(err) {
			return model.Token{}, ErrConflict
		}
		return model.Token{}, err
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		t.ID = id
	}
	t.CreatedAt = now()
	return t, nil
}

// TokenByHash reads a token by the SHA-256 of its secret.
func (s *Store) TokenByHash(ctx context.Context, hash string) (model.Token, error) {
	var t model.Token
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			t, scanErr = scanToken(row.Scan)
			return scanErr
		},
		`SELECT `+tokenColumns+` FROM api_tokens WHERE token_hash = ?`, hash)
	return t, notFound(err)
}

// TokenByID reads one token row by primary key.
func (s *Store) TokenByID(ctx context.Context, id int64) (model.Token, error) {
	var t model.Token
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			t, scanErr = scanToken(row.Scan)
			return scanErr
		},
		`SELECT `+tokenColumns+` FROM api_tokens WHERE id = ?`, id)
	return t, notFound(err)
}

// ListTokens returns every token belonging to one owner, newest first.
func (s *Store) ListTokens(ctx context.Context, ownerType string, ownerID int64) ([]model.Token, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutSimple,
		`SELECT `+tokenColumns+` FROM api_tokens
		 WHERE owner_type = ? AND owner_id = ? ORDER BY id DESC`,
		ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Token
	for rows.Next() {
		t, scanErr := scanToken(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountLiveTokens counts an owner's tokens that are neither revoked nor
// expired, which is the number the per-user cap applies to.
func (s *Store) CountLiveTokens(ctx context.Context, ownerType string, ownerID int64) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM api_tokens
		 WHERE owner_type = ? AND owner_id = ? AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > ?)`,
		ownerType, ownerID, database.FormatTime(now()))
	return n, err
}

// TouchToken stamps the last-used time. A failure here is reported but
// never blocks the request that used the token: usage tracking is
// bookkeeping, not authorization.
func (s *Store) TouchToken(ctx context.Context, id int64) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`,
		database.FormatTime(now()), id)
	return err
}

// RevokeToken marks a token unusable without deleting the row, so an
// audit trail of what the credential did survives its revocation.
func (s *Store) RevokeToken(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		database.FormatTime(now()), id)
	return affected(res, err)
}

// RevokeOrgTokensForUser revokes every token a user holds against one
// organization. Removing a member must take their credentials with
// them, or the membership check is bypassed by the token they kept.
func (s *Store) RevokeOrgTokensForUser(ctx context.Context, orgID, userID int64) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE api_tokens SET revoked_at = ?
		 WHERE org_id = ? AND owner_type = ? AND owner_id = ? AND revoked_at IS NULL`,
		database.FormatTime(now()), orgID, string(security.OwnerUser), userID)
	return err
}

// DeleteToken removes a token row outright, for an owner who wants the
// record gone rather than kept as revoked.
func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM api_tokens WHERE id = ?`, id)
	return affected(res, err)
}
