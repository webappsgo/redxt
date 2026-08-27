// Package middleware implements the HTTP middleware chain redxt wraps
// around every route.
//
// It covers four AI.md PARTs:
//
//   - PART 12 — the fixed middleware execution order, the per-IP sliding
//     window rate limiter, and the trusted-proxy client-IP resolution the
//     limiter, blocklist and access log all depend on.
//   - PART 11 — the security headers every response must carry, the six
//     stage output sanitization pipeline, and the Public Endpoint Safety
//     Principle: no stack traces, no internal IPs, no database structure,
//     and no signal telling an attacker whether the username or the
//     password was the wrong half of a failed login.
//   - PART 16 — URL normalization (trailing-slash 301), CORS, and CSRF.
//   - PART 14 — the unified error envelope every rejection is written in,
//     including the rate limiter's 429 body.
//
// Execution order is fixed by PART 12 and is not a tuning knob. Chain
// composes middlewares so the first argument runs first; New assembles
// the whole PART 12 stack from a single Options value:
//
//  1. URLNormalize   — canonical URL, 301 on change
//  2. RequestID      — urlvars.RequestIDMiddleware
//  3. PathSecurity   — traversal, null bytes, encoded separators
//  4. SecurityHeaders
//  5. CORS           — preflight answered here, before any auth work
//  6. CSRF           — cookie-authed state change guard
//  7. Allowlist      — sets the bypass flag for 8, 9 and 10
//  8. Blocklist
//  9. RateLimit
//  10. GeoIP          — annotates; gates only when deny/allow lists are set
//  11. Auth
//  12. Logging
//
// CORS and CSRF are the two PART 16 stages PART 12's numbered list does
// not name. CORS sits directly after SecurityHeaders so a preflight is
// answered before the request costs a database lookup, and CSRF sits
// directly after CORS so a forged state-changing request is rejected
// before any credential is verified.
//
// Every middleware in this package is safe for concurrent use and never
// mutates the request beyond the documented URL normalization and the
// context values it attaches.
package middleware

import (
	"context"
	"net/http"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/urlvars"
)

// Middleware is one link in the HTTP handler chain.
type Middleware func(http.Handler) http.Handler

// contextKey is this package's unexported context key type, so no other
// package can collide with the values attached here.
type contextKey struct{ name string }

// Chain composes middlewares into a single Middleware, preserving
// execution order: Chain(a, b, c) runs a first, then b, then c, then the
// wrapped handler. Nil entries are skipped, so an optional stage can be
// left unset without the caller building a variable-length slice.
func Chain(middlewares ...Middleware) Middleware {
	stack := make([]Middleware, 0, len(middlewares))
	for _, mw := range middlewares {
		if mw != nil {
			stack = append(stack, mw)
		}
	}
	return func(next http.Handler) http.Handler {
		for i := len(stack) - 1; i >= 0; i-- {
			next = stack[i](next)
		}
		return next
	}
}

// Options carries the configuration for every stage of the PART 12
// chain. DefaultOptions fills it from a *config.Config; callers override
// individual stages afterwards.
type Options struct {
	URLNormalize URLNormalizeOptions
	PathSecurity PathSecurityOptions
	Headers      HeadersOptions
	CORS         CORSOptions
	CSRF         CSRFOptions
	Allowlist    AllowlistOptions
	Blocklist    BlocklistOptions
	RateLimit    RateLimitOptions
	GeoIP        GeoIPOptions
	Auth         AuthOptions
	Logging      LoggingOptions
}

// New builds the complete PART 12 middleware chain from opts.
//
// The returned Middleware wraps the router. Stage 2 is
// urlvars.RequestIDMiddleware rather than a local reimplementation: the
// request ID contract (accept a client-supplied UUID, otherwise mint
// one) belongs to urlvars and has one owner.
func New(opts Options) Middleware {
	return Chain(
		URLNormalize(opts.URLNormalize),
		Middleware(urlvars.RequestIDMiddleware),
		PathSecurity(opts.PathSecurity),
		SecurityHeaders(opts.Headers),
		CORS(opts.CORS),
		CSRF(opts.CSRF),
		Allowlist(opts.Allowlist),
		Blocklist(opts.Blocklist),
		RateLimit(opts.RateLimit),
		GeoIP(opts.GeoIP),
		Auth(opts.Auth),
		Logging(opts.Logging),
	)
}

// DefaultOptions derives every stage's settings from server.yml and the
// URL variable resolver.
//
// res supplies the trusted-proxy-aware client address used by the rate
// limiter, the blocklist and the access log; passing nil falls back to
// the request's own TCP peer address, which is correct only when the
// server is not behind a reverse proxy.
//
// The rate limiter is left with no store: the caller must set
// Options.RateLimit.Store to a SQLStore over server.db (or a MemoryStore
// for a single-process deployment) before the chain enforces limits.
func DefaultOptions(cfg *config.Config, res *urlvars.Resolver) Options {
	clientIP := ClientIPFunc(res)
	apiBase := cfg.APIBasePath()

	return Options{
		URLNormalize: URLNormalizeOptions{},
		PathSecurity: PathSecurityOptions{},
		Headers:      DefaultHeadersOptions(cfg),
		CORS: CORSOptions{
			Configured:     cfg.Server.Web.CORS,
			Domains:        SplitDomains(config.Domain()),
			IsTrustedProxy: TrustedProxyFunc(res),
		},
		CSRF: CSRFOptions{
			Enabled:     cfg.Server.Web.CSRF.Enabled,
			ExemptPaths: cfg.Server.Web.CSRF.ExemptPaths,
			SessionCookieNames: []string{
				cfg.Server.Session.Admin.CookieName,
				cfg.Server.Session.User.CookieName,
			},
			SessionSameSiteStrict: isSameSiteStrict(cfg.Server.Session.SameSite),
		},
		Allowlist: AllowlistOptions{ClientIP: clientIP},
		Blocklist: BlocklistOptions{ClientIP: clientIP},
		RateLimit: RateLimitOptions{
			Config:   cfg.Server.RateLimit,
			ClientIP: clientIP,
			AuthRules: []AuthRateRule{
				{Prefix: apiBase + "/server/auth/login", Bucket: BucketLogin},
				{Prefix: apiBase + "/server/auth/password-reset", Bucket: BucketPasswordReset},
				{Prefix: apiBase + "/server/auth/register", Bucket: BucketRegistration},
			},
		},
		GeoIP: GeoIPOptions{ClientIP: clientIP},
		Auth:  AuthOptions{},
		Logging: LoggingOptions{
			ClientIP: clientIP,
		},
	}
}

// ClientIPFunc returns the client-address resolver every IP-keyed stage
// uses. It delegates to urlvars so the trusted-proxy rules from PART 12
// have exactly one implementation.
func ClientIPFunc(res *urlvars.Resolver) func(*http.Request) string {
	if res == nil {
		return func(req *http.Request) string {
			return remoteHost(req)
		}
	}
	return res.ClientIP
}

// TrustedProxyFunc returns the trusted-peer predicate CORS uses before
// honoring a reverse-proxy-learned X-Forwarded-Host. A nil resolver
// trusts nothing, which is the safe default.
func TrustedProxyFunc(res *urlvars.Resolver) func(remoteAddr string) bool {
	if res == nil {
		return func(string) bool { return false }
	}
	return res.IsTrustedProxy
}

// isSameSiteStrict reports whether the configured session cookie
// SameSite attribute is Strict, which is the condition PART 16 lets a
// same-origin request use to bypass the CSRF token check.
func isSameSiteStrict(value string) bool {
	return equalFold(value, "strict")
}

// withValue attaches v to the request's context under key and returns
// the derived request.
func withValue(req *http.Request, key contextKey, v any) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), key, v))
}
