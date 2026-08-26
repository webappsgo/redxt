package urlvars

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Request tracing headers, in the PART 8 priority order.
const (
	// HeaderRequestID is the standard request identifier, echoed back on
	// every response.
	HeaderRequestID = "X-Request-ID"
	// HeaderCorrelationID is the alternative request identifier.
	HeaderCorrelationID = "X-Correlation-ID"
	// HeaderTraceID is the distributed-tracing identifier.
	HeaderTraceID = "X-Trace-ID"
)

// Auth token headers. Later PARTs read tokens through these constants
// rather than retyping the header strings.
const (
	// HeaderAuthorization carries Bearer, Basic or Digest credentials.
	HeaderAuthorization = "Authorization"
	// HeaderXAPIKey is the common API key header.
	HeaderXAPIKey = "X-API-Key"
	// HeaderAPIKey is the API key header without the X- prefix.
	HeaderAPIKey = "API-Key"
	// HeaderAPIKeyCompact is the unhyphenated API key header.
	HeaderAPIKeyCompact = "ApiKey"
	// HeaderAuthToken is the custom auth token header.
	HeaderAuthToken = "X-Auth-Token"
	// HeaderAccessToken is the access token header.
	HeaderAccessToken = "X-Access-Token"
	// HeaderXToken is the short-form token header.
	HeaderXToken = "X-Token"
	// HeaderToken is the minimal-form token header.
	HeaderToken = "Token"
	// QueryParamToken is the least-preferred token source, avoided in
	// production because query strings land in access logs.
	QueryParamToken = "token"
)

// Session and CSRF headers.
const (
	// HeaderCSRFToken is the CSRF protection token header.
	HeaderCSRFToken = "X-CSRF-Token"
	// HeaderXSRFToken is the Angular CSRF token variant.
	HeaderXSRFToken = "X-XSRF-Token"
	// HeaderSessionID is the session identifier header.
	HeaderSessionID = "X-Session-ID"
)

// Service-to-service headers.
const (
	// HeaderServiceToken authenticates internal service calls.
	HeaderServiceToken = "X-Service-Token"
	// HeaderInternalToken authenticates internal API calls.
	HeaderInternalToken = "X-Internal-Token"
)

// Auth token sources reported by AuthToken. Header sources are reported
// as the header name itself; these name the sources that are not a plain
// header lookup.
const (
	// SourceBearer is an Authorization header carrying a Bearer token.
	SourceBearer = "Authorization Bearer"
	// SourceBasic is an Authorization header carrying Basic credentials.
	SourceBasic = "Authorization Basic"
	// SourceDigest is an Authorization header carrying Digest credentials.
	SourceDigest = "Authorization Digest"
	// SourceQuery is the ?token= query parameter.
	SourceQuery = "query:" + QueryParamToken
)

// contextKey is the unexported type for this package's context keys, so
// no other package can collide with them.
type contextKey struct{ name string }

// requestIDKey stores the resolved request ID in a request context.
var requestIDKey = contextKey{name: "request_id"}

// RequestID returns the request's trace identifier: the client-supplied
// X-Request-ID, X-Correlation-ID or X-Trace-ID when one of them is a
// valid UUID, otherwise a freshly generated UUID v4.
func RequestID(req *http.Request) string {
	if req != nil {
		for _, name := range []string{HeaderRequestID, HeaderCorrelationID, HeaderTraceID} {
			candidate := strings.TrimSpace(req.Header.Get(name))
			if IsValidUUID(candidate) {
				return strings.ToLower(candidate)
			}
		}
	}
	return uuid.NewString()
}

// IsValidUUID reports whether raw is a canonical 36-character UUID.
// Braced and URN forms are rejected: an ID that reaches a log or a
// downstream service must be in one shape only.
func IsValidUUID(raw string) bool {
	if len(raw) != 36 {
		return false
	}
	_, err := uuid.Parse(raw)
	return err == nil
}

// RequestIDMiddleware resolves the request ID, echoes it in the
// X-Request-ID response header, and stores it in the request context for
// logging and downstream propagation.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := RequestID(req)
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, req.WithContext(WithRequestID(req.Context(), id)))
	})
}

// WithRequestID returns a copy of ctx carrying id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext returns the request ID stored by
// RequestIDMiddleware, or "" when the middleware did not run.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, ok := ctx.Value(requestIDKey).(string)
	if !ok {
		return ""
	}
	return id
}

// tokenHeaders lists the plain token headers in priority order, after
// the Authorization header and before the query parameter.
var tokenHeaders = []string{
	HeaderXAPIKey,
	HeaderAPIKey,
	HeaderAPIKeyCompact,
	HeaderAuthToken,
	HeaderAccessToken,
	HeaderXToken,
	HeaderToken,
}

// AuthToken extracts the request's credential and names the source it
// came from, following the AI.md PART 8 priority order: Authorization,
// then the API key headers, then the custom token headers, then the
// ?token= query parameter.
//
// The returned source never contains the token itself, so callers may
// log it freely; the token value must never be logged.
func AuthToken(req *http.Request) (token string, source string) {
	if req == nil {
		return "", ""
	}

	if value, src := authorizationToken(req.Header.Get(HeaderAuthorization)); value != "" {
		return value, src
	}
	for _, name := range tokenHeaders {
		if value := strings.TrimSpace(req.Header.Get(name)); value != "" {
			return value, name
		}
	}
	if req.URL != nil {
		if value := strings.TrimSpace(req.URL.Query().Get(QueryParamToken)); value != "" {
			return value, SourceQuery
		}
	}
	return "", ""
}

// authorizationToken splits an Authorization header into its credential
// and a source label. An unrecognized scheme yields no token so
// resolution continues with the next header.
func authorizationToken(raw string) (token string, source string) {
	raw = strings.TrimSpace(raw)
	scheme, credentials, found := strings.Cut(raw, " ")
	if !found {
		return "", ""
	}
	credentials = strings.TrimSpace(credentials)
	if credentials == "" {
		return "", ""
	}

	switch strings.ToLower(scheme) {
	case "bearer":
		return credentials, SourceBearer
	case "basic":
		return credentials, SourceBasic
	case "digest":
		return credentials, SourceDigest
	default:
		return "", ""
	}
}
