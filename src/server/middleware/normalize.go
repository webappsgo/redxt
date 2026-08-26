package middleware

import (
	"net/http"
	"strings"
)

// URLNormalizeOptions configures the PART 16 URL normalization stage.
type URLNormalizeOptions struct {
	// LowercasePaths adds a 301 to the lowercased path when the request
	// path contains an uppercase ASCII letter.
	//
	// It is off by default because the PART 16 "Normalization Rules"
	// table defines exactly one generic transformation — trailing-slash
	// removal — and case folding is scoped there to vanity slug
	// resolution, which the slug handler performs itself. Static assets
	// with case-sensitive names would break under a blanket fold, so
	// enabling this is an explicit operator decision.
	LowercasePaths bool
}

// URLNormalize returns the PART 16 URL normalization middleware, which
// runs first in the PART 12 chain so every later stage — traversal
// checks, rate-limit buckets, route matching — sees one canonical form
// of each URL.
//
// Canonical form has no trailing slash. Root "/" is exempt, and a final
// segment containing a dot is treated as an explicit file request and
// left alone. A path that changes is answered with a 301 to the
// canonical URL, query string preserved, rather than being rewritten
// silently: search engines and clients both need the redirect to
// converge on one URL per resource.
func URLNormalize(opts URLNormalizeOptions) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			p := req.URL.Path
			if p == "" {
				p = "/"
			}

			canonical := p
			if opts.LowercasePaths && hasUpperASCII(canonical) {
				canonical = strings.ToLower(canonical)
			}
			if canonical != "/" && strings.HasSuffix(canonical, "/") && !lastSegmentIsFile(canonical) {
				canonical = strings.TrimSuffix(canonical, "/")
			}

			if canonical == p {
				next.ServeHTTP(w, req)
				return
			}

			target := canonical
			if req.URL.RawQuery != "" {
				target += "?" + req.URL.RawQuery
			}
			http.Redirect(w, req, target, http.StatusMovedPermanently)
		})
	}
}

// lastSegmentIsFile reports whether the final path segment names a file
// rather than a collection, which PART 16 recognizes by the presence of
// a dot: "/dir/index.html" keeps its shape, "/users/" does not.
func lastSegmentIsFile(p string) bool {
	// A trailing slash is dropped first, so the segment examined is the
	// one that names the resource rather than the empty string after it.
	p = strings.TrimSuffix(p, "/")
	return strings.Contains(p[strings.LastIndex(p, "/")+1:], ".")
}

// hasUpperASCII reports whether s contains an uppercase ASCII letter.
func hasUpperASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return true
		}
	}
	return false
}
