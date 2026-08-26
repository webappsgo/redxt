// Package admin implements the AI.md PART 17 Admin Panel account space:
// the Server Admin table, its web sessions, and the first-run setup
// wizard that creates the first admin.
//
// Server Admins manage the running application. They are never a
// Regular User: the two account spaces live in separate tables in
// users.db (admins vs. users, both defined in schema_users.go) so a
// credential compromise in one space can never be replayed against the
// other, per PART 17's "Admin credentials are stored in users.db
// (admins table), NOT in config file" rule.
package admin

import "time"

// Admin is one Server Admin row from the users.db admins table.
type Admin struct {
	ID       int64
	Username string
	Email    string
	// PasswordHash is the Argon2id PHC string. It never leaves the
	// server and is never rendered into a response.
	PasswordHash string
	// MustChangePassword forces a password change on next login, which
	// the setup wizard never sets since the operator chooses the
	// password directly.
	MustChangePassword bool
	Disabled           bool
	LastLoginAt        time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Session is one Server Admin web session from the users.db
// admin_sessions table.
type Session struct {
	ID      int64
	AdminID int64
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
