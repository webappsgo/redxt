package middleware

import (
	"net/http"
	"strings"

	"github.com/webappsgo/redxt/src/urlvars"
)

// CORS header names.
const (
	// HeaderOrigin is the request's originating site.
	HeaderOrigin = "Origin"
	// HeaderVary lists the request headers a response varies on.
	HeaderVary = "Vary"
	// HeaderACAOrigin is Access-Control-Allow-Origin.
	HeaderACAOrigin = "Access-Control-Allow-Origin"
	// HeaderACAMethods is Access-Control-Allow-Methods.
	HeaderACAMethods = "Access-Control-Allow-Methods"
	// HeaderACAHeaders is Access-Control-Allow-Headers.
	HeaderACAHeaders = "Access-Control-Allow-Headers"
	// HeaderACACredentials is Access-Control-Allow-Credentials.
	HeaderACACredentials = "Access-Control-Allow-Credentials"
	// HeaderACMaxAge is Access-Control-Max-Age.
	HeaderACMaxAge = "Access-Control-Max-Age"
	// HeaderACRequestMethod is the preflight's Access-Control-Request-Method.
	HeaderACRequestMethod = "Access-Control-Request-Method"
	// HeaderForwardedHost is the reverse-proxy-supplied original host.
	HeaderForwardedHost = "X-Forwarded-Host"
	// HeaderForwardedProto is the reverse-proxy-supplied original scheme.
	HeaderForwardedProto = "X-Forwarded-Proto"
)

// CORS response values from the PART 16 "CORS Headers" table.
const (
	// CORSAllowMethods is the method list redxt advertises.
	CORSAllowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	// CORSMaxAge is the preflight cache lifetime in seconds, 24 hours.
	CORSMaxAge = "86400"
	// CORSWildcard is the any-origin value. Credentials are never sent
	// alongside it.
	CORSWildcard = "*"
)

// corsAllowHeaders is the Access-Control-Allow-Headers list, spelled out
// header by header.
//
// PART 16 forbids the "*" wildcard here for two reasons: the Fetch
// standard excludes Authorization from the wildcard's coverage, and a
// wildcard is invalid outright on a credentialed response. Keep this
// list in sync with the PART 8 auth token headers in urlvars.
var corsAllowHeaders = []string{
	"Content-Type",
	"Accept",
	"X-Requested-With",
	urlvars.HeaderAuthorization,
	urlvars.HeaderXAPIKey,
	"X-Api-Key",
	urlvars.HeaderAPIKey,
	urlvars.HeaderAPIKeyCompact,
	urlvars.HeaderAuthToken,
	urlvars.HeaderAccessToken,
	urlvars.HeaderXToken,
	urlvars.HeaderToken,
	urlvars.HeaderCSRFToken,
	urlvars.HeaderXSRFToken,
	urlvars.HeaderSessionID,
	urlvars.HeaderServiceToken,
	urlvars.HeaderInternalToken,
}

// CORSAllowHeaders returns the explicit Access-Control-Allow-Headers
// value redxt sends.
func CORSAllowHeaders() string {
	return strings.Join(corsAllowHeaders, ", ")
}

// CORSTier names which step of the PART 16 allow-list resolution order
// produced the effective origin list. It is returned by ResolveCORS so
// the admin panel preview and the debug-mode CORS rejection context can
// explain a decision instead of guessing at it.
type CORSTier string

// The four resolution tiers from PART 16 "CORS Allow-list Resolution
// Order", plus the disabled state.
const (
	// CORSTierDisabled means web.cors is "" — CORS is off entirely and
	// resolution stopped before any other source was consulted.
	CORSTierDisabled CORSTier = "disabled"
	// CORSTierConfig means the list came from an explicit web.cors value.
	CORSTierConfig CORSTier = "config"
	// CORSTierDomain means the list came from the DOMAIN environment
	// variable, expanded to https:// origins.
	CORSTierDomain CORSTier = "domain"
	// CORSTierLearned means the list came from X-Forwarded-Host observed
	// through a trusted proxy.
	CORSTierLearned CORSTier = "learned"
	// CORSTierWildcard means no source produced a list and the fallback
	// "*" applies. Credentials are never allowed on this tier.
	CORSTierWildcard CORSTier = "wildcard"
)

// CORSOptions configures the PART 16 CORS stage.
type CORSOptions struct {
	// Configured is the raw server.web.cors value. "" disables CORS and
	// stops resolution; "*" is the shipped default and means "unset", so
	// resolution continues past it; anything else is a comma-separated
	// explicit origin list.
	Configured string

	// Domains holds the hostnames from the DOMAIN environment variable.
	// Each becomes an https:// origin at resolution tier 2.
	Domains []string

	// LearnedOrigins returns origins the server has already learned from
	// reverse proxies during earlier requests. Nil means the only
	// learning source is the current request's own X-Forwarded-Host.
	LearnedOrigins func() []string

	// IsTrustedProxy gates tier 3: an X-Forwarded-Host is honored only
	// when the immediate peer is a trusted proxy. Nil trusts nothing.
	IsTrustedProxy func(remoteAddr string) bool
}

// CORSDecision is the resolved policy for one request.
type CORSDecision struct {
	// Enabled is false when web.cors is "" and no CORS header is sent.
	Enabled bool
	// Tier names the resolution step that produced Origins.
	Tier CORSTier
	// Origins is the effective allow-list, or the single element "*" on
	// the wildcard tier.
	Origins []string
	// Explicit is true when Origins is a named list rather than "*".
	// Access-Control-Allow-Credentials is sent only when it is true.
	Explicit bool
}

// Allows reports whether origin is on the decision's allow-list and
// returns the Access-Control-Allow-Origin value to echo back. Matching
// is case-insensitive because scheme and host are both case-insensitive.
func (d CORSDecision) Allows(origin string) (string, bool) {
	if !d.Enabled {
		return "", false
	}
	if !d.Explicit {
		return CORSWildcard, true
	}
	for _, allowed := range d.Origins {
		if equalFold(allowed, origin) {
			return allowed, true
		}
	}
	return "", false
}

// ResolveCORS applies the PART 16 allow-list resolution order to a
// request and returns the effective policy.
//
// The order is fixed: an explicit web.cors value wins outright, an empty
// value disables CORS and stops resolution, then DOMAIN entries, then
// reverse-proxy-learned hosts from trusted peers only, then the "*"
// fallback. Credentials ride only on an explicit list.
func ResolveCORS(opts CORSOptions, req *http.Request) CORSDecision {
	configured := strings.TrimSpace(opts.Configured)
	if configured == "" {
		return CORSDecision{Enabled: false, Tier: CORSTierDisabled}
	}
	if configured != CORSWildcard {
		origins := splitOrigins(configured)
		if len(origins) > 0 {
			return CORSDecision{Enabled: true, Tier: CORSTierConfig, Origins: origins, Explicit: true}
		}
	}

	origins := make([]string, 0, len(opts.Domains)+2)
	for _, domain := range opts.Domains {
		origins = appendOrigin(origins, "https://"+domain)
	}
	tier := CORSTierWildcard
	if len(origins) > 0 {
		tier = CORSTierDomain
	}

	learnedCount := len(origins)
	if opts.LearnedOrigins != nil {
		for _, origin := range opts.LearnedOrigins() {
			origins = appendOrigin(origins, origin)
		}
	}
	if origin := forwardedOrigin(opts, req); origin != "" {
		origins = appendOrigin(origins, origin)
	}
	if len(origins) > learnedCount {
		tier = CORSTierLearned
	}

	if len(origins) == 0 {
		return CORSDecision{Enabled: true, Tier: CORSTierWildcard, Origins: []string{CORSWildcard}}
	}
	return CORSDecision{Enabled: true, Tier: tier, Origins: origins, Explicit: true}
}

// CORS returns the PART 16 CORS middleware.
//
// It runs directly after the security headers so a preflight is answered
// with 204 before the request costs a rate-limit lookup, a GeoIP lookup
// or a credential verification. Actual requests carry only the origin
// headers; the method, header and max-age advertisements belong on the
// preflight response, which is the only place a browser reads them.
//
// A request whose Origin is not on the allow-list is passed through
// without Access-Control-Allow-Origin. The browser then blocks the
// response itself, which is the correct CORS failure mode: refusing the
// request server-side would break non-browser clients, which are not
// subject to CORS at all.
func CORS(opts CORSOptions) Middleware {
	allowHeaders := CORSAllowHeaders()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			decision := ResolveCORS(opts, req)
			preflight := req.Method == http.MethodOptions && req.Header.Get(HeaderACRequestMethod) != ""

			if !decision.Enabled {
				if preflight {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, req)
				return
			}

			h := w.Header()
			h.Add(HeaderVary, HeaderOrigin)
			if value, ok := decision.Allows(req.Header.Get(HeaderOrigin)); ok {
				h.Set(HeaderACAOrigin, value)
				if decision.Explicit {
					h.Set(HeaderACACredentials, "true")
				}
			}

			if preflight {
				h.Set(HeaderACAMethods, CORSAllowMethods)
				h.Set(HeaderACAHeaders, allowHeaders)
				h.Set(HeaderACMaxAge, CORSMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

// forwardedOrigin returns the origin learned from this request's
// X-Forwarded-Host, or "" when the peer is not a trusted proxy or sent
// no usable host. The scheme comes from X-Forwarded-Proto when the same
// trusted peer supplied one, and defaults to https otherwise: PART 16
// expands learned hosts as https origins.
func forwardedOrigin(opts CORSOptions, req *http.Request) string {
	if req == nil || opts.IsTrustedProxy == nil || !opts.IsTrustedProxy(req.RemoteAddr) {
		return ""
	}
	host := strings.TrimSpace(strings.Split(req.Header.Get(HeaderForwardedHost), ",")[0])
	if host == "" {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(strings.Split(req.Header.Get(HeaderForwardedProto), ",")[0]))
	if scheme != "http" && scheme != "https" {
		scheme = "https"
	}
	return scheme + "://" + strings.ToLower(host)
}

// splitOrigins splits a comma-separated origin list into trimmed
// entries, dropping empties and the "*" wildcard, which is never mixed
// into an explicit list.
func splitOrigins(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == CORSWildcard {
			continue
		}
		out = appendOrigin(out, part)
	}
	return out
}

// appendOrigin adds origin to list unless it is empty or already
// present, comparing case-insensitively.
func appendOrigin(list []string, origin string) []string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return list
	}
	for _, existing := range list {
		if equalFold(existing, origin) {
			return list
		}
	}
	return append(list, origin)
}
