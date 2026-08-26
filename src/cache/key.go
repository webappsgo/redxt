package cache

import (
	"strconv"
	"strings"
)

// Key joins parts into a hierarchical cache key using the PART 9 naming
// rules: colon separators, lowercase only, no spaces or special characters.
// Empty parts are dropped.
func Key(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, p := range parts {
		if n := normalize(p); n != "" {
			normalized = append(normalized, n)
		}
	}
	return strings.Join(normalized, ":")
}

// normalize rewrites a single key segment to the PART 9 character set: it
// trims surrounding space, lowercases the segment, and collapses any run of
// characters outside [a-z0-9._-] into a single dash.
func normalize(part string) string {
	lowered := strings.ToLower(strings.TrimSpace(part))
	var b strings.Builder
	b.Grow(len(lowered))
	dashed := false
	for _, r := range lowered {
		if allowedKeyRune(r) {
			b.WriteRune(r)
			dashed = false
			continue
		}
		if !dashed {
			b.WriteByte('-')
			dashed = true
		}
	}
	return b.String()
}

// allowedKeyRune reports whether a rune may appear in a key segment.
func allowedKeyRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == '-':
		return true
	default:
		return false
	}
}

// Resource builds a single-resource key: {type}:{id}.
func Resource(typ, id string) string {
	return Key(typ, id)
}

// Field builds a resource sub-field key: {type}:{id}:{field}.
func Field(typ, id, field string) string {
	return Key(typ, id, field)
}

// List builds a filtered-list key: {type}:list:{filter}.
func List(typ, filter string) string {
	return Key(typ, "list", filter)
}

// Scoped builds a scoped-resource key: {scope}:{scopeID}:{type}:{id}.
func Scoped(scope, scopeID, typ, id string) string {
	return Key(scope, scopeID, typ, id)
}

// Rate builds a rate-limit counter key: rate:{type}:{key}.
func Rate(typ, key string) string {
	return Key("rate", typ, key)
}

// Lock builds a distributed-lock key: lock:{resource}.
func Lock(resource string) string {
	return Key("lock", resource)
}

// Versioned prefixes a key with a version segment for cache-busting:
// v{version}:{key}. Bumping the version invalidates every reader immediately
// while the old keys expire naturally by TTL.
func Versioned(version int, key string) string {
	return "v" + strconv.Itoa(version) + ":" + key
}
