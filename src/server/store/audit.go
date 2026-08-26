package store

import (
	"context"
	"database/sql"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// Organization audit events. Every membership and settings change is
// recorded under one of these names, so the trail can be filtered by
// what happened rather than by parsing free text.
const (
	EventOrgCreated       = "org.created"
	EventOrgUpdated       = "org.updated"
	EventOrgDeleted       = "org.deleted"
	EventOrgTransferred   = "org.transferred"
	EventMemberInvited    = "member.invited"
	EventMemberJoined     = "member.joined"
	EventMemberRoleSet    = "member.role_changed"
	EventMemberRemoved    = "member.removed"
	EventInviteRevoked    = "invite.revoked"
	EventTokenIssued      = "token.issued"
	EventTokenRevoked     = "token.revoked"
	EventZoneGranted      = "zone.granted"
	EventZoneRevoked      = "zone.revoked"
	EventDomainAdded      = "domain.added"
	EventDomainVerified   = "domain.verified"
	EventDomainFailed     = "domain.verification_failed"
	EventDomainActivated  = "domain.activated"
	EventDomainSuspended  = "domain.suspended"
	EventDomainRemoved    = "domain.removed"
	EventDomainSSLIssued  = "domain.ssl_issued"
	EventDomainSSLFailure = "domain.ssl_failed"
)

// RecordOrgAudit appends one organization audit row.
//
// Details carries a JSON object; the column defaults to an empty object,
// so an empty string is stored as "{}" rather than as text that no
// consumer can parse.
func (s *Store) RecordOrgAudit(ctx context.Context, e model.AuditEntry) error {
	details := e.Details
	if details == "" {
		details = "{}"
	}
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO organization_audit (org_id, event, actor_type, actor_id,
			target_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.SubjectID, e.Event, e.ActorType, nullInt(e.ActorID),
		nullInt(e.TargetID), details, database.FormatTime(now()))
	return err
}

// ListOrgAudit returns an organization's audit trail, newest first.
func (s *Store) ListOrgAudit(ctx context.Context, orgID int64, limit, offset int) ([]model.AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutComplex,
		`SELECT id, org_id, event, actor_type, actor_id, target_id, details,
			created_at
		 FROM organization_audit WHERE org_id = ?
		 ORDER BY id DESC LIMIT ? OFFSET ?`, orgID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.AuditEntry
	for rows.Next() {
		var (
			e               model.AuditEntry
			actor, target   sql.NullInt64
			createdTimeCell any
		)
		if scanErr := rows.Scan(&e.ID, &e.SubjectID, &e.Event, &e.ActorType,
			&actor, &target, &e.Details, &createdTimeCell); scanErr != nil {
			return nil, scanErr
		}
		e.ActorID = scanInt(actor)
		e.TargetID = scanInt(target)
		e.CreatedAt = database.ScanTime(createdTimeCell)
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountOrgAudit reports how many audit rows an organization has, for
// paging the trail.
func (s *Store) CountOrgAudit(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM organization_audit WHERE org_id = ?`, orgID)
	return n, err
}

// RecordDomainAudit appends one custom-domain audit row. The domain
// table has no target column, so AuditEntry.TargetID is not written here.
func (s *Store) RecordDomainAudit(ctx context.Context, e model.AuditEntry) error {
	details := e.Details
	if details == "" {
		details = "{}"
	}
	_, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO custom_domain_audit (domain_id, event, actor_type,
			actor_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.SubjectID, e.Event, e.ActorType, nullInt(e.ActorID), details,
		database.FormatTime(now()))
	return err
}

// ListDomainAudit returns one domain's history, newest first.
func (s *Store) ListDomainAudit(ctx context.Context, domainID int64, limit int) ([]model.AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutComplex,
		`SELECT id, domain_id, event, actor_type, actor_id, details, created_at
		 FROM custom_domain_audit WHERE domain_id = ?
		 ORDER BY id DESC LIMIT ?`, domainID, limit)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.AuditEntry
	for rows.Next() {
		var (
			e               model.AuditEntry
			actor           sql.NullInt64
			createdTimeCell any
		)
		if scanErr := rows.Scan(&e.ID, &e.SubjectID, &e.Event, &e.ActorType,
			&actor, &e.Details, &createdTimeCell); scanErr != nil {
			return nil, scanErr
		}
		e.ActorID = scanInt(actor)
		e.CreatedAt = database.ScanTime(createdTimeCell)
		out = append(out, e)
	}
	return out, rows.Err()
}
