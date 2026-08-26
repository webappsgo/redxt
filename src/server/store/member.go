package store

import (
	"context"
	"database/sql"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// Membership reads one user's role in one organization.
//
// Every org-scoped request resolves through this method. A user with no
// row here has no role, which is what makes an organization the caller
// does not belong to indistinguishable from one that does not exist.
func (s *Store) Membership(ctx context.Context, orgID, userID int64) (model.Member, error) {
	var (
		m       model.Member
		created any
	)
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			return row.Scan(&m.OrgID, &m.UserID, &m.Role, &created)
		},
		`SELECT org_id, user_id, role, created_at FROM organization_members
		 WHERE org_id = ? AND user_id = ?`, orgID, userID)
	if err != nil {
		return model.Member{}, notFound(err)
	}
	m.CreatedAt = database.ScanTime(created)
	return m, nil
}

// ListMembers returns every member of an organization with the account
// details the member list needs, ordered by role then username.
func (s *Store) ListMembers(ctx context.Context, orgID int64) ([]model.Member, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutComplex,
		`SELECT m.org_id, m.user_id, m.role, m.created_at, u.username, u.email
		 FROM organization_members m
		 JOIN users u ON u.id = m.user_id
		 WHERE m.org_id = ?
		 ORDER BY u.username`, orgID)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Member
	for rows.Next() {
		var (
			m       model.Member
			created any
		)
		if scanErr := rows.Scan(&m.OrgID, &m.UserID, &m.Role, &created,
			&m.Username, &m.Email); scanErr != nil {
			return nil, scanErr
		}
		m.CreatedAt = database.ScanTime(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMembers returns how many members an organization has.
func (s *Store) CountMembers(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM organization_members WHERE org_id = ?`, orgID)
	return n, err
}

// AddMember inserts a membership row.
func (s *Store) AddMember(ctx context.Context, orgID, userID int64, role string) error {
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO organization_members (org_id, user_id, role, created_at)
		 VALUES (?, ?, ?, ?)`,
		orgID, userID, role, database.FormatTime(now()))
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// SetMemberRole changes an existing member's role.
func (s *Store) SetMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE organization_members SET role = ? WHERE org_id = ? AND user_id = ?`,
		role, orgID, userID)
	return affected(res, err)
}

// RemoveMember deletes a membership, revokes the org tokens that member
// held, and drops their zone grants, in one transaction.
//
// Removing the membership alone would leave the user's org-scoped token
// working, which would let a removed member keep the access the removal
// was meant to end.
func (s *Store) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return database.WithTransaction(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM organization_members WHERE org_id = ? AND user_id = ?`,
			orgID, userID)
		if err != nil {
			return err
		}
		if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
			return ErrNotFound
		}

		if _, err = tx.ExecContext(ctx,
			`DELETE FROM zone_grants WHERE org_id = ? AND user_id = ?`,
			orgID, userID); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE api_tokens SET revoked_at = ?
			 WHERE org_id = ? AND owner_type = 'user' AND owner_id = ?
			   AND revoked_at IS NULL`,
			database.FormatTime(now()), orgID, userID)
		return err
	})
}

// ZoneGranted reports whether a member has an explicit grant on a zone.
// An Editor's authority is limited to granted zones, so a role check
// alone is not enough for a record write.
func (s *Store) ZoneGranted(ctx context.Context, orgID, userID, zoneID int64) (bool, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM zone_grants
		 WHERE org_id = ? AND user_id = ? AND zone_id = ?`,
		orgID, userID, zoneID)
	return n > 0, err
}

// GrantZone gives a member explicit authority over one zone.
func (s *Store) GrantZone(ctx context.Context, orgID, userID, zoneID int64, permission string) error {
	if err := s.RevokeZone(ctx, orgID, userID, zoneID); err != nil && err != ErrNotFound {
		return err
	}
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO zone_grants (org_id, user_id, zone_id, permission, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		orgID, userID, zoneID, permission, database.FormatTime(now()))
	return err
}

// RevokeZone removes a member's grant on one zone.
func (s *Store) RevokeZone(ctx context.Context, orgID, userID, zoneID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM zone_grants WHERE org_id = ? AND user_id = ? AND zone_id = ?`,
		orgID, userID, zoneID)
	return affected(res, err)
}
