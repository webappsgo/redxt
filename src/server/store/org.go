package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/user"
)

// orgColumns is the column list shared by every organization read.
const orgColumns = `id, slug, name, description, website, location, avatar_url,
	visibility, personal, owner_id, status, created_at, updated_at`

// scanOrg reads one row in orgColumns order.
func scanOrg(scan func(...any) error) (model.Org, error) {
	var (
		o             model.Org
		personal      int
		created, upd  any
		visibilityRaw string
	)
	err := scan(&o.ID, &o.Slug, &o.Name, &o.Description, &o.Website,
		&o.Location, &o.AvatarURL, &visibilityRaw, &personal, &o.OwnerID,
		&o.Status, &created, &upd)
	if err != nil {
		return model.Org{}, err
	}
	o.Visibility = visibilityRaw
	o.Personal = personal != 0
	o.CreatedAt = database.ScanTime(created)
	o.UpdatedAt = database.ScanTime(upd)
	return o, nil
}

// CreateOrg inserts an organization and makes its owner the first
// member, in one transaction. An organization without an owner row
// would be unreachable through every membership-scoped query, so the
// two writes must succeed or fail together.
func (s *Store) CreateOrg(ctx context.Context, o model.Org) (model.Org, error) {
	ts := database.FormatTime(now())

	var id int64
	err := database.WithTransaction(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO organizations (slug, name, description, website,
				location, avatar_url, visibility, personal, owner_id, status,
				created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			o.Slug, o.Name, o.Description, o.Website, o.Location, o.AvatarURL,
			o.Visibility, boolInt(o.Personal), o.OwnerID, o.Status, ts, ts)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO organization_members (org_id, user_id, role, created_at)
			 VALUES (?, ?, ?, ?)`,
			id, o.OwnerID, string(user.RoleOwner), ts)
		return err
	})
	if err != nil {
		if isUniqueViolation(err) {
			return model.Org{}, ErrConflict
		}
		return model.Org{}, err
	}

	return s.OrgByID(ctx, id)
}

// OrgByID reads one organization by primary key.
func (s *Store) OrgByID(ctx context.Context, id int64) (model.Org, error) {
	var o model.Org
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			o, scanErr = scanOrg(row.Scan)
			return scanErr
		},
		`SELECT `+orgColumns+` FROM organizations WHERE id = ?`, id)
	return o, notFound(err)
}

// OrgBySlug reads one organization by its vanity slug.
func (s *Store) OrgBySlug(ctx context.Context, slug string) (model.Org, error) {
	var o model.Org
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			o, scanErr = scanOrg(row.Scan)
			return scanErr
		},
		`SELECT `+orgColumns+` FROM organizations WHERE slug = ?`,
		strings.ToLower(strings.TrimSpace(slug)))
	return o, notFound(err)
}

// PersonalOrg reads the organization created automatically with a user
// account.
func (s *Store) PersonalOrg(ctx context.Context, userID int64) (model.Org, error) {
	var o model.Org
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			o, scanErr = scanOrg(row.Scan)
			return scanErr
		},
		`SELECT `+orgColumns+` FROM organizations
		 WHERE owner_id = ? AND personal = 1`, userID)
	return o, notFound(err)
}

// OrgsForUser returns every organization the user is a member of.
//
// This is the query that makes org scoping the default: a listing is
// derived from membership rather than filtered after the fact, so an
// organization the caller does not belong to never enters the result
// set to begin with.
func (s *Store) OrgsForUser(ctx context.Context, userID int64) ([]model.Org, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutComplex,
		`SELECT o.id, o.slug, o.name, o.description, o.website, o.location,
			o.avatar_url, o.visibility, o.personal, o.owner_id, o.status,
			o.created_at, o.updated_at
		 FROM organizations o
		 JOIN organization_members m ON m.org_id = o.id
		 WHERE m.user_id = ?
		 ORDER BY o.personal DESC, o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Org
	for rows.Next() {
		o, scanErr := scanOrg(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// CountOwnedOrgs counts the shared organizations a user owns. The
// personal organization is excluded because it is created by the server
// rather than by the user, so it does not consume their quota.
func (s *Store) CountOwnedOrgs(ctx context.Context, userID int64) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM organizations WHERE owner_id = ? AND personal = 0`,
		userID)
	return n, err
}

// UpdateOrg writes the editable organization profile fields.
func (s *Store) UpdateOrg(ctx context.Context, o model.Org) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE organizations SET name = ?, description = ?, website = ?,
			location = ?, avatar_url = ?, visibility = ?, updated_at = ?
		 WHERE id = ?`,
		o.Name, o.Description, o.Website, o.Location, o.AvatarURL,
		o.Visibility, database.FormatTime(now()), o.ID)
	return affected(res, err)
}

// TransferOrg hands ownership to another member, in one transaction:
// the new owner is promoted, the previous owner is demoted to admin,
// and the organization's owner_id is repointed. Doing this in three
// separate writes could leave the organization with two owners or none.
func (s *Store) TransferOrg(ctx context.Context, orgID, fromUserID, toUserID int64) error {
	ts := database.FormatTime(now())

	return database.WithTransaction(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE organization_members SET role = ?
			 WHERE org_id = ? AND user_id = ?`, string(user.RoleOwner), orgID, toUserID)
		if err != nil {
			return err
		}
		if n, rowsErr := res.RowsAffected(); rowsErr == nil && n == 0 {
			return ErrNotFound
		}

		if _, err = tx.ExecContext(ctx,
			`UPDATE organization_members SET role = ?
			 WHERE org_id = ? AND user_id = ?`,
			string(user.RoleAdmin), orgID, fromUserID); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`UPDATE organizations SET owner_id = ?, updated_at = ? WHERE id = ?`,
			toUserID, ts, orgID)
		return err
	})
}

// DeleteOrg removes an organization. Members, audit rows, zone grants,
// and custom domains cascade with it.
func (s *Store) DeleteOrg(ctx context.Context, orgID int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM organizations WHERE id = ?`, orgID)
	return affected(res, err)
}
