package middleware

import (
	"context"
	"net/http"
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
}

// GeoIP returns the PART 12 GeoIP middleware.
//
// It annotates and never gates. Whatever the lookup returns, the request
// continues to the next stage with the result attached to its context,
// where an authentication handler can weigh an unexpected country as one
// factor among several — a reason to demand a second factor or to raise
// an alert.
//
// This is deliberate and is the PART 11 rule for the whole project:
// GeoIP is a risk signal, never a sole access gate. Geolocation data is
// wrong often enough, and trivially defeated by a VPN often enough, that
// a request refused on country alone blocks travelling legitimate users
// while stopping no motivated attacker. An operator who wants country
// blocking builds it as a policy decision on top of this annotation,
// alongside other signals, rather than as a silent refusal here.
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
			if result, ok := opts.Lookup(clientIP(req)); ok {
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
