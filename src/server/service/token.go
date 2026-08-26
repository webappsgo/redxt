package service

import (
	"context"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/user"
)

// IssueTokenInput is one API token being created.
//
// The narrowing fields are the PART 35 credential-scoping rule: a token
// may cover one organization, one zone inside it, or one capability, and
// never more than the role of the member who issued it.
type IssueTokenInput struct {
	Name string
	// OrgID is the organization the token acts inside. It is required
	// for a user-issued token, because every zone belongs to an
	// organization and a credential with no organization would have
	// nothing legitimate to reach.
	OrgID int64
	// ZoneID narrows the token to a single zone. Zero leaves the role's
	// own reach in place.
	ZoneID int64
	// Capability narrows the token to one action, for example
	// records:write or acme-challenge.
	Capability string
	// Role is the requested organization role. It is capped to the
	// issuer's own role, never raised to it.
	Role string
	// Scope is the read/read-write breadth recorded alongside the role.
	Scope string
	// ExpiresIn is how long the token lives. Zero uses the configured
	// default, which is the usual case.
	ExpiresIn time.Duration
}

// IssuedToken carries a new token and its plaintext secret. The secret
// exists only in this value: it is shown to the caller once and never
// recoverable afterwards, because only its SHA-256 is stored.
type IssuedToken struct {
	Token  model.Token
	Secret string
}

// Capabilities that are not organization permissions but are still valid
// narrowings for a token.
//
// The schema's capability column deliberately accepts any name so that
// future DNS credential kinds (TSIG, GSS-TSIG, DDNS host, agent tokens)
// can record their own narrowing without a migration. Only the two below
// are recognized today; none of the others are implemented.
const (
	// CapabilityACMEChallenge lets a token write only the TXT record an
	// ACME DNS-01 challenge needs.
	CapabilityACMEChallenge = "acme-challenge"
	// CapabilityMetricsRead lets a token read org-scoped metrics and
	// nothing else.
	CapabilityMetricsRead = "metrics:read"
)

// Audit events for credential lifecycle changes.
const (
	EventTokenIssued  = "token.issued"
	EventTokenRevoked = "token.revoked"
)

// IssueToken creates an API token for a user, scoped to an organization.
func (s *Service) IssueToken(ctx context.Context, userID int64, in IssueTokenInput) (IssuedToken, error) {
	if !s.users().Tokens.Enabled {
		return IssuedToken{}, ErrDisabled
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		return IssuedToken{}, validationError("a token name is required")
	}
	if in.OrgID == 0 {
		return IssuedToken{}, validationError("a token must be scoped to an organization")
	}

	access, err := s.require(ctx, in.OrgID, userID, user.PermTokensManage)
	if err != nil {
		return IssuedToken{}, err
	}

	requested := access.Role
	if in.Role != "" {
		parsed, roleErr := user.ParseRole(in.Role)
		if roleErr != nil {
			return IssuedToken{}, validationError("unknown organization role")
		}
		requested = parsed
	}
	// The cap is applied rather than the request rejected, so an
	// over-broad request yields a correctly narrow token instead of an
	// error the caller might work around by retrying at lower roles until
	// one succeeds.
	role := user.CapRole(access.Role, requested)

	scope := security.ScopeRead
	if in.Scope != "" {
		parsed, scopeErr := security.ParseScope(in.Scope)
		if scopeErr != nil {
			return IssuedToken{}, validationError("unknown token scope")
		}
		scope = parsed
	}
	// A global-scope token is an administrative credential. A Regular
	// User never issues one, whatever they ask for.
	if scope == security.ScopeGlobal {
		return IssuedToken{}, ErrForbidden
	}
	// A read-write token cannot come from a role that cannot write.
	if scope == security.ScopeReadWrite && !role.Can(user.PermRecordsWrite) {
		return IssuedToken{}, ErrForbidden
	}

	// A zone-narrowed token must name a zone the issuer may actually
	// edit, so narrowing cannot be used to reach sideways into a zone the
	// member was never granted.
	if in.ZoneID != 0 {
		allowed, zoneErr := s.CanEditZone(ctx, in.OrgID, userID, in.ZoneID)
		if zoneErr != nil {
			return IssuedToken{}, zoneErr
		}
		if !allowed {
			return IssuedToken{}, ErrForbidden
		}
	}

	capability := strings.TrimSpace(in.Capability)
	if capability != "" && capability != CapabilityACMEChallenge &&
		capability != CapabilityMetricsRead && !role.Can(user.Permission(capability)) {
		return IssuedToken{}, validationError("unknown token capability")
	}

	if max := s.users().Tokens.MaxPerUser; max > 0 {
		live, countErr := s.store.CountLiveTokens(ctx, string(security.OwnerUser), userID)
		if countErr != nil {
			return IssuedToken{}, countErr
		}
		if live >= max {
			return IssuedToken{}, ErrQuotaExceeded
		}
	}

	secret, err := security.GenerateToken(security.PrefixUser)
	if err != nil {
		return IssuedToken{}, err
	}

	expires := time.Time{}
	switch {
	case in.ExpiresIn > 0:
		expires = s.now().Add(in.ExpiresIn)
	case s.users().Tokens.ExpirationDays > 0:
		expires = s.now().Add(time.Duration(s.users().Tokens.ExpirationDays) * 24 * time.Hour)
	}

	token, err := s.store.CreateToken(ctx, model.Token{
		OwnerType:  string(security.OwnerUser),
		OwnerID:    userID,
		Name:       name,
		Hash:       security.HashToken(secret),
		Prefix:     security.TokenPrefixDisplay(secret),
		Scope:      string(scope),
		Role:       string(role),
		OrgID:      in.OrgID,
		ZoneID:     in.ZoneID,
		Capability: capability,
		ExpiresAt:  expires,
	})
	if err != nil {
		return IssuedToken{}, mapStoreErr(err)
	}

	s.audit(ctx, in.OrgID, EventTokenIssued, model.ActorUser, userID, 0,
		map[string]any{
			"name": name, "role": string(role), "scope": string(scope),
			"zone_id": in.ZoneID, "capability": capability,
		})

	return IssuedToken{Token: token, Secret: secret}, nil
}

// ListTokens returns a user's own tokens.
func (s *Service) ListTokens(ctx context.Context, userID int64) ([]model.Token, error) {
	tokens, err := s.store.ListTokens(ctx, string(security.OwnerUser), userID)
	return tokens, mapStoreErr(err)
}

// ListOrgTokens returns the tokens held by the organization itself, for
// a caller allowed to manage its credentials.
func (s *Service) ListOrgTokens(ctx context.Context, orgID, actorID int64) ([]model.Token, error) {
	if _, err := s.require(ctx, orgID, actorID, user.PermTokensManage); err != nil {
		return nil, err
	}

	tokens, err := s.store.ListTokens(ctx, string(security.OwnerOrg), orgID)
	return tokens, mapStoreErr(err)
}

// RevokeToken revokes one of the caller's own tokens.
//
// Ownership is checked before the revocation and a token belonging to
// somebody else is reported as not found, so the endpoint cannot be used
// to probe which token identifiers exist.
func (s *Service) RevokeToken(ctx context.Context, userID, tokenID int64) error {
	token, err := s.store.TokenByID(ctx, tokenID)
	if err != nil {
		return mapStoreErr(err)
	}
	if token.OwnerType != string(security.OwnerUser) || token.OwnerID != userID {
		return ErrNotFound
	}
	if err = s.store.RevokeToken(ctx, tokenID); err != nil {
		return mapStoreErr(err)
	}

	if token.OrgID != 0 {
		s.audit(ctx, token.OrgID, EventTokenRevoked, model.ActorUser, userID, 0,
			map[string]any{"name": token.Name})
	}
	return nil
}

// RevokeOrgToken lets an organization's credential manager revoke a
// token scoped to that organization, including one issued by another
// member.
func (s *Service) RevokeOrgToken(ctx context.Context, orgID, actorID, tokenID int64) error {
	if _, err := s.require(ctx, orgID, actorID, user.PermTokensManage); err != nil {
		return err
	}

	token, err := s.store.TokenByID(ctx, tokenID)
	if err != nil {
		return mapStoreErr(err)
	}
	if token.OrgID != orgID {
		return ErrNotFound
	}
	if err = s.store.RevokeToken(ctx, tokenID); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, EventTokenRevoked, model.ActorUser, actorID,
		token.OwnerID, map[string]any{"name": token.Name})
	return nil
}

// TokenAuth is a verified credential together with the standing it
// carries, which is what an authorization check needs.
type TokenAuth struct {
	Token model.Token
	Role  user.Role
	Scope security.Scope
}

// AllowsCapability reports whether the credential permits an action,
// checking the token's own narrowing first and then the role behind it.
func (a TokenAuth) AllowsCapability(name string) bool {
	if !a.Token.AllowsCapability(name) {
		return false
	}
	if name == CapabilityACMEChallenge || name == CapabilityMetricsRead {
		return a.Token.Capability == name || a.Role.Can(user.PermRecordsWrite)
	}
	return a.Role.Can(user.Permission(name))
}

// AllowsZone reports whether the credential reaches a given zone.
func (a TokenAuth) AllowsZone(zoneID int64) bool {
	return a.Token.AllowsZone(zoneID)
}

// AuthenticateToken resolves a presented token string.
//
// The lookup is by hash, so the plaintext is never compared against
// anything stored, and an unusable token is reported with the same error
// as an unknown one: a caller must not be able to tell a revoked
// credential from one that never existed.
func (s *Service) AuthenticateToken(ctx context.Context, presented string) (TokenAuth, error) {
	presented = strings.TrimSpace(presented)
	if presented == "" {
		return TokenAuth{}, ErrInvalidCredentials
	}

	token, err := s.store.TokenByHash(ctx, security.HashToken(presented))
	if err != nil {
		return TokenAuth{}, ErrInvalidCredentials
	}
	if !token.Usable(s.now()) {
		return TokenAuth{}, ErrInvalidCredentials
	}

	// A user token is only as good as the membership behind it: if the
	// holder left the organization the credential stops working, even if
	// the revocation sweep has not reached it.
	role := user.RoleViewer
	if token.OwnerType == string(security.OwnerUser) && token.OrgID != 0 {
		access, accessErr := s.Access(ctx, token.OrgID, token.OwnerID)
		if accessErr != nil {
			return TokenAuth{}, ErrInvalidCredentials
		}
		granted, roleErr := user.ParseRole(token.Role)
		if roleErr != nil {
			return TokenAuth{}, ErrInvalidCredentials
		}
		role = user.CapRole(access.Role, granted)
	} else if parsed, roleErr := user.ParseRole(token.Role); roleErr == nil {
		role = parsed
	}

	scope, err := security.ParseScope(token.Scope)
	if err != nil {
		scope = security.ScopeRead
	}

	// A successful authentication records last_used_at. A failure to
	// write it does not fail the request: the credential is valid and the
	// timestamp is bookkeeping.
	_ = s.store.TouchToken(ctx, token.ID)

	return TokenAuth{Token: token, Role: role, Scope: scope}, nil
}
