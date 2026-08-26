package model

import "time"

// Token is one api_tokens row. Only the SHA-256 hash of the secret is
// persisted, so a database read can never be replayed as a credential.
type Token struct {
	ID        int64
	OwnerType string
	OwnerID   int64
	Name      string
	// Hash is the SHA-256 of the token string.
	Hash string
	// Prefix is the leading, non-secret part of the token kept purely so
	// a user can tell two tokens apart in a list.
	Prefix string
	Scope  string
	// Role is the organization role the token was capped to when it was
	// issued. It can never exceed the issuer's own role, which is the
	// IDEA.md rule that stops a credential from escalating.
	Role string
	// OrgID scopes the token to one organization. Zero means the token
	// is not org-scoped, which is only valid for an admin token.
	OrgID int64
	// ZoneID narrows the token to a single zone. Zero means every zone
	// the org role already allows.
	ZoneID int64
	// Capability narrows the token to one action such as records:write
	// or acme-challenge. An empty value means the role decides.
	Capability string
	LastUsedAt time.Time
	RevokedAt  time.Time
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

// Usable reports whether the token may authenticate a request: not
// revoked, and not past its expiry.
func (t Token) Usable(now time.Time) bool {
	if !t.RevokedAt.IsZero() {
		return false
	}
	if !t.ExpiresAt.IsZero() && !now.Before(t.ExpiresAt) {
		return false
	}
	return true
}

// Narrowed reports whether the token is restricted below its role by a
// zone or a capability.
func (t Token) Narrowed() bool {
	return t.ZoneID != 0 || t.Capability != ""
}

// AllowsCapability reports whether the token permits the named action.
// A token with no capability restriction defers to its role, which the
// caller checks separately; a token with one permits only that action.
func (t Token) AllowsCapability(name string) bool {
	return t.Capability == "" || t.Capability == name
}

// AllowsZone reports whether the token may act on the given zone.
func (t Token) AllowsZone(zoneID int64) bool {
	return t.ZoneID == 0 || t.ZoneID == zoneID
}
