package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/ssl"
	"github.com/webappsgo/redxt/src/user"
)

// VerificationPrefix is the label the ownership TXT record is published
// under. Ownership is proved by a record only the domain's operator can
// publish; the CNAME or address record that later routes traffic is not
// proof, because anyone may point a name at this server.
const VerificationPrefix = "_verify."

// Verification failure codes reported to the caller.
const (
	// CodeTXTRecordMissing means the lookup succeeded but no published
	// value matched the expected token.
	CodeTXTRecordMissing = "TXT_RECORD_MISSING"
	// CodeDNSLookupFailed means the lookup itself did not complete, which
	// is a different problem from a wrong value and is retryable.
	CodeDNSLookupFailed = "DNS_LOOKUP_FAILED"
)

// VerificationError reports why a domain failed to verify.
type VerificationError struct {
	Code   string
	Domain string
	Detail string
}

// Error renders the failure for logs and API details.
func (e *VerificationError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Domain)
	}
	return fmt.Sprintf("%s: %s: %s", e.Code, e.Domain, e.Detail)
}

// Is lets a caller classify a verification failure with errors.Is while
// still reading the specific code off the value.
func (e *VerificationError) Is(target error) bool {
	return target == ErrValidation
}

// Resolver is the DNS lookup the verification flow needs. It is an
// interface so a test can supply published values without a network.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// SetResolver replaces the resolver used for ownership verification.
func (s *Service) SetResolver(r Resolver) {
	if r != nil {
		s.resolver = r
	}
}

// SetCertManager supplies the ACME manager used to issue certificates
// for verified custom domains. Issuance goes through the existing
// manager rather than a second TLS path, so every certificate this
// server holds is stored, renewed, and served the same way.
func (s *Service) SetCertManager(m *ssl.Manager) {
	s.certs = m
}

// domains returns the custom-domain configuration block.
func (s *Service) domainConfig() *domainPolicy {
	cfg := s.domains()
	return &domainPolicy{
		enabled:         cfg.Enabled,
		maxPerOrg:       cfg.MaxDomainsPerOrg,
		requireSSL:      cfg.RequireSSL,
		allowApex:       cfg.AllowApex,
		allowSubdomain:  cfg.AllowSubdomain,
		allowWildcard:   cfg.AllowWildcard,
		reserved:        cfg.Reserved,
		blockedPatterns: cfg.BlockedPatterns,
		renewalDays:     cfg.SSLRenewalDays,
	}
}

// domainPolicy is the resolved custom-domain policy for one request,
// pulled out of config once so each check below reads plainly.
type domainPolicy struct {
	enabled         bool
	maxPerOrg       int
	requireSSL      bool
	allowApex       bool
	allowSubdomain  bool
	allowWildcard   bool
	reserved        []string
	blockedPatterns []string
	renewalDays     int
}

// AddDomainInput is one custom domain being claimed.
type AddDomainInput struct {
	Domain  string
	Purpose string
}

// AddDomain records a custom domain for an organization in the pending,
// unverified state.
//
// Nothing is served for the domain until ownership is proved: the row is
// created so the verification token has somewhere to live, not so the
// domain becomes usable.
func (s *Service) AddDomain(ctx context.Context, orgID, actorID int64, in AddDomainInput) (model.Domain, error) {
	policy := s.domainConfig()
	if !policy.enabled {
		return model.Domain{}, ErrDisabled
	}

	if _, err := s.require(ctx, orgID, actorID, user.PermOrgSettings); err != nil {
		return model.Domain{}, err
	}

	name, wildcard, err := s.normalizeClaim(in.Domain, policy)
	if err != nil {
		return model.Domain{}, err
	}

	purpose := strings.TrimSpace(in.Purpose)
	if purpose == "" {
		purpose = model.PurposeUI
	}
	if !model.ValidPurpose(purpose) {
		return model.Domain{}, validationError("unknown custom domain purpose")
	}

	if policy.maxPerOrg > 0 {
		held, countErr := s.store.CountOrgDomains(ctx, orgID)
		if countErr != nil {
			return model.Domain{}, countErr
		}
		if held >= policy.maxPerOrg {
			return model.Domain{}, ErrQuotaExceeded
		}
	}

	token, err := security.RandomString(security.RandomLength)
	if err != nil {
		return model.Domain{}, err
	}

	domain, err := s.store.CreateDomain(ctx, model.Domain{
		OrgID:             orgID,
		Name:              name,
		Purpose:           purpose,
		IsApex:            user.IsApexDomain(name),
		IsWildcard:        wildcard,
		VerificationToken: token,
		SSLEnabled:        policy.requireSSL,
	})
	if err != nil {
		return model.Domain{}, mapStoreErr(err)
	}

	s.domainAudit(ctx, domain.ID, store.EventDomainAdded, actorID,
		map[string]any{"domain": name, "purpose": purpose})
	s.audit(ctx, orgID, store.EventDomainAdded, model.ActorUser, actorID, 0,
		map[string]any{"domain": name})

	return domain, nil
}

// normalizeClaim validates a requested domain against the operator's
// policy and returns the canonical name plus whether it is a wildcard.
func (s *Service) normalizeClaim(raw string, policy *domainPolicy) (string, bool, error) {
	candidate := strings.ToLower(strings.TrimSpace(raw))
	candidate = strings.TrimSuffix(candidate, ".")

	wildcard := user.IsWildcardDomain(candidate)
	if wildcard && !policy.allowWildcard {
		return "", false, validationError("wildcard domains are not accepted")
	}

	name, err := user.ValidateDomain(candidate)
	if err != nil {
		return "", false, validationError(err.Error())
	}

	apex := user.IsApexDomain(name)
	if apex && !policy.allowApex {
		return "", false, validationError("apex domains are not accepted")
	}
	if !apex && !wildcard && !policy.allowSubdomain {
		return "", false, validationError("subdomains are not accepted")
	}

	// A reserved name is one the operator serves themselves. Letting a
	// tenant claim it would let them intercept this server's own traffic.
	bare := strings.TrimPrefix(name, "*.")
	for _, reserved := range policy.reserved {
		reserved = strings.ToLower(strings.TrimSpace(reserved))
		if reserved == "" {
			continue
		}
		if bare == reserved || strings.HasSuffix(bare, "."+reserved) {
			return "", false, validationError("that domain is reserved")
		}
	}

	for _, pattern := range policy.blockedPatterns {
		re, compileErr := regexp.Compile(pattern)
		if compileErr != nil {
			continue
		}
		if re.MatchString(bare) {
			return "", false, validationError("that domain is not accepted")
		}
	}

	return name, wildcard, nil
}

// VerificationRecord describes the TXT record an operator must publish.
type VerificationRecord struct {
	Name  string
	Type  string
	Value string
}

// VerificationInstructions returns what to publish to prove ownership of
// a domain the caller's organization has claimed.
func (s *Service) VerificationInstructions(ctx context.Context, orgID, actorID, domainID int64) (VerificationRecord, error) {
	domain, err := s.domainForOrg(ctx, orgID, actorID, domainID, user.PermOrgSettings)
	if err != nil {
		return VerificationRecord{}, err
	}
	return VerificationRecord{
		Name:  VerificationPrefix + strings.TrimPrefix(domain.Name, "*."),
		Type:  "TXT",
		Value: domain.VerificationToken,
	}, nil
}

// VerifyDomain checks the ownership TXT record and, on success, puts the
// domain into service.
//
// Verification is never skipped and never assumed: activation happens
// only after a published record matches the token this server issued.
func (s *Service) VerifyDomain(ctx context.Context, orgID, actorID, domainID int64) (model.Domain, error) {
	domain, err := s.domainForOrg(ctx, orgID, actorID, domainID, user.PermOrgSettings)
	if err != nil {
		return model.Domain{}, err
	}
	if domain.Verified() {
		return domain, nil
	}

	if err = s.checkOwnership(ctx, domain); err != nil {
		if recordErr := s.store.RecordVerificationAttempt(ctx, domain.ID,
			model.VerificationFailed); recordErr != nil {
			return model.Domain{}, mapStoreErr(recordErr)
		}
		s.domainAudit(ctx, domain.ID, store.EventDomainFailed, actorID,
			map[string]any{"error": err.Error()})
		return model.Domain{}, err
	}

	if err = s.store.RecordVerificationAttempt(ctx, domain.ID,
		model.VerificationVerified); err != nil {
		return model.Domain{}, mapStoreErr(err)
	}
	if err = s.store.ActivateDomain(ctx, domain.ID); err != nil {
		return model.Domain{}, mapStoreErr(err)
	}

	s.domainAudit(ctx, domain.ID, store.EventDomainVerified, actorID, nil)
	s.audit(ctx, orgID, store.EventDomainActivated, model.ActorUser, actorID, 0,
		map[string]any{"domain": domain.Name})

	verified, err := s.store.DomainByID(ctx, domain.ID)
	if err != nil {
		return model.Domain{}, mapStoreErr(err)
	}

	// Certificate issuance is best-effort at this point: the domain is
	// proven and may be served over plain HTTP while the ACME order
	// completes, and a transient CA failure must not undo a verification
	// the operator has already satisfied. The failure is recorded on the
	// row so the scheduled renewal pass retries it.
	if verified.SSLEnabled {
		if issueErr := s.issueCertificate(ctx, verified); issueErr != nil {
			s.domainAudit(ctx, verified.ID, store.EventDomainSSLFailure, actorID,
				map[string]any{"error": issueErr.Error()})
		}
		return s.reloadDomain(ctx, verified.ID)
	}

	return verified, nil
}

// checkOwnership performs the TXT comparison.
func (s *Service) checkOwnership(ctx context.Context, domain model.Domain) error {
	name := VerificationPrefix + strings.TrimPrefix(domain.Name, "*.")

	values, err := s.resolver.LookupTXT(ctx, name)
	if err != nil {
		var dnsErr *net.DNSError
		// A name that does not exist yet is the ordinary case of an
		// operator who has not published the record, so it is reported as
		// a missing record rather than as a lookup failure the caller
		// cannot act on.
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return &VerificationError{Code: CodeTXTRecordMissing, Domain: name}
		}
		return &VerificationError{
			Code: CodeDNSLookupFailed, Domain: name, Detail: err.Error(),
		}
	}

	want := []byte(domain.VerificationToken)
	match := 0
	for _, value := range values {
		got := []byte(strings.TrimSpace(value))
		if len(got) != len(want) {
			continue
		}
		match |= subtle.ConstantTimeCompare(got, want)
	}
	if match != 1 {
		return &VerificationError{Code: CodeTXTRecordMissing, Domain: name}
	}
	return nil
}

// issueCertificate obtains a certificate through the server's existing
// ACME manager and records the outcome on the domain row.
func (s *Service) issueCertificate(ctx context.Context, domain model.Domain) error {
	if s.certs == nil {
		return errors.New("service: no certificate manager configured")
	}
	// A certificate is only ever requested for a proven domain. Asking a
	// CA to validate a name this server has no claim to would be an
	// abuse of the ACME account as well as a way to probe other people's
	// DNS.
	if !domain.Verified() {
		return ErrForbidden
	}

	updated := domain
	updated.SSLChallenge = s.certs.Challenge().String()

	cert, err := s.certs.IssueFor(ctx, strings.TrimPrefix(domain.Name, "*."))
	if err != nil {
		updated.SSLStatus = model.SSLFailed
		updated.SSLLastError = err.Error()
		if saveErr := s.store.SetDomainSSL(ctx, updated); saveErr != nil {
			return saveErr
		}
		return err
	}

	updated.SSLStatus = model.SSLIssued
	updated.SSLLastError = ""
	updated.SSLIssuedAt = cert.NotBefore
	updated.SSLExpiresAt = cert.NotAfter
	if err = s.store.SetDomainSSL(ctx, updated); err != nil {
		return mapStoreErr(err)
	}

	s.domainAudit(ctx, domain.ID, store.EventDomainSSLIssued, 0,
		map[string]any{"expires_at": cert.NotAfter.UTC().Format(time.RFC3339)})
	return nil
}

// RenewCertificates reissues certificates that are inside the renewal
// window. It is what the scheduler calls; it takes no caller identity
// because it acts for the server itself.
func (s *Service) RenewCertificates(ctx context.Context) (int, error) {
	policy := s.domainConfig()
	if !policy.enabled || s.certs == nil {
		return 0, nil
	}

	days := policy.renewalDays
	if days <= 0 {
		days = 30
	}
	cutoff := s.now().Add(time.Duration(days) * 24 * time.Hour)

	due, err := s.store.ListRenewals(ctx, cutoff)
	if err != nil {
		return 0, mapStoreErr(err)
	}

	renewed := 0
	for _, domain := range due {
		if issueErr := s.issueCertificate(ctx, domain); issueErr != nil {
			s.domainAudit(ctx, domain.ID, store.EventDomainSSLFailure, 0,
				map[string]any{"error": issueErr.Error()})
			continue
		}
		renewed++
	}
	return renewed, nil
}

// ListDomains returns an organization's custom domains.
func (s *Service) ListDomains(ctx context.Context, orgID, actorID int64) ([]model.Domain, error) {
	if _, err := s.require(ctx, orgID, actorID, user.PermRead); err != nil {
		return nil, err
	}
	domains, err := s.store.ListOrgDomains(ctx, orgID)
	return domains, mapStoreErr(err)
}

// SetDomainPurpose changes which surface a verified domain serves.
func (s *Service) SetDomainPurpose(ctx context.Context, orgID, actorID, domainID int64, purpose string) error {
	if !model.ValidPurpose(purpose) {
		return validationError("unknown custom domain purpose")
	}
	domain, err := s.domainForOrg(ctx, orgID, actorID, domainID, user.PermOrgSettings)
	if err != nil {
		return err
	}
	if err = s.store.UpdateDomainPurpose(ctx, domain.ID, purpose); err != nil {
		return mapStoreErr(err)
	}

	s.domainAudit(ctx, domain.ID, store.EventDomainAdded, actorID,
		map[string]any{"purpose": purpose})
	return nil
}

// RemoveDomain releases a custom domain.
func (s *Service) RemoveDomain(ctx context.Context, orgID, actorID, domainID int64) error {
	domain, err := s.domainForOrg(ctx, orgID, actorID, domainID, user.PermOrgSettings)
	if err != nil {
		return err
	}
	if err = s.store.DeleteDomain(ctx, domain.ID); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventDomainRemoved, model.ActorUser, actorID, 0,
		map[string]any{"domain": domain.Name})
	return nil
}

// SuspendDomain takes a domain out of service without releasing it.
func (s *Service) SuspendDomain(ctx context.Context, orgID, actorID, domainID int64, reason string) error {
	domain, err := s.domainForOrg(ctx, orgID, actorID, domainID, user.PermOrgSettings)
	if err != nil {
		return err
	}
	if err = s.store.SuspendDomain(ctx, domain.ID, reason); err != nil {
		return mapStoreErr(err)
	}

	s.domainAudit(ctx, domain.ID, store.EventDomainSuspended, actorID,
		map[string]any{"reason": reason})
	return nil
}

// ResolveServableDomain maps an incoming Host header to the domain that
// should answer it.
//
// Only an active, verified domain resolves. A pending or suspended
// record answers as unknown, which is what keeps an unverified claim
// from taking over a hostname.
func (s *Service) ResolveServableDomain(ctx context.Context, host string) (model.Domain, error) {
	name := strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(name); err == nil {
		name = h
	}
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return model.Domain{}, ErrNotFound
	}

	domain, err := s.store.DomainByName(ctx, name)
	if err != nil {
		return model.Domain{}, mapStoreErr(err)
	}
	if !domain.Servable() {
		return model.Domain{}, ErrNotFound
	}
	return domain, nil
}

// domainForOrg loads a domain and confirms it belongs to the caller's
// organization.
//
// The ownership test is on the loaded row rather than in the query, and
// a mismatch is reported as not found, so a caller cannot learn that a
// domain id exists in some other organization.
func (s *Service) domainForOrg(ctx context.Context, orgID, actorID, domainID int64, perm user.Permission) (model.Domain, error) {
	if _, err := s.require(ctx, orgID, actorID, perm); err != nil {
		return model.Domain{}, err
	}

	domain, err := s.store.DomainByID(ctx, domainID)
	if err != nil {
		return model.Domain{}, mapStoreErr(err)
	}
	if domain.OrgID != orgID {
		return model.Domain{}, ErrNotFound
	}
	return domain, nil
}

// reloadDomain re-reads a row after a write so the caller sees the
// stored state rather than a locally assembled copy of it.
func (s *Service) reloadDomain(ctx context.Context, id int64) (model.Domain, error) {
	domain, err := s.store.DomainByID(ctx, id)
	return domain, mapStoreErr(err)
}

// domainAudit records a custom-domain event. As with the organization
// trail, a logging failure does not undo the action it describes.
func (s *Service) domainAudit(ctx context.Context, domainID int64, event string, actorID int64, details map[string]any) {
	actorType := model.ActorUser
	if actorID == 0 {
		actorType = model.ActorSystem
	}

	_ = s.store.RecordDomainAudit(ctx, model.AuditEntry{
		SubjectID: domainID,
		Event:     event,
		ActorType: actorType,
		ActorID:   actorID,
		Details:   encodeDetails(details),
	})
}
