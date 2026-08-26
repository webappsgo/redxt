package store

import (
	"context"
	"database/sql"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// inviteColumns is the column list shared by every invitation read.
const inviteColumns = `id, org_id, email, role, code_hash, max_uses, use_count,
	invited_by, accepted_at, created_at, expires_at`

// scanInvite reads one row in inviteColumns order.
func scanInvite(scan func(...any) error) (model.Invite, error) {
	var (
		i                   model.Invite
		orgID, invitedBy    sql.NullInt64
		accepted, created   any
		expires             any
		maxUses, useCount   int
		email, role, hashed string
	)
	err := scan(&i.ID, &orgID, &email, &role, &hashed, &maxUses, &useCount,
		&invitedBy, &accepted, &created, &expires)
	if err != nil {
		return model.Invite{}, err
	}
	i.OrgID = scanInt(orgID)
	i.InvitedBy = scanInt(invitedBy)
	i.Email = email
	i.Role = role
	i.CodeHash = hashed
	i.MaxUses = maxUses
	i.UseCount = useCount
	i.AcceptedAt = database.ScanTime(accepted)
	i.CreatedAt = database.ScanTime(created)
	i.ExpiresAt = database.ScanTime(expires)
	return i, nil
}

// CreateInvite stores an invitation by code hash and returns the row.
func (s *Store) CreateInvite(ctx context.Context, i model.Invite) (model.Invite, error) {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO invitations (org_id, email, role, code_hash, max_uses,
			use_count, invited_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		nullInt(i.OrgID), i.Email, i.Role, i.CodeHash, i.MaxUses,
		nullInt(i.InvitedBy), database.FormatTime(now()),
		database.FormatTime(i.ExpiresAt.UTC()))
	if err != nil {
		if isUniqueViolation(err) {
			return model.Invite{}, ErrConflict
		}
		return model.Invite{}, err
	}
	if id, idErr := res.LastInsertId(); idErr == nil {
		i.ID = id
	}
	i.UseCount = 0
	i.CreatedAt = now()
	return i, nil
}

// InviteByHash reads an invitation by the SHA-256 of its code.
func (s *Store) InviteByHash(ctx context.Context, hash string) (model.Invite, error) {
	var i model.Invite
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			i, scanErr = scanInvite(row.Scan)
			return scanErr
		},
		`SELECT `+inviteColumns+` FROM invitations WHERE code_hash = ?`, hash)
	return i, notFound(err)
}

// ListOrgInvites returns the invitations issued for one organization,
// newest first.
func (s *Store) ListOrgInvites(ctx context.Context, orgID int64) ([]model.Invite, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutSimple,
		`SELECT `+inviteColumns+` FROM invitations WHERE org_id = ?
		 ORDER BY id DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Invite
	for rows.Next() {
		i, scanErr := scanInvite(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListServerInvites returns the server-level registration invitations,
// which are the rows with no organization attached.
func (s *Store) ListServerInvites(ctx context.Context) ([]model.Invite, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutSimple,
		`SELECT `+inviteColumns+` FROM invitations WHERE org_id IS NULL
		 ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Invite
	for rows.Next() {
		i, scanErr := scanInvite(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// RedeemInvite counts one redemption against an invitation.
//
// The use_count guard is part of the statement rather than a prior read,
// so two registrations racing on the last remaining use of an invite
// cannot both succeed: only one of them updates a row. A max_uses of
// zero means unlimited, and the guard lets those rows through.
func (s *Store) RedeemInvite(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE invitations
		 SET use_count = use_count + 1, accepted_at = ?
		 WHERE id = ? AND expires_at > ?
		   AND (max_uses = 0 OR use_count < max_uses)`,
		database.FormatTime(now()), id, database.FormatTime(now()))
	return affected(res, err)
}

// DeleteInvite withdraws an invitation that has not been used.
func (s *Store) DeleteInvite(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM invitations WHERE id = ?`, id)
	return affected(res, err)
}

// PurgeExpiredInvites deletes invitations past their expiry, reporting
// how many went away.
func (s *Store) PurgeExpiredInvites(ctx context.Context) (int64, error) {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM invitations WHERE expires_at <= ?`,
		database.FormatTime(now()))
	if err != nil {
		return 0, err
	}
	n, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return 0, nil
	}
	return n, nil
}
