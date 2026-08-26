package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
)

// domainColumns is the column list shared by every custom domain read.
const domainColumns = `id, org_id, domain, purpose, is_apex, is_wildcard,
	verification_status, verification_token, verified_at, last_check_at,
	check_count, ssl_enabled, ssl_status, ssl_challenge, ssl_issued_at,
	ssl_expires_at, ssl_last_error, status, suspended_reason,
	created_at, updated_at`

// scanDomain reads one row in domainColumns order.
func scanDomain(scan func(...any) error) (model.Domain, error) {
	var (
		d                          model.Domain
		apex, wildcard, sslEnabled int
		verified, checked, issued  any
		sslExpires, created, upd   any
	)
	err := scan(&d.ID, &d.OrgID, &d.Name, &d.Purpose, &apex, &wildcard,
		&d.VerificationStatus, &d.VerificationToken, &verified, &checked,
		&d.CheckCount, &sslEnabled, &d.SSLStatus, &d.SSLChallenge, &issued,
		&sslExpires, &d.SSLLastError, &d.Status, &d.SuspendReason,
		&created, &upd)
	if err != nil {
		return model.Domain{}, err
	}
	d.IsApex = apex != 0
	d.IsWildcard = wildcard != 0
	d.SSLEnabled = sslEnabled != 0
	d.VerifiedAt = database.ScanTime(verified)
	d.LastCheckAt = database.ScanTime(checked)
	d.SSLIssuedAt = database.ScanTime(issued)
	d.SSLExpiresAt = database.ScanTime(sslExpires)
	d.CreatedAt = database.ScanTime(created)
	d.UpdatedAt = database.ScanTime(upd)
	return d, nil
}

// normalizeDomain lowercases a hostname and drops the trailing dot, so
// the UNIQUE constraint on domain sees one spelling per name.
func normalizeDomain(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

// CreateDomain registers a custom domain in the pending state.
//
// The row is written unverified and inactive whatever the caller asked
// for: PART 36 requires proof of control before a domain serves
// anything, and refusing to accept a caller-supplied status here is what
// makes that impossible to skip.
func (s *Store) CreateDomain(ctx context.Context, d model.Domain) (model.Domain, error) {
	ts := database.FormatTime(now())

	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`INSERT INTO custom_domains (org_id, domain, purpose, is_apex,
			is_wildcard, verification_status, verification_token, check_count,
			ssl_enabled, ssl_status, ssl_challenge, ssl_last_error, status,
			suspended_reason, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, '', '', ?, '', ?, ?)`,
		d.OrgID, normalizeDomain(d.Name), d.Purpose, boolInt(d.IsApex),
		boolInt(d.IsWildcard), model.VerificationPending, d.VerificationToken,
		boolInt(d.SSLEnabled), model.SSLNone, model.DomainPending, ts, ts)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Domain{}, ErrConflict
		}
		return model.Domain{}, err
	}
	id, idErr := res.LastInsertId()
	if idErr != nil {
		return model.Domain{}, idErr
	}
	return s.DomainByID(ctx, id)
}

// DomainByID reads one custom domain by primary key.
func (s *Store) DomainByID(ctx context.Context, id int64) (model.Domain, error) {
	var d model.Domain
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			d, scanErr = scanDomain(row.Scan)
			return scanErr
		},
		`SELECT `+domainColumns+` FROM custom_domains WHERE id = ?`, id)
	return d, notFound(err)
}

// DomainByName reads one custom domain by hostname. This is the lookup
// the request router uses to decide which organization's surface a
// request for an unfamiliar Host header belongs to.
func (s *Store) DomainByName(ctx context.Context, name string) (model.Domain, error) {
	var d model.Domain
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error {
			var scanErr error
			d, scanErr = scanDomain(row.Scan)
			return scanErr
		},
		`SELECT `+domainColumns+` FROM custom_domains WHERE domain = ?`,
		normalizeDomain(name))
	return d, notFound(err)
}

// ListOrgDomains returns every domain an organization has registered.
func (s *Store) ListOrgDomains(ctx context.Context, orgID int64) ([]model.Domain, error) {
	return s.domainList(ctx,
		`SELECT `+domainColumns+` FROM custom_domains WHERE org_id = ?
		 ORDER BY domain`, orgID)
}

// ListServableDomains returns every verified, active domain, which is the
// set the TLS layer must be able to answer for.
func (s *Store) ListServableDomains(ctx context.Context) ([]model.Domain, error) {
	return s.domainList(ctx,
		`SELECT `+domainColumns+` FROM custom_domains
		 WHERE verification_status = ? AND status = ?
		 ORDER BY domain`, model.VerificationVerified, model.DomainActive)
}

// ListPendingVerification returns the domains still awaiting proof of
// control, oldest check first, which is the order the scheduler retries
// them in.
func (s *Store) ListPendingVerification(ctx context.Context, limit int) ([]model.Domain, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.domainList(ctx,
		`SELECT `+domainColumns+` FROM custom_domains
		 WHERE verification_status = ?
		 ORDER BY last_check_at ASC, id ASC
		 LIMIT ?`, model.VerificationPending, limit)
}

// ListRenewals returns the active domains whose certificate expires at or
// before the cutoff.
func (s *Store) ListRenewals(ctx context.Context, cutoff time.Time) ([]model.Domain, error) {
	return s.domainList(ctx,
		`SELECT `+domainColumns+` FROM custom_domains
		 WHERE ssl_enabled = 1 AND status = ?
		   AND ssl_expires_at IS NOT NULL AND ssl_expires_at <= ?
		 ORDER BY ssl_expires_at`,
		model.DomainActive, database.FormatTime(cutoff.UTC()))
}

// CountOrgDomains counts an organization's domains, which is the number
// the per-org quota applies to.
func (s *Store) CountOrgDomains(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := database.QueryRowContext(ctx, s.db, database.TimeoutSimple,
		func(row *sql.Row) error { return row.Scan(&n) },
		`SELECT COUNT(*) FROM custom_domains WHERE org_id = ?`, orgID)
	return n, err
}

// domainList runs a domain query and collects its rows.
func (s *Store) domainList(ctx context.Context, query string, args ...any) ([]model.Domain, error) {
	rows, cancel, err := database.QueryContext(ctx, s.db, database.TimeoutComplex,
		query, args...)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()

	var out []model.Domain
	for rows.Next() {
		d, scanErr := scanDomain(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDomainPurpose changes which surface a domain serves.
func (s *Store) UpdateDomainPurpose(ctx context.Context, id int64, purpose string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE custom_domains SET purpose = ?, updated_at = ? WHERE id = ?`,
		purpose, database.FormatTime(now()), id)
	return affected(res, err)
}

// RecordVerificationAttempt stores the outcome of one ownership check.
// A successful check stamps verified_at; a failed one only advances the
// counter, leaving the domain unverified and therefore unservable.
func (s *Store) RecordVerificationAttempt(ctx context.Context, id int64, status string) error {
	ts := database.FormatTime(now())

	var verifiedAt any
	if status == model.VerificationVerified {
		verifiedAt = ts
	}

	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE custom_domains
		 SET verification_status = ?, verified_at = ?, last_check_at = ?,
			check_count = check_count + 1, updated_at = ?
		 WHERE id = ?`,
		status, verifiedAt, ts, ts, id)
	return affected(res, err)
}

// ActivateDomain puts a verified domain into service.
//
// The verification_status guard is in the statement, so a domain that
// has not proven ownership cannot be activated even by a caller that
// skipped the service-layer check.
func (s *Store) ActivateDomain(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE custom_domains
		 SET status = ?, suspended_reason = '', updated_at = ?
		 WHERE id = ? AND verification_status = ?`,
		model.DomainActive, database.FormatTime(now()), id,
		model.VerificationVerified)
	return affected(res, err)
}

// SuspendDomain takes a domain out of service with a stated reason.
func (s *Store) SuspendDomain(ctx context.Context, id int64, reason string) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE custom_domains
		 SET status = ?, suspended_reason = ?, updated_at = ?
		 WHERE id = ?`,
		model.DomainSuspended, reason, database.FormatTime(now()), id)
	return affected(res, err)
}

// SetDomainSSL records the certificate state for a domain after an ACME
// attempt, whether it succeeded or failed.
func (s *Store) SetDomainSSL(ctx context.Context, d model.Domain) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`UPDATE custom_domains
		 SET ssl_enabled = ?, ssl_status = ?, ssl_challenge = ?,
			ssl_issued_at = ?, ssl_expires_at = ?, ssl_last_error = ?,
			updated_at = ?
		 WHERE id = ?`,
		boolInt(d.SSLEnabled), d.SSLStatus, d.SSLChallenge,
		nullTime(d.SSLIssuedAt), nullTime(d.SSLExpiresAt), d.SSLLastError,
		database.FormatTime(now()), d.ID)
	return affected(res, err)
}

// DeleteDomain removes a custom domain. Its audit rows cascade with it.
func (s *Store) DeleteDomain(ctx context.Context, id int64) error {
	res, err := database.ExecContext(ctx, s.db, database.TimeoutWrite,
		`DELETE FROM custom_domains WHERE id = ?`, id)
	return affected(res, err)
}
