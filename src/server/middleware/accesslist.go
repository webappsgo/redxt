package middleware

import (
	"context"
	"net/http"

	"github.com/webappsgo/redxt/src/apierror"
)

// allowlistKey marks a request whose client IP is on the allow-list.
var allowlistKey = contextKey{name: "allowlisted"}

// AllowlistOptions configures the PART 12 allow-list stage.
type AllowlistOptions struct {
	// Enabled turns the stage on. A disabled allow-list marks nothing,
	// which leaves the block-list and the rate limiter fully in force.
	Enabled bool

	// ClientIP resolves the trusted-proxy-aware client address. Nil falls
	// back to the request's TCP peer.
	ClientIP func(*http.Request) string

	// Contains reports whether an IP is on the allow-list. Build it with
	// NewCIDRMatcher so a CIDR block and a bare address behave alike.
	// Nil matches nothing.
	Contains func(ip string) bool
}

// BlocklistOptions configures the PART 12 block-list stage.
type BlocklistOptions struct {
	// Enabled turns the stage on.
	Enabled bool

	// ClientIP resolves the trusted-proxy-aware client address. Nil falls
	// back to the request's TCP peer.
	ClientIP func(*http.Request) string

	// Contains reports whether an IP is blocked. Nil matches nothing.
	Contains func(ip string) bool
}

// Allowlist returns the PART 12 allow-list middleware.
//
// It never short-circuits a request. Its only job is to mark the ones
// whose client IP is explicitly trusted, so the three stages after it —
// the block-list, the rate limiter and the GeoIP annotation — can let
// them through. Marking rather than skipping keeps the decision in one
// place: an operator who allow-lists their monitoring host does not have
// to also add it to a second exemption list for every later stage.
func Allowlist(opts AllowlistOptions) Middleware {
	if !opts.Enabled || opts.Contains == nil {
		return passthrough
	}
	clientIP := opts.ClientIP
	if clientIP == nil {
		clientIP = remoteHost
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if opts.Contains(clientIP(req)) {
				req = withValue(req, allowlistKey, true)
			}
			next.ServeHTTP(w, req)
		})
	}
}

// Blocklist returns the PART 12 block-list middleware, which refuses a
// blocked client with the PART 14 FORBIDDEN envelope.
//
// It runs after the allow-list so an explicitly trusted address is never
// caught by a broad CIDR block, and before the rate limiter so a blocked
// client costs no counter write.
func Blocklist(opts BlocklistOptions) Middleware {
	if !opts.Enabled || opts.Contains == nil {
		return passthrough
	}
	clientIP := opts.ClientIP
	if clientIP == nil {
		clientIP = remoteHost
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !IsAllowlisted(req.Context()) && opts.Contains(clientIP(req)) {
				_ = apierror.SendErrorCode(w, apierror.CodeForbidden)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// IsAllowlisted reports whether the allow-list stage marked this request
// as coming from an explicitly trusted address.
func IsAllowlisted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marked, ok := ctx.Value(allowlistKey).(bool)
	return ok && marked
}

// passthrough is the identity middleware, returned by a stage that is
// switched off. Returning it rather than nil keeps New's argument list
// uniform and costs one function call per disabled stage.
func passthrough(next http.Handler) http.Handler {
	return next
}
