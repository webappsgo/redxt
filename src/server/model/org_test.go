package model

import (
	"testing"
	"time"
)

func TestOrgPublic(t *testing.T) {
	tests := []struct {
		name       string
		visibility string
		want       bool
	}{
		{name: "public", visibility: VisibilityPublic, want: true},
		{name: "private", visibility: VisibilityPrivate},
		{name: "unset is not public", visibility: ""},
		{name: "unknown value is not public", visibility: "internal"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := Org{Visibility: tc.visibility}
			if got := o.Public(); got != tc.want {
				t.Fatalf("Public() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOrgActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "active", status: StatusActive, want: true},
		{name: "suspended", status: StatusSuspended},
		{name: "pending", status: StatusPending},
		{name: "unset is not active", status: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := Org{Status: tc.status}
			if got := o.Active(); got != tc.want {
				t.Fatalf("Active() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInviteRedeemable(t *testing.T) {
	tests := []struct {
		name      string
		maxUses   int
		useCount  int
		expiresAt time.Time
		want      bool
	}{
		{name: "unlimited and unexpiring", want: true},
		{name: "unlimited, already used many times", useCount: 99, want: true},
		{name: "first of one use", maxUses: 1, want: true},
		{name: "single use already spent", maxUses: 1, useCount: 1},
		{name: "last of several uses", maxUses: 3, useCount: 2, want: true},
		{name: "all uses spent", maxUses: 3, useCount: 3},
		// A count past the limit can only come from a race or a hand-edited
		// row. It must still refuse rather than wrap back into validity.
		{name: "over-spent beyond the limit", maxUses: 3, useCount: 4},
		{name: "not yet expired", expiresAt: now.Add(time.Hour), want: true},
		// Expiry is checked before the use count, so an expired invite with
		// uses remaining is still refused.
		{name: "expired with uses remaining", maxUses: 5, expiresAt: now.Add(-time.Hour)},
		{name: "expires at this instant", expiresAt: now},
		{name: "expired a moment ago", expiresAt: now.Add(-time.Nanosecond)},
		{name: "expires a moment from now", expiresAt: now.Add(time.Nanosecond), want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i := Invite{MaxUses: tc.maxUses, UseCount: tc.useCount, ExpiresAt: tc.expiresAt}
			if got := i.Redeemable(now); got != tc.want {
				t.Fatalf("Redeemable() = %v, want %v", got, tc.want)
			}
		})
	}
}
