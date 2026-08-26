package security

import (
	"net/netip"
	"strings"
)

// SensitiveFieldSubstrings lists the case-insensitive substrings that mark a
// field name as sensitive. Any field whose name contains one of them is
// redacted before it reaches a log line, an audit entry, or an API response.
//
// The first group comes from the AI.md PART 11 Output Sanitization Pipeline
// ("any field whose name contains secret, key, password, token"); the tsig and
// dnssec entries come from IDEA.md's data-sensitivity table, which classifies
// TSIG secrets and DNSSEC private keys as critical and never loggable.
var SensitiveFieldSubstrings = []string{
	"secret",
	"key",
	"password",
	"passwd",
	"token",
	"credential",
	"private",
	"apikey",
	"api_key",
	"authorization",
	"cookie",
	"session",
	"tsig",
	"dnssec",
}

// redactedValue is the placeholder that replaces every redacted value; the
// field name is always preserved so the shape of the record stays readable.
const redactedValue = "xxxxx"

// maskedPlaceholder is returned when a value cannot be partially masked
// without leaking it.
const maskedPlaceholder = "***"

// IsSensitiveField reports whether a field name marks a value that must never
// be logged. The match is case-insensitive and substring based, so
// "TSIG_Secret", "apiToken", and "user_password" are all caught.
func IsSensitiveField(name string) bool {
	lower := strings.ToLower(name)
	for _, needle := range SensitiveFieldSubstrings {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// RedactValue returns the placeholder that replaces a redacted value.
func RedactValue() string {
	return redactedValue
}

// RedactMap returns a copy of m with the value of every sensitive key replaced
// by the redaction placeholder. Nested map[string]any and []any values are
// walked recursively. The input map is never mutated, so a caller may safely
// pass a live structure straight to the logger.
func RedactMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveField(k) {
			out[k] = redactedValue
			continue
		}
		out[k] = redactAny(v)
	}
	return out
}

// redactAny redacts nested containers, leaving scalar values untouched.
func redactAny(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return RedactMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactAny(item)
		}
		return out
	default:
		return v
	}
}

// MaskEmail returns the audit-log form of an email address, for example
// "john@example.com" becomes "j***n@e***.com".
//
// The local part keeps its first and last character; a local part of one or
// two characters is replaced entirely because keeping both ends would keep the
// whole value. Every domain label except the final one keeps only its first
// character, and the final label (the TLD) is kept whole. A value with no "@"
// is not an address and is masked completely.
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return maskedPlaceholder
	}
	local := email[:at]
	domain := email[at+1:]
	if domain == "" {
		return maskedPlaceholder
	}

	masked := maskedPlaceholder
	if len(local) > 2 {
		masked = local[:1] + maskedPlaceholder + local[len(local)-1:]
	}

	labels := strings.Split(domain, ".")
	for i := 0; i < len(labels)-1; i++ {
		if labels[i] == "" {
			continue
		}
		labels[i] = labels[i][:1] + maskedPlaceholder
	}
	return masked + "@" + strings.Join(labels, ".")
}

// MaskUsername returns the audit-log form of a username: its first character
// followed by "***". An empty username is masked completely.
func MaskUsername(u string) string {
	if u == "" {
		return maskedPlaceholder
	}
	return u[:1] + maskedPlaceholder
}

// MaskToken returns the audit-log form of an API token: the stored 8-character
// display prefix followed by an ellipsis, matching the AI.md PART 11
// "Token/ID Masking" rule. The rest of the token never appears anywhere.
func MaskToken(token string) string {
	return TokenPrefixDisplay(token) + "..."
}

// MaskIP applies the query-log IP anonymization IDEA.md requires: an IPv4
// address has its final octet zeroed ("192.168.1.100" becomes "192.168.1.0")
// and an IPv6 address keeps its first 48 bits with the remainder zeroed
// ("2001:db8:1:2::1" becomes "2001:db8:1::"). An IPv4-mapped IPv6 address is
// unmapped first and treated as IPv4. Input that is not an IP address returns
// an empty string rather than being passed through.
func MaskIP(ip string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	bits := 48
	if addr.Is4() {
		bits = 24
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.Addr().String()
}
