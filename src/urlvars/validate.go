package urlvars

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// devOnlyTLDs lists the internal suffixes accepted in development mode
// and rejected in production, per AI.md PART 8 "Internal/Dev-Only TLDs".
var devOnlyTLDs = map[string]bool{
	"localhost": true, "test": true, "example": true, "invalid": true,
	"local": true, "lan": true, "internal": true, "home": true,
	"localdomain": true, "home.arpa": true, "intranet": true,
	"corp": true, "private": true,
}

// overlayTLDs lists the app-managed overlay-network suffixes. They are
// always valid for internal use and are never set through DOMAIN.
var overlayTLDs = []string{".onion", ".i2p", ".exit"}

// IsValidHost reports whether host may be used as the server's FQDN in
// the current mode, per AI.md PART 8 "FQDN Validation Rules".
//
// IP addresses and single-label names are always rejected. Production
// requires a valid ICANN public suffix and at least an eTLD+1;
// development additionally accepts localhost, the internal dev TLDs and
// the dynamic project TLD derived from projectName.
func IsValidHost(host string, devMode bool, projectName string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))

	// Reject empty.
	if lower == "" {
		return false
	}

	// Reject IP addresses always, bracketed IPv6 literals included.
	if net.ParseIP(strings.Trim(lower, "[]")) != nil {
		return false
	}

	// Handle localhost.
	if lower == FallbackFQDN {
		return devMode
	}

	// Must contain at least one dot.
	if !strings.Contains(lower, ".") {
		return false
	}

	// Overlay network TLDs are valid but app-managed, never set via
	// DOMAIN; they are checked here for internal validation only.
	for _, suffix := range overlayTLDs {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}

	// Dynamic project-specific TLD (e.g. app.redxt) is dev-only.
	if projectName != "" && strings.HasSuffix(lower, "."+strings.ToLower(strings.TrimSpace(projectName))) {
		return devMode
	}

	// Public suffix: a TLD, or an eTLD such as co.uk.
	suffix, icann := publicsuffix.PublicSuffix(lower)

	// Dev-only TLDs are valid in development mode alone.
	if devOnlyTLDs[suffix] {
		return devMode
	}

	// Production requires a valid ICANN TLD.
	if !devMode && !icann {
		return false
	}

	// The host must be at least eTLD+1, not the bare suffix.
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(lower)
	if err != nil {
		return false
	}
	return len(etldPlusOne) > 0
}

// IsValidSSLHost reports whether host is eligible for a publicly
// trusted certificate (Let's Encrypt). Overlay addresses are not
// publicly resolvable and SSL always applies production-strict rules,
// so project TLDs — which are dev-only — cannot qualify.
func IsValidSSLHost(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))

	// .onion addresses cannot use Let's Encrypt; Tor already provides
	// end-to-end encryption, so SSL is optional for them.
	if strings.HasSuffix(lower, ".onion") {
		return false
	}
	// .i2p and .exit are equally unresolvable from the public internet.
	if strings.HasSuffix(lower, ".i2p") || strings.HasSuffix(lower, ".exit") {
		return false
	}

	return IsValidHost(host, false, "")
}

// BaseDomainOf returns the eTLD+1 of host — "myapp.com" for both
// "myapp.com" and "www.myapp.com". It returns "" for IP addresses and
// for hosts with no registrable domain.
func BaseDomainOf(host string) string {
	lower := strings.ToLower(strings.TrimSpace(host))
	if h, _ := splitHostPort(lower); h != "" {
		lower = h
	}
	if lower == "" || net.ParseIP(strings.Trim(lower, "[]")) != nil {
		return ""
	}
	if !strings.Contains(lower, ".") {
		return ""
	}

	// Overlay suffixes are not registrable domains in the public suffix
	// list sense, so take the last two labels directly.
	for _, suffix := range overlayTLDs {
		if strings.HasSuffix(lower, suffix) {
			return lastLabels(lower, 2)
		}
	}

	base, err := publicsuffix.EffectiveTLDPlusOne(lower)
	if err != nil {
		return lastLabels(lower, 2)
	}
	return base
}

// lastLabels returns the final n dot-separated labels of host.
func lastLabels(host string, n int) string {
	labels := strings.Split(host, ".")
	if len(labels) <= n {
		return host
	}
	return strings.Join(labels[len(labels)-n:], ".")
}
