package model

import "time"

// Org is one organizations row. PART 35 makes the organization the
// tenancy boundary: in redxt every zone, policy, key, and DDNS host
// belongs to an organization and never to a user directly.
type Org struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Website     string
	Location    string
	AvatarURL   string
	Visibility  string
	// Personal marks the organization created automatically with a user
	// account. It cannot be deleted, renamed away from its owner, or
	// have members added, because it exists to give a solo user the same
	// org-scoped code path a team uses.
	Personal  bool
	OwnerID   int64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Public reports whether the organization profile is visible to an
// anonymous visitor.
func (o Org) Public() bool {
	return o.Visibility == VisibilityPublic
}

// Active reports whether the organization may be used.
func (o Org) Active() bool {
	return o.Status == StatusActive
}

// Member is one organization_members row: a user's role inside one
// organization.
type Member struct {
	OrgID     int64
	UserID    int64
	Role      string
	CreatedAt time.Time
	// Username and Email are filled only by the listing query that joins
	// users, so a member list does not need a second round trip.
	Username string
	Email    string
}

// Invite is one invitations row. The plaintext code exists only in the
// message sent to the invitee; the row holds its hash.
type Invite struct {
	ID int64
	// CodeHash is the SHA-256 of the invite code. The plaintext is never
	// stored, so a database read cannot recover a usable invitation.
	CodeHash string
	// OrgID is zero for a server-level registration invite and set for
	// an invitation into a specific organization.
	OrgID      int64
	Email      string
	Role       string
	MaxUses    int
	UseCount   int
	InvitedBy  int64
	AcceptedAt time.Time
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// Redeemable reports whether the invite can still be accepted. A zero
// MaxUses means unlimited redemptions, per PART 34.
func (i Invite) Redeemable(now time.Time) bool {
	if !i.ExpiresAt.IsZero() && !now.Before(i.ExpiresAt) {
		return false
	}
	if i.MaxUses == 0 {
		return true
	}
	return i.UseCount < i.MaxUses
}

// AuditEntry is one organization_audit or custom_domain_audit row.
type AuditEntry struct {
	ID        int64
	SubjectID int64
	Event     string
	ActorType string
	ActorID   int64
	TargetID  int64
	Details   string
	CreatedAt time.Time
}

// Actor types recorded in an audit row.
const (
	ActorUser   = "user"
	ActorAdmin  = "admin"
	ActorSystem = "system"
	ActorToken  = "token"
)
