package middleware

import (
	"net/http"
	"path"
	"strings"

	"github.com/webappsgo/redxt/src/apierror"
)

// DefaultMaxPathLength is the PART 12 ceiling on a request path.
const DefaultMaxPathLength = 2048

// encodedRejects lists the percent-encodings a request path must never
// carry. They are the escaped forms of the characters the decoded check
// already rejects; an attacker uses them to slip a traversal past a
// filter that only inspects the decoded path.
//
// %2e is ".", %2f is "/", %5c is "\", %00 is NUL.
var encodedRejects = []string{"%2e", "%2f", "%5c", "%00"}

// PathSecurityOptions configures the PART 12 path security stage.
type PathSecurityOptions struct {
	// MaxPathLength caps the request path length. Zero selects
	// DefaultMaxPathLength.
	MaxPathLength int
}

// PathSecurity returns the PART 12 path security middleware. It runs
// third — after URL normalization and the request ID, and before every
// stage that makes a decision from the path — so traversal checks see
// the canonical URL and every rejection is already correlatable in the
// logs.
//
// A request is rejected with the PART 14 BAD_REQUEST envelope when its
// path contains a traversal sequence, a NUL byte, an encoded path
// separator, or an encoded dot, in either the decoded or the raw form.
// Surviving requests have their path collapsed with path.Clean so
// duplicate slashes and single-dot segments cannot produce two spellings
// of one route.
func PathSecurity(opts PathSecurityOptions) Middleware {
	limit := opts.MaxPathLength
	if limit <= 0 {
		limit = DefaultMaxPathLength
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			decoded := req.URL.Path
			raw := req.URL.RawPath
			if raw == "" {
				raw = decoded
			}

			if len(decoded) > limit || len(raw) > limit {
				rejectPath(w)
				return
			}
			if strings.Contains(decoded, "..") || strings.Contains(raw, "..") {
				rejectPath(w)
				return
			}
			if strings.ContainsRune(decoded, 0) || strings.ContainsRune(raw, 0) {
				rejectPath(w)
				return
			}
			if strings.Contains(decoded, `\`) {
				rejectPath(w)
				return
			}
			lowered := strings.ToLower(raw)
			for _, encoded := range encodedRejects {
				if strings.Contains(lowered, encoded) {
					rejectPath(w)
					return
				}
			}

			cleaned := path.Clean(decoded)
			if !strings.HasPrefix(cleaned, "/") {
				cleaned = "/" + cleaned
			}
			// URL normalization already removed the trailing slash from
			// every collection path, so a surviving trailing slash marks
			// a file request that path.Clean must not reshape.
			if decoded != "/" && strings.HasSuffix(decoded, "/") && !strings.HasSuffix(cleaned, "/") {
				cleaned += "/"
			}
			req.URL.Path = cleaned

			next.ServeHTTP(w, req)
		})
	}
}

// rejectPath writes the PART 14 BAD_REQUEST envelope. The response says
// nothing about which check tripped: naming the failing rule tells an
// attacker exactly which encoding to try next, which the PART 11 Public
// Endpoint Safety Principle forbids.
func rejectPath(w http.ResponseWriter) {
	_ = apierror.SendErrorCode(w, apierror.CodeBadRequest)
}
