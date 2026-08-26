package model

import "testing"

func TestDomainVerified(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "verified", status: VerificationVerified, want: true},
		{name: "pending", status: VerificationPending},
		{name: "failed", status: VerificationFailed},
		{name: "unset is not verified", status: ""},
		{name: "unknown value is not verified", status: "ok"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Domain{VerificationStatus: tc.status}
			if got := d.Verified(); got != tc.want {
				t.Fatalf("Verified() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDomainServable(t *testing.T) {
	tests := []struct {
		name         string
		verification string
		status       string
		want         bool
	}{
		{
			name:         "verified and active",
			verification: VerificationVerified,
			status:       DomainActive,
			want:         true,
		},
		// PART 36 never serves a domain whose ownership was not proven, so
		// an active row alone is not enough. Both halves must hold.
		{name: "active but never verified", verification: VerificationPending, status: DomainActive},
		{name: "active but verification failed", verification: VerificationFailed, status: DomainActive},
		{name: "verified but still pending activation", verification: VerificationVerified, status: DomainPending},
		{name: "verified but suspended", verification: VerificationVerified, status: DomainSuspended},
		{name: "neither verified nor active"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Domain{VerificationStatus: tc.verification, Status: tc.status}
			if got := d.Servable(); got != tc.want {
				t.Fatalf("Servable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidPurpose(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "ui", input: PurposeUI, want: true},
		{name: "ddns", input: PurposeDDNS, want: true},
		{name: "redirect", input: PurposeRedirect, want: true},
		{name: "parking", input: PurposeParking, want: true},
		{name: "gateway", input: PurposeGateway, want: true},
		{name: "empty", input: ""},
		{name: "unknown", input: "proxy"},
		{name: "case differences are rejected", input: "UI"},
		{name: "surrounding space is rejected", input: " ui "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidPurpose(tc.input); got != tc.want {
				t.Fatalf("ValidPurpose(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestEveryListedPurposeIsAccepted(t *testing.T) {
	// The display list and the validator must agree. A purpose offered in
	// a form but refused on submit would be a dead choice in the UI.
	for _, p := range DomainPurposes {
		if !ValidPurpose(p) {
			t.Errorf("purpose %q is offered but not accepted", p)
		}
	}

	seen := make(map[string]bool, len(DomainPurposes))
	for _, p := range DomainPurposes {
		if seen[p] {
			t.Errorf("purpose %q is listed twice", p)
		}
		seen[p] = true
	}
}
