package model

import (
	"testing"
	"time"
)

func TestTokenUsable(t *testing.T) {
	tests := []struct {
		name      string
		revokedAt time.Time
		expiresAt time.Time
		want      bool
	}{
		{name: "live and unexpiring", want: true},
		{name: "live until the future", expiresAt: now.Add(time.Hour), want: true},
		{name: "revoked", revokedAt: now.Add(-time.Hour)},
		// Revocation is absolute. A token revoked at an instant still in
		// the future is already refused, because a revocation record is a
		// decision that has been made rather than one that is scheduled.
		{name: "revoked with a future timestamp", revokedAt: now.Add(time.Hour)},
		{name: "revoked at this instant", revokedAt: now},
		// Revocation is checked before expiry, so a revoked token that has
		// not expired is still refused.
		{name: "revoked but not expired", revokedAt: now.Add(-time.Hour), expiresAt: now.Add(time.Hour)},
		{name: "expires at this instant", expiresAt: now},
		{name: "expired a moment ago", expiresAt: now.Add(-time.Nanosecond)},
		{name: "expires a moment from now", expiresAt: now.Add(time.Nanosecond), want: true},
		{name: "long expired", expiresAt: now.Add(-24 * time.Hour)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := Token{RevokedAt: tc.revokedAt, ExpiresAt: tc.expiresAt}
			if got := tok.Usable(now); got != tc.want {
				t.Fatalf("Usable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTokenNarrowed(t *testing.T) {
	tests := []struct {
		name       string
		zoneID     int64
		capability string
		want       bool
	}{
		{name: "role decides everything"},
		{name: "narrowed to one zone", zoneID: 7, want: true},
		{name: "narrowed to one capability", capability: "records:write", want: true},
		{name: "narrowed by both", zoneID: 7, capability: "records:write", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := Token{ZoneID: tc.zoneID, Capability: tc.capability}
			if got := tok.Narrowed(); got != tc.want {
				t.Fatalf("Narrowed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTokenAllowsCapability(t *testing.T) {
	tests := []struct {
		name       string
		capability string
		ask        string
		want       bool
	}{
		{name: "unrestricted defers to the role", ask: "records:write", want: true},
		{name: "unrestricted with an empty ask", want: true},
		{name: "exact match", capability: "records:write", ask: "records:write", want: true},
		{name: "different action", capability: "records:write", ask: "records:read"},
		{name: "different action entirely", capability: "acme-challenge", ask: "records:write"},
		// Matching is exact rather than by prefix. Were a prefix enough, a
		// token scoped to records:read would also pass for a name like
		// records:read-write that it was never granted.
		{name: "prefix is not a match", capability: "records:read", ask: "records:read-write"},
		{name: "case differences are not a match", capability: "records:write", ask: "Records:Write"},
		// A restricted token must not be satisfied by asking for nothing.
		{name: "restricted token with an empty ask", capability: "records:write", ask: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := Token{Capability: tc.capability}
			if got := tok.AllowsCapability(tc.ask); got != tc.want {
				t.Fatalf("AllowsCapability(%q) = %v, want %v", tc.ask, got, tc.want)
			}
		})
	}
}

func TestTokenAllowsZone(t *testing.T) {
	tests := []struct {
		name   string
		zoneID int64
		ask    int64
		want   bool
	}{
		{name: "unrestricted allows any zone", ask: 7, want: true},
		{name: "unrestricted allows the zero zone", want: true},
		{name: "matching zone", zoneID: 7, ask: 7, want: true},
		{name: "another zone", zoneID: 7, ask: 8},
		// A token pinned to a zone must not be satisfied by an unset zone
		// id, which is what an uninitialised caller would pass.
		{name: "pinned token asked about no zone", zoneID: 7, ask: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := Token{ZoneID: tc.zoneID}
			if got := tok.AllowsZone(tc.ask); got != tc.want {
				t.Fatalf("AllowsZone(%d) = %v, want %v", tc.ask, got, tc.want)
			}
		})
	}
}
