package middleware

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/urlvars"
)

// CSRF request identifiers.
const (
	// HeaderAPIToken is the programmatic-client credential header. A
	// request carrying it is not a browser form post and is exempt from
	// the CSRF check.
	HeaderAPIToken = "X-API-Token"
	// HeaderReferer is the fallback origin source when Origin is absent.
	HeaderReferer = "Referer"
	// HeaderUpgrade carries the WebSocket upgrade request.
	HeaderUpgrade = "Upgrade"
	// DefaultCSRFCookie is the double-submit cookie name.
	DefaultCSRFCookie = "csrf_token"
	// DefaultCSRFField is the hidden form field carrying the token, used
	// by every no-JavaScript form in the admin panel.
	DefaultCSRFField = "csrf_token"
	// SchemeWebSocket is the Upgrade value that identifies a WebSocket
	// handshake.
	SchemeWebSocket = "websocket"
)

// csrfSafeMethods are the methods that never change state and therefore
// never require a token.
var csrfSafeMethods = []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace}

// csrfHeaderNames are the headers a client may carry the double-submit
// token in, in priority order.
var csrfHeaderNames = []string{urlvars.HeaderCSRFToken, urlvars.HeaderXSRFToken}

// CSRFOptions configures the PART 16 CSRF stage.
type CSRFOptions struct {
	// Enabled mirrors server.web.csrf.enabled. When false the stage is a
	// pass-through.
	Enabled bool

	// ExemptPaths lists path prefixes from server.web.csrf.exempt_paths
	// that skip the check — inbound webhooks, whose sender cannot know a
	// per-session token, are the reason this list exists.
	ExemptPaths []string

	// SessionCookieNames names the cookies that constitute a browser
	// session. The check applies only to a request authenticated by one
	// of them: a request with no session cookie has no ambient authority
	// for a third-party site to abuse.
	SessionCookieNames []string

	// SessionSameSiteStrict reports whether session cookies are issued
	// with SameSite=Strict. When they are, the browser itself withholds
	// them from cross-site requests, so a same-origin request needs no
	// second check.
	SessionSameSiteStrict bool

	// CookieName is the double-submit cookie. Empty selects
	// DefaultCSRFCookie.
	CookieName string

	// HeaderNames overrides the token headers. Empty selects
	// X-CSRF-Token and X-XSRF-Token.
	HeaderNames []string

	// FieldName is the form field carrying the token. Empty selects
	// DefaultCSRFField.
	FieldName string

	// IsPublicPath reports whether a path is a public endpoint, which is
	// exempt because it is reachable without a session at all. Nil means
	// no path is public.
	IsPublicPath func(path string) bool

	// SelfOrigin returns this server's own origin for the request, used
	// to classify Origin and Referer as same-site. Nil derives it from
	// the request scheme and Host.
	SelfOrigin func(*http.Request) string
}

// CSRF returns the PART 16 CSRF middleware.
//
// The check is deliberately narrow. It runs only when all three of the
// following hold, which is the exact shape of a cross-site request
// forgery and nothing else:
//
//  1. the method changes state — POST, PUT, PATCH or DELETE;
//  2. the request is authenticated by a session cookie, so the browser
//     is attaching ambient authority the attacker's page cannot read;
//  3. the Origin or Referer is cross-site, or missing entirely.
//
// Everything else is bypassed: Bearer and X-API-Token credentials, which
// an attacker's page cannot make the browser attach; public endpoints;
// safe methods; same-origin requests under SameSite=Strict cookies;
// WebSocket upgrades, which carry their own origin check in the
// handshake; and any prefix in web.csrf.exempt_paths.
//
// Validation is the double-submit pattern: the token in the request must
// equal the token in the CSRF cookie. An attacker's page can cause the
// cookie to be sent but cannot read it, so it cannot echo the value
// back. The comparison is constant-time so a rejected request leaks no
// information about how much of the token was correct.
func CSRF(opts CSRFOptions) Middleware {
	cookieName := firstNonEmpty(opts.CookieName, DefaultCSRFCookie)
	fieldName := firstNonEmpty(opts.FieldName, DefaultCSRFField)
	headerNames := opts.HeaderNames
	if len(headerNames) == 0 {
		headerNames = csrfHeaderNames
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !CSRFRequired(opts, req) {
				next.ServeHTTP(w, req)
				return
			}

			cookie, err := req.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				rejectCSRF(w)
				return
			}
			presented := csrfPresentedToken(req, headerNames, fieldName)
			if presented == "" {
				rejectCSRF(w)
				return
			}
			if subtle.ConstantTimeCompare([]byte(presented), []byte(cookie.Value)) != 1 {
				rejectCSRF(w)
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

// CSRFRequired reports whether a request must present a valid CSRF
// token. It is exported so a handler that consumes the request body
// itself can apply the same decision without re-deriving the rules.
func CSRFRequired(opts CSRFOptions, req *http.Request) bool {
	if !opts.Enabled || req == nil {
		return false
	}
	for _, method := range csrfSafeMethods {
		if req.Method == method {
			return false
		}
	}
	if isWebSocketUpgrade(req) {
		return false
	}
	if matchesAnyPath(req.URL.Path, opts.ExemptPaths) {
		return false
	}
	if opts.IsPublicPath != nil && opts.IsPublicPath(req.URL.Path) {
		return false
	}
	if hasProgrammaticCredential(req) {
		return false
	}
	if !hasSessionCookie(req, opts.SessionCookieNames) {
		return false
	}
	// Under SameSite=Strict the browser withholds the session cookie from
	// every cross-site request, so the cookie arriving with no Origin and
	// no Referer is itself proof of a same-site request. Without that
	// guarantee the same request is "unknown origin" and must present a
	// token.
	if opts.SessionSameSiteStrict && !hasOriginOrReferer(req) {
		return false
	}
	return !requestIsSameSite(opts, req)
}

// hasOriginOrReferer reports whether the request declares where it came
// from.
func hasOriginOrReferer(req *http.Request) bool {
	return strings.TrimSpace(req.Header.Get(HeaderOrigin)) != "" ||
		strings.TrimSpace(req.Header.Get(HeaderReferer)) != ""
}

// hasProgrammaticCredential reports whether the request authenticates
// with a token a cross-site page cannot cause the browser to attach.
// Bearer tokens and X-API-Token both have to be set explicitly by the
// caller, which forgery from a third-party page cannot do.
func hasProgrammaticCredential(req *http.Request) bool {
	if strings.TrimSpace(req.Header.Get(HeaderAPIToken)) != "" {
		return true
	}
	_, source := urlvars.AuthToken(req)
	return source == urlvars.SourceBearer
}

// hasSessionCookie reports whether the request carries one of the
// session cookies that give it ambient authority.
func hasSessionCookie(req *http.Request, names []string) bool {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if cookie, err := req.Cookie(name); err == nil && cookie.Value != "" {
			return true
		}
	}
	return false
}

// isWebSocketUpgrade reports whether the request is a WebSocket
// handshake, which performs its own origin validation.
func isWebSocketUpgrade(req *http.Request) bool {
	return equalFold(req.Header.Get(HeaderUpgrade), SchemeWebSocket)
}

// requestIsSameSite reports whether the request's Origin — or its
// Referer when no Origin was sent — matches this server's own origin. A
// request with neither header is treated as cross-site: an unknown
// origin gets the same answer as a hostile one.
func requestIsSameSite(opts CSRFOptions, req *http.Request) bool {
	self := csrfSelfOrigin(opts, req)
	if self == "" {
		return false
	}
	if origin := strings.TrimSpace(req.Header.Get(HeaderOrigin)); origin != "" {
		return equalFold(origin, self)
	}
	referer := strings.TrimSpace(req.Header.Get(HeaderReferer))
	if referer == "" {
		return false
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	return equalFold(parsed.Scheme+"://"+parsed.Host, self)
}

// csrfSelfOrigin returns the server's own origin for this request.
func csrfSelfOrigin(opts CSRFOptions, req *http.Request) string {
	if opts.SelfOrigin != nil {
		return opts.SelfOrigin(req)
	}
	if req.Host == "" {
		return ""
	}
	return requestScheme(req) + "://" + req.Host
}

// csrfPresentedToken returns the token the client supplied, preferring
// the headers so a JSON request body is never consumed.
//
// The form field is read only for an urlencoded body, and through
// PostFormValue, which caches the parsed form on the request: the
// handler downstream still sees its own form values. A multipart body is
// left untouched — an upload form carries its token in a header or not
// at all, because buffering the upload here to find a field would defeat
// the streaming the handler relies on.
func csrfPresentedToken(req *http.Request, headerNames []string, fieldName string) string {
	for _, name := range headerNames {
		if value := strings.TrimSpace(req.Header.Get(name)); value != "" {
			return value
		}
	}
	contentType, _, _ := strings.Cut(req.Header.Get("Content-Type"), ";")
	if equalFold(contentType, "application/x-www-form-urlencoded") {
		return strings.TrimSpace(req.PostFormValue(fieldName))
	}
	return ""
}

// rejectCSRF writes the PART 14 FORBIDDEN envelope. It names no reason:
// telling a caller whether the cookie, the header or the comparison
// failed hands an attacker a probe, which the PART 11 Public Endpoint
// Safety Principle forbids.
func rejectCSRF(w http.ResponseWriter) {
	_ = apierror.SendErrorCode(w, apierror.CodeForbidden)
}
