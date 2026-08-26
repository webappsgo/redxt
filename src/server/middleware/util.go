package middleware

import (
	"net"
	"net/http"
	"path"
	"strings"
)

// equalFold reports whether a and b are equal under ASCII case folding.
// Header tokens, scheme names and cookie attributes are all ASCII, so
// the Unicode-aware comparison is neither needed nor wanted here.
func equalFold(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// remoteHost returns the bare host portion of the request's TCP peer
// address, with no proxy headers consulted.
func remoteHost(req *http.Request) string {
	if req == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(req.RemoteAddr)
	}
	return strings.TrimSpace(host)
}

// SplitDomains splits a comma-separated DOMAIN value into trimmed,
// lowercased hostnames, dropping empty entries. It is the input to the
// CORS allow-list resolution order's second tier.
func SplitDomains(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// requestScheme reports the scheme the client used to reach this
// server: https when the connection terminated TLS here, http
// otherwise. Proxy headers are deliberately not consulted — the callers
// that need the forwarded scheme resolve it explicitly with their own
// trusted-proxy check.
func requestScheme(req *http.Request) string {
	if req != nil && req.TLS != nil {
		return "https"
	}
	return "http"
}

// matchesPathPrefix reports whether p is covered by pattern.
//
// A pattern containing a glob metacharacter is matched with path.Match
// against the whole path and against every leading segment prefix, so
// "/api/v1/webhooks/*" covers "/api/v1/webhooks/stripe/events" and not
// just its first segment. Any other pattern is a plain prefix match on
// segment boundaries, so "/server/auth" never matches "/server/authors".
func matchesPathPrefix(p, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.ContainsAny(pattern, "*?[") {
		if ok, err := path.Match(pattern, p); err == nil && ok {
			return true
		}
		for i := len(p) - 1; i > 0; i-- {
			if p[i] != '/' {
				continue
			}
			if ok, err := path.Match(pattern, p[:i]); err == nil && ok {
				return true
			}
		}
		return false
	}
	if p == pattern {
		return true
	}
	return strings.HasPrefix(p, strings.TrimSuffix(pattern, "/")+"/")
}

// matchesAnyPath reports whether p is covered by any of the patterns.
func matchesAnyPath(p string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPathPrefix(p, pattern) {
			return true
		}
	}
	return false
}

// NewCIDRMatcher compiles a list of IPs and CIDR blocks into a
// membership predicate for the allow-list and block-list stages. A bare
// IP is treated as a single-address block. Entries that do not parse are
// returned as the error, so a typo in server.yml surfaces at startup
// rather than silently widening or narrowing the list.
func NewCIDRMatcher(entries []string) (func(ip string) bool, error) {
	nets := make([]*net.IPNet, 0, len(entries))
	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, block, err := net.ParseCIDR(raw); err == nil {
			nets = append(nets, block)
			continue
		}
		ip := net.ParseIP(raw)
		if ip == nil {
			return nil, &net.ParseError{Type: "IP address or CIDR block", Text: raw}
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}

	return func(candidate string) bool {
		ip := net.ParseIP(strings.TrimSpace(candidate))
		if ip == nil {
			return false
		}
		for _, block := range nets {
			if block.Contains(ip) {
				return true
			}
		}
		return false
	}, nil
}
