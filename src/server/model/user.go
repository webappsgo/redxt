// Package model holds the request-independent value types shared by the
// store, service, and handler layers for AI.md PART 34 (Multi-User),
// PART 35 (Organizations), and PART 36 (Custom Domains).
//
// A model type mirrors one persisted row. It never carries a database
// handle and never talks to the network, so the same value can be
// returned from a store, checked by a service, and rendered by a
// handler without a second conversion step.
package model

import "time"

// User is one Regular User row from users.db. Regular Users live in
// their own table and are never merged with Server Admins, per the
// PART 34 and PART 17 separation rule.
type User struct {
	ID       int64
	Username string
	Email    string
	// NotificationEmail receives account mail when it differs from the
	// login address. An empty value means Email is used.
	NotificationEmail string
	// PasswordHash is the Argon2id PHC string. It never leaves the
	// server and is never rendered into a response.
	PasswordHash string
	DisplayName  string
	Bio          string
	Location     string
	Website      string
	AvatarURL    string
	Timezone     string
	Language     string
	// Role is the instance-level role of the account, distinct from the
	// per-organization role held in a Member row.
	Role string
	// Visibility is public or private. A private profile answers 404
	// rather than 403, so its existence is not disclosed.
	Visibility string
	// OrgVisibility lets a private profile still show basic information
	// to members of an organization the user belongs to.
	OrgVisibility bool
	// Status is active, suspended, or pending. Only an active account
	// may open a session.
	Status        string
	EmailVerified bool
	FailedLogins  int
	LockedUntil   time.Time
	LastLoginAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Account status values.
const (
	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusPending   = "pending"
)

// Active reports whether the account may authenticate.
func (u User) Active() bool {
	return u.Status == StatusActive
}

// Locked reports whether the account is inside a lockout window.
func (u User) Locked(now time.Time) bool {
	return !u.LockedUntil.IsZero() && u.LockedUntil.After(now)
}

// Public reports whether the profile is visible to an anonymous visitor.
func (u User) Public() bool {
	return u.Visibility == VisibilityPublic
}

// ContactEmail returns the address account mail should be sent to.
func (u User) ContactEmail() string {
	if u.NotificationEmail != "" {
		return u.NotificationEmail
	}
	return u.Email
}

// Visibility values shared by user and organization profiles.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// Preferences is one user_preferences row. PART 34 creates the row
// lazily on first read, so a user who never changed anything still
// resolves to the documented defaults.
type Preferences struct {
	UserID        int64
	ShowEmail     bool
	ShowActivity  bool
	ShowOrgs      bool
	Searchable    bool
	EmailSecurity bool
	EmailOrg      bool
	EmailProduct  bool
	Theme         string
	FontSize      string
	ReduceMotion  bool
	DateFormat    string
	TimeFormat    string
	UpdatedAt     time.Time
}

// DefaultPreferences returns the PART 34 defaults for a user who has
// never saved a preference. Security mail is on and cannot be turned
// off through this struct's default, because an account-security notice
// is not marketing.
func DefaultPreferences(userID int64) Preferences {
	return Preferences{
		UserID:        userID,
		ShowEmail:     false,
		ShowActivity:  true,
		ShowOrgs:      true,
		Searchable:    true,
		EmailSecurity: true,
		EmailOrg:      true,
		EmailProduct:  false,
		Theme:         "auto",
		FontSize:      "medium",
		ReduceMotion:  false,
		DateFormat:    "iso",
		TimeFormat:    "24h",
	}
}

// Session is one user_sessions row. The cookie value itself is never
// persisted; the row is found by the SHA-256 hash of that value.
type Session struct {
	ID     int64
	UserID int64
	// Hash is the SHA-256 of the session identifier handed to the
	// browser, which is the only form of it the server keeps.
	Hash      string
	IP        string
	UserAgent string
	// TwoFactorOK marks a session that has already satisfied the second
	// factor. A session without it may only reach the 2FA challenge.
	TwoFactorOK bool
	CreatedAt   time.Time
	LastActive  time.Time
	ExpiresAt   time.Time
}

// Expired reports whether the session is past its absolute lifetime.
func (s Session) Expired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt)
}
