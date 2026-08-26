// Package security implements the cryptographic primitives redxt relies on:
// API token generation/validation, Argon2id password hashing, AES-256-GCM
// at-rest encryption, project-level HMAC secrets, and log/audit redaction.
//
// The authoritative specification for this package is AI.md PART 11
// ("API Token Security", "Cryptographic Keys", "Server Encryption Key",
// "Output Sanitization Pipeline", "Audit Log Rules"). Every rule implemented
// here traces back to that PART; nothing in this package may log, return, or
// otherwise expose a secret value.
package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OwnerType identifies which principal a token belongs to. It matches the
// tokens.owner_type column defined in AI.md PART 11.
type OwnerType string

const (
	// OwnerAdmin marks a token owned by a server admin.
	OwnerAdmin OwnerType = "admin"
	// OwnerUser marks a token owned by a regular user (multi-user builds).
	OwnerUser OwnerType = "user"
	// OwnerOrg marks a token owned by an organization.
	OwnerOrg OwnerType = "org"
)

const (
	// PrefixAdmin is the token prefix for server admin API tokens.
	PrefixAdmin = "adm_"
	// PrefixUser is the token prefix for regular user API tokens.
	PrefixUser = "usr_"
	// PrefixOrg is the token prefix for organization API tokens.
	PrefixOrg = "org_"
	// PrefixAdminAgent is the compound prefix for admin (server
	// infrastructure) agent tokens.
	PrefixAdminAgent = "adm_agt_"
	// PrefixUserAgent is the compound prefix for a user's personal agent
	// tokens.
	PrefixUserAgent = "usr_agt_"
	// PrefixOrgAgent is the compound prefix for organization agent tokens.
	PrefixOrgAgent = "org_agt_"
)

// Scope is the permission scope attached to a token.
type Scope string

const (
	// ScopeGlobal grants every permission the token owner holds.
	ScopeGlobal Scope = "global"
	// ScopeReadWrite grants read and write operations, but no delete and no
	// admin actions.
	ScopeReadWrite Scope = "read-write"
	// ScopeRead grants read-only operations.
	ScopeRead Scope = "read"
)

// ErrInvalidScope is returned by ParseScope for a value that is not one of the
// three documented scopes.
var ErrInvalidScope = errors.New("security: invalid token scope")

// ParseScope converts a stored or user-supplied scope string into a Scope. An
// empty string defaults to ScopeGlobal, matching the schema default in
// AI.md PART 11.
func ParseScope(s string) (Scope, error) {
	switch Scope(s) {
	case "":
		return ScopeGlobal, nil
	case ScopeGlobal:
		return ScopeGlobal, nil
	case ScopeReadWrite:
		return ScopeReadWrite, nil
	case ScopeRead:
		return ScopeRead, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidScope, s)
	}
}

// RandomLength is the number of random alphanumeric characters that follow a
// token prefix.
const RandomLength = 32

// alphabet is the alphanumeric character set token random parts are drawn from.
const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var (
	// ErrInvalidTokenFormat means the token did not match
	// {prefix}_{random_32_alphanumeric}.
	ErrInvalidTokenFormat = errors.New("security: invalid token format")
	// ErrUnknownTokenType means the token prefix is not one of the six
	// documented prefixes.
	ErrUnknownTokenType = errors.New("security: unknown token type")
	// ErrInvalidLength is returned when a caller asks for a non-positive
	// amount of random material.
	ErrInvalidLength = errors.New("security: length must be positive")
)

// RandomString returns n characters drawn uniformly from alphabet using
// crypto/rand. Rejection sampling is used instead of a modulo reduction so
// every character is equally likely; modulo would bias the first
// 256 % len(alphabet) characters of the alphabet.
func RandomString(n int) (string, error) {
	if n <= 0 {
		return "", ErrInvalidLength
	}
	// Any byte at or above this limit would land in a partial cycle of the
	// alphabet and is therefore discarded.
	limit := byte(256 - (256 % len(alphabet)))
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("security: read random bytes: %w", err)
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, alphabet[int(b)%len(alphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}

// GenerateToken returns a new token of the form {prefix}{32 random
// alphanumerics}. The prefix must be one of the six documented token prefixes.
// The returned value is the only time the plaintext token exists; callers must
// hash it with HashToken for storage and show the plaintext exactly once.
func GenerateToken(prefix string) (string, error) {
	switch prefix {
	case PrefixAdmin, PrefixUser, PrefixOrg,
		PrefixAdminAgent, PrefixUserAgent, PrefixOrgAgent:
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownTokenType, prefix)
	}
	random, err := RandomString(RandomLength)
	if err != nil {
		return "", err
	}
	return prefix + random, nil
}

// HashToken returns the lowercase hex SHA-256 digest of a token, which is what
// the tokens.token_hash column stores.
//
// A plain SHA-256 (rather than a password KDF such as Argon2id) is the correct
// and specified choice here: tokens are 32 characters drawn uniformly from a
// 62-character alphabet, giving ~190 bits of entropy, so there is no offline
// guessing attack for a KDF to slow down. Password hashing exists to defend
// low-entropy human input; tokens are not that.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokenPrefixDisplay returns the first 8 characters of a token (for example
// "adm_a1b2"), which is what the tokens.token_prefix column stores.
//
// This truncated form is the ONLY part of a token that may ever be displayed
// in the UI, written to a log, or recorded in the audit trail after creation.
// The full token is shown once at creation and never again. Inputs shorter
// than 8 characters are returned unchanged.
func TokenPrefixDisplay(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

// VerifyTokenHash reports whether token hashes to storedHash. The comparison is
// constant time so a caller cannot learn the stored digest by timing.
func VerifyTokenHash(token, storedHash string) bool {
	computed := HashToken(token)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// Info describes a syntactically valid token: its prefix, the owner type that
// prefix maps to, whether it is an agent token, and the random portion.
type Info struct {
	Prefix    string
	OwnerType OwnerType
	IsAgent   bool
	Random    string
}

// ParseToken validates a token's syntax and reports what kind of token it is.
// It performs no database lookup; callers must still resolve the hash against
// the tokens table.
//
// The dispatch order follows AI.md PART 11 exactly: the three compound agent
// prefixes are tested first so that "adm_agt_..." is recognised as an admin
// agent token and never mis-parsed as a plain "adm_" token whose random part
// happens to begin with "agt_".
func ParseToken(token string) (Info, error) {
	switch {
	case strings.HasPrefix(token, PrefixAdminAgent):
		return parseAgentToken(token, PrefixAdminAgent, OwnerAdmin)
	case strings.HasPrefix(token, PrefixUserAgent):
		return parseAgentToken(token, PrefixUserAgent, OwnerUser)
	case strings.HasPrefix(token, PrefixOrgAgent):
		return parseAgentToken(token, PrefixOrgAgent, OwnerOrg)
	}

	parts := strings.SplitN(token, "_", 2)
	if len(parts) != 2 || len(parts[1]) != RandomLength {
		return Info{}, ErrInvalidTokenFormat
	}
	prefix := parts[0] + "_"
	var owner OwnerType
	switch prefix {
	case PrefixAdmin:
		owner = OwnerAdmin
	case PrefixUser:
		owner = OwnerUser
	case PrefixOrg:
		owner = OwnerOrg
	default:
		return Info{}, ErrUnknownTokenType
	}
	if !isAlphanumeric(parts[1]) {
		return Info{}, ErrInvalidTokenFormat
	}
	return Info{Prefix: prefix, OwnerType: owner, IsAgent: false, Random: parts[1]}, nil
}

// parseAgentToken validates the random portion that follows a compound agent
// prefix.
func parseAgentToken(token, prefix string, owner OwnerType) (Info, error) {
	random := strings.TrimPrefix(token, prefix)
	if len(random) != RandomLength || !isAlphanumeric(random) {
		return Info{}, ErrInvalidTokenFormat
	}
	return Info{Prefix: prefix, OwnerType: owner, IsAgent: true, Random: random}, nil
}

// isAlphanumeric reports whether every character of s belongs to alphabet.
func isAlphanumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(alphabet, rune(s[i])) {
			return false
		}
	}
	return true
}

// ExpirationOptions maps the token expiration choices offered in the UI to
// their durations. "never" maps to a zero duration, which is stored as NULL in
// tokens.expires_at.
var ExpirationOptions = map[string]time.Duration{
	"never":   0,
	"7days":   7 * 24 * time.Hour,
	"1month":  30 * 24 * time.Hour,
	"6months": 180 * 24 * time.Hour,
	"1year":   365 * 24 * time.Hour,
}

// ErrUnknownExpiration is returned for an expiration option that is not in
// ExpirationOptions.
var ErrUnknownExpiration = errors.New("security: unknown expiration option")

// ExpiresAt resolves an expiration option against now. The boolean reports
// whether an expiry applies: "never" returns (zero time, false, nil) so the
// caller writes NULL, every other known option returns (now+d, true, nil).
func ExpiresAt(now time.Time, option string) (time.Time, bool, error) {
	d, ok := ExpirationOptions[option]
	if !ok {
		return time.Time{}, false, fmt.Errorf("%w: %q", ErrUnknownExpiration, option)
	}
	if d == 0 {
		return time.Time{}, false, nil
	}
	return now.Add(d), true, nil
}

// SetupTokenLength is the number of hex characters in a first-run setup token.
const SetupTokenLength = 32

// GenerateSetupToken returns the one-time first-run setup token described in
// AI.md PART 8: 16 bytes of crypto/rand rendered as 32 hexadecimal characters.
// It is displayed once on the console at first start and is not a prefixed API
// token.
func GenerateSetupToken() (string, error) {
	buf := make([]byte, SetupTokenLength/2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("security: read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
