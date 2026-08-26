package model

import (
	"testing"
	"time"
)

// now is the fixed instant every time-based case in this package is
// judged against. A literal keeps a boundary case honest: a test that
// derives its instant from the clock cannot state which side of the
// boundary it actually landed on.
var now = time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)

func TestUserActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "active", status: StatusActive, want: true},
		{name: "suspended", status: StatusSuspended},
		{name: "pending", status: StatusPending},
		{name: "empty status is not active", status: ""},
		{name: "unknown status is not active", status: "enabled"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := User{Status: tc.status}
			if got := u.Active(); got != tc.want {
				t.Fatalf("Active() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUserLocked(t *testing.T) {
	tests := []struct {
		name  string
		until time.Time
		want  bool
	}{
		{name: "never locked", until: time.Time{}},
		{name: "lockout ends in the future", until: now.Add(time.Second), want: true},
		{name: "lockout ends at this instant", until: now},
		{name: "lockout ended a moment ago", until: now.Add(-time.Nanosecond)},
		{name: "lockout ends a moment from now", until: now.Add(time.Nanosecond), want: true},
		{name: "lockout long expired", until: now.Add(-time.Hour)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := User{LockedUntil: tc.until}
			if got := u.Locked(now); got != tc.want {
				t.Fatalf("Locked() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUserPublic(t *testing.T) {
	tests := []struct {
		name       string
		visibility string
		want       bool
	}{
		{name: "public", visibility: VisibilityPublic, want: true},
		{name: "private", visibility: VisibilityPrivate},
		// An unset visibility must not read as public. A profile whose
		// column was never written would otherwise be exposed by default,
		// which is the wrong way for the mistake to fail.
		{name: "unset is not public", visibility: ""},
		{name: "unknown value is not public", visibility: "visible"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := User{Visibility: tc.visibility}
			if got := u.Public(); got != tc.want {
				t.Fatalf("Public() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUserContactEmail(t *testing.T) {
	tests := []struct {
		name              string
		email             string
		notificationEmail string
		want              string
	}{
		{name: "falls back to the login address", email: "a@example.com", want: "a@example.com"},
		{
			name:              "prefers the notification address",
			email:             "a@example.com",
			notificationEmail: "b@example.com",
			want:              "b@example.com",
		},
		{name: "no address at all", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := User{Email: tc.email, NotificationEmail: tc.notificationEmail}
			if got := u.ContactEmail(); got != tc.want {
				t.Fatalf("ContactEmail() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDefaultPreferences(t *testing.T) {
	prefs := DefaultPreferences(42)

	if prefs.UserID != 42 {
		t.Fatalf("UserID = %d, want 42", prefs.UserID)
	}

	// Security mail defaults on and marketing mail defaults off. A new
	// account must hear about a password change it did not make without
	// having opted into anything first.
	if !prefs.EmailSecurity {
		t.Error("EmailSecurity defaults off, want on")
	}
	if prefs.EmailProduct {
		t.Error("EmailProduct defaults on, want off")
	}
	// The address is not published until the user asks for it to be.
	if prefs.ShowEmail {
		t.Error("ShowEmail defaults on, want off")
	}

	bools := map[string]struct{ got, want bool }{
		"ShowActivity": {prefs.ShowActivity, true},
		"ShowOrgs":     {prefs.ShowOrgs, true},
		"Searchable":   {prefs.Searchable, true},
		"EmailOrg":     {prefs.EmailOrg, true},
		"ReduceMotion": {prefs.ReduceMotion, false},
	}
	for field, c := range bools {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", field, c.got, c.want)
		}
	}

	strs := map[string]struct{ got, want string }{
		"Theme":      {prefs.Theme, "auto"},
		"FontSize":   {prefs.FontSize, "medium"},
		"DateFormat": {prefs.DateFormat, "iso"},
		"TimeFormat": {prefs.TimeFormat, "24h"},
	}
	for field, c := range strs {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", field, c.got, c.want)
		}
	}
}

func TestSessionExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "no expiry set", expiresAt: time.Time{}},
		{name: "expires in the future", expiresAt: now.Add(time.Hour)},
		// A session whose lifetime ends exactly now is over. Treating the
		// boundary as still valid would leave a one-instant window in
		// which an expired cookie still authenticates.
		{name: "expires at this instant", expiresAt: now, want: true},
		{name: "expired a moment ago", expiresAt: now.Add(-time.Nanosecond), want: true},
		{name: "expires a moment from now", expiresAt: now.Add(time.Nanosecond)},
		{name: "long expired", expiresAt: now.Add(-24 * time.Hour), want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := Session{ExpiresAt: tc.expiresAt}
			if got := s.Expired(now); got != tc.want {
				t.Fatalf("Expired() = %v, want %v", got, tc.want)
			}
		})
	}
}
