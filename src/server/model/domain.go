package model

import "time"

// Domain is one custom_domains row: an organization-owned hostname that
// serves a redxt surface under the organization's own name, per PART 36.
//
// White-labelling in redxt is cosmetic only. A custom domain changes the
// hostname a page is served from; it never changes which organization's
// data is visible, which is still decided by the org scoping on every
// query.
type Domain struct {
	ID    int64
	OrgID int64
	// Name is the fully qualified domain, lowercased and without a
	// trailing dot.
	Name string
	// Purpose selects which surface the domain serves.
	Purpose    string
	IsApex     bool
	IsWildcard bool
	// VerificationStatus is pending, verified, or failed. It must reach
	// verified before Status may become active: PART 36 never activates
	// an unverified domain.
	VerificationStatus string
	// VerificationToken is the value expected in the TXT record. It is
	// compared in constant time.
	VerificationToken string
	VerifiedAt        time.Time
	LastCheckAt       time.Time
	CheckCount        int
	SSLEnabled        bool
	// SSLStatus is none, pending, issued, or failed.
	SSLStatus     string
	SSLChallenge  string
	SSLIssuedAt   time.Time
	SSLExpiresAt  time.Time
	SSLLastError  string
	Status        string
	SuspendReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Domain purposes. Each names a surface IDEA.md already defines, so a
// domain is bound to a specific role rather than serving everything.
const (
	// PurposeUI serves the white-labelled web interface.
	PurposeUI = "ui"
	// PurposeDDNS serves the dynamic DNS signup and update surface.
	PurposeDDNS = "ddns"
	// PurposeRedirect serves the HTTP redirector.
	PurposeRedirect = "redirect"
	// PurposeParking serves the parking page for a registered but
	// unused name.
	PurposeParking = "parking"
	// PurposeGateway serves the read-only data gateway.
	PurposeGateway = "gateway"
)

// DomainPurposes lists every accepted purpose in display order.
var DomainPurposes = []string{
	PurposeUI, PurposeDDNS, PurposeRedirect, PurposeParking, PurposeGateway,
}

// Verification status values.
const (
	VerificationPending  = "pending"
	VerificationVerified = "verified"
	VerificationFailed   = "failed"
)

// Certificate status values.
const (
	SSLNone    = "none"
	SSLPending = "pending"
	SSLIssued  = "issued"
	SSLFailed  = "failed"
)

// Domain lifecycle status values.
const (
	DomainPending   = "pending"
	DomainActive    = "active"
	DomainSuspended = "suspended"
)

// Verified reports whether ownership has been proven.
func (d Domain) Verified() bool {
	return d.VerificationStatus == VerificationVerified
}

// Servable reports whether the domain may answer requests: ownership
// proven and the domain activated.
func (d Domain) Servable() bool {
	return d.Verified() && d.Status == DomainActive
}

// ValidPurpose reports whether name is one of the accepted purposes.
func ValidPurpose(name string) bool {
	for _, p := range DomainPurposes {
		if p == name {
			return true
		}
	}
	return false
}
