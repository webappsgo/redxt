package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/urlvars"
)

// authKey stores the resolved identity in a request context.
var authKey = contextKey{name: "auth"}

// Authenticated subject kinds.
const (
	// SubjectAdmin is a Server Admin, who manages the application.
	SubjectAdmin = "admin"
	// SubjectUser is a Regular User, an end user of the application.
	SubjectUser = "user"
	// SubjectService is a machine caller using a service token.
	SubjectService = "service"
)

// AuthInfo describes the identity behind a request.
//
// It never carries the credential itself. The token is verified once, in
// the Verifier, and is not propagated: a value that reaches a handler
// eventually reaches a log, and PART 11 forbids a raw token appearing in
// one.
type AuthInfo struct {
	// Subject is the account identifier.
	Subject string
	// Kind is SubjectAdmin, SubjectUser or SubjectService.
	Kind string
	// Source names where the credential came from, as reported by
	// urlvars.AuthToken — a header name, "Authorization Bearer", or
	// "query:token". It is safe to log; it contains no secret.
	Source string
	// Scopes are the permissions the credential grants.
	Scopes []string
	// SessionID identifies the browser session when the credential was a
	// session cookie, and is "" otherwise.
	SessionID string
}

// HasScope reports whether the identity holds scope.
func (a AuthInfo) HasScope(scope string) bool {
	for _, held := range a.Scopes {
		if held == scope {
			return true
		}
	}
	return false
}

// TokenVerifier turns a credential into an identity.
//
// VerifyToken receives the raw credential and the source urlvars
// reported it came from. VerifySession receives a session cookie value.
// Both return ok=false for a credential that does not resolve; neither
// distinguishes "no such account" from "wrong secret", because PART 11
// forbids telling a caller which half of a failed login was wrong.
//
// The implementation compares hashes, never plaintext: passwords are
// Argon2id and tokens are SHA-256, per PART 11.
type TokenVerifier interface {
	VerifyToken(ctx context.Context, token, source string) (AuthInfo, bool)
	VerifySession(ctx context.Context, cookieName, value string) (AuthInfo, bool)
}

// AuthOptions configures the PART 12 authentication stage.
type AuthOptions struct {
	// Verifier resolves credentials. Nil leaves every request
	// unauthenticated, which is the correct state before the account
	// store is wired up: handlers still guard themselves.
	Verifier TokenVerifier

	// SessionCookieNames are the cookies that may carry a session, in
	// priority order.
	SessionCookieNames []string

	// IsPublicPath reports whether a path is reachable without
	// credentials. Nil treats every path as public, so this stage
	// refuses nothing on its own.
	IsPublicPath func(path string) bool
}

// Auth returns the PART 12 authentication middleware.
//
// It resolves a credential — a session cookie first, then any of the
// PART 8 token sources through urlvars.AuthToken — and attaches the
// resulting identity to the request context. It does not authorize:
// deciding whether an identity may perform an action belongs to the
// handler, which knows what the action is.
//
// An unauthenticated request to a public route passes through untouched,
// with no identity attached and no header rewritten. Only a
// non-public route with no valid credential is refused, with the PART 14
// UNAUTHORIZED envelope and no hint about why: an invalid token, an
// expired token and an unknown account produce byte-identical responses.
func Auth(opts AuthOptions) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if info, ok := resolveIdentity(opts, req); ok {
				next.ServeHTTP(w, withValue(req, authKey, info))
				return
			}
			if opts.IsPublicPath == nil || opts.IsPublicPath(req.URL.Path) {
				next.ServeHTTP(w, req)
				return
			}
			_ = apierror.SendErrorCode(w, apierror.CodeUnauthorized)
		})
	}
}

// resolveIdentity tries the session cookies and then the PART 8 token
// sources, returning the first identity that verifies.
func resolveIdentity(opts AuthOptions, req *http.Request) (AuthInfo, bool) {
	if opts.Verifier == nil {
		return AuthInfo{}, false
	}
	ctx := req.Context()

	for _, name := range opts.SessionCookieNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cookie, err := req.Cookie(name)
		if err != nil || cookie.Value == "" {
			continue
		}
		if info, ok := opts.Verifier.VerifySession(ctx, name, cookie.Value); ok {
			return info, true
		}
	}

	token, source := urlvars.AuthToken(req)
	if token == "" {
		return AuthInfo{}, false
	}
	return opts.Verifier.VerifyToken(ctx, token, source)
}

// AuthFromContext returns the identity the Auth stage resolved, and
// whether the request was authenticated at all.
func AuthFromContext(ctx context.Context) (AuthInfo, bool) {
	if ctx == nil {
		return AuthInfo{}, false
	}
	info, ok := ctx.Value(authKey).(AuthInfo)
	return info, ok
}
