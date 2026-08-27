package middleware

import (
	"context"
	"net/http"

	"github.com/webappsgo/redxt/src/apierror"
)

// geoKey stores the resolved GeoIP annotation in a request context.
var geoKey = contextKey{name: "geoip"}

// GeoResult is what a GeoIP lookup contributes to a request.
type GeoResult struct {
	// Country is the ISO 3166-1 alpha-2 country code, or "" when the
	// address did not resolve.
	Country string
	// ASN is the autonomous system number as a string, or "" when
	// unknown.
	ASN string
	// Organization is the ASN's registered name, or "".
	Organization string
}

// GeoIPOptions configures the PART 12 GeoIP stage.
type GeoIPOptions struct {
	// ClientIP resolves the trusted-proxy-aware client address. Nil
	// falls back to the request's TCP peer.
	ClientIP func(*http.Request) string

	// Lookup resolves an address to a GeoResult. It is the seam PART 20
	// plugs its ip-location-db reader into; nil leaves every request
	// unannotated, which is the correct behavior before the databases
	// have been downloaded.
	Lookup func(ip string) (GeoResult, bool)

	// Blocked reports whether server.geoip.deny_countries/allow_countries
	// refuse this address. It is the seam geoip.Service.Blocked plugs
	// into; nil (the default, and the only behavior when both lists are
	// empty) leaves this stage a pure annotator. This is a policy
	// decision layered on top of the annotation, not a replacement for
	// it — PART 11's "never a sole gate" rule still holds: an operator
	// who leaves both lists empty gets exactly the old annotate-only
	// behavior, and Blocked itself fails open on a missing/stale/errored
	// database or a private address, per PART 20.
	Blocked func(ip string) bool
}

// GeoIP returns the PART 12 GeoIP middleware.
//
// By default it only annotates: whatever the lookup returns, the request
// continues to the next stage with the result attached to its context,
// where an authentication handler can weigh an unexpected country as one
// factor among several — a reason to demand a second factor or to raise
// an alert.
//
// This is deliberate and is the PART 11 rule for the whole project:
// GeoIP is a risk signal, never a sole access gate. Geolocation data is
// wrong often enough, and trivially defeated by a VPN often enough, that
// a request refused on country alone blocks travelling legitimate users
// while stopping no motivated attacker.
//
// PART 20 gives operators an explicit opt-in country allow/deny list.
// When opts.Blocked is set (server.geoip.deny_countries/allow_countries
// non-empty), a blocked address is refused here with 403 FORBIDDEN,
// mirroring the Blocklist stage two steps earlier in the chain. Both
// lists default to empty, so this is a no-op under default config —
// an operator gets the old silent-annotate behavior until they
// explicitly configure a list, at which point PART 20's behavior table
// takes effect exactly as documented. This still respects PART 11:
// Blocked never runs alone (it sits after the rate limiter, so a
// blocked-country flood still spends rate-limit budget, and every
// other stage — allowlist, auth, input validation — still applies to
// every other request regardless of country), it fails open on a
// missing/stale/errored database, and it never looks up or blocks a
// private/internal address.
//
// The stage runs after the rate limiter so a flood costs no database
// lookups, and before Auth so the credential check can already see the
// annotation.
func GeoIP(opts GeoIPOptions) Middleware {
	if opts.Lookup == nil {
		return passthrough
	}
	clientIP := opts.ClientIP
	if clientIP == nil {
		clientIP = remoteHost
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := clientIP(req)
			if opts.Blocked != nil && !IsAllowlisted(req.Context()) && opts.Blocked(ip) {
				_ = apierror.SendErrorCode(w, apierror.CodeForbidden)
				return
			}
			if result, ok := opts.Lookup(ip); ok {
				req = withValue(req, geoKey, result)
			}
			next.ServeHTTP(w, req)
		})
	}
}

// GeoFromContext returns the annotation the GeoIP stage attached, and
// whether one was attached at all. A false second return means the
// address did not resolve or the databases are not loaded — it never
// means the request should be refused.
func GeoFromContext(ctx context.Context) (GeoResult, bool) {
	if ctx == nil {
		return GeoResult{}, false
	}
	result, ok := ctx.Value(geoKey).(GeoResult)
	return result, ok
}
