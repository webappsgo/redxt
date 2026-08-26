package user

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// Name length bounds from AI.md PART 34, "Username & Email Rules". The
// same bounds apply to an organization slug because PART 35 gives users
// and organizations a single shared namespace.
const (
	// MinNameLength is the shortest accepted username or org slug.
	MinNameLength = 2
	// MaxNameLength is the longest accepted username or org slug.
	MaxNameLength = 39
)

// Email length bounds from PART 34. The total is the RFC 5321 limit; the
// local and domain halves carry their own ceilings.
const (
	// MaxEmailLength is the longest accepted address.
	MaxEmailLength = 254
	// MaxEmailLocalLength is the longest accepted local part.
	MaxEmailLocalLength = 64
	// MaxEmailDomainLength is the longest accepted domain part.
	MaxEmailDomainLength = 255
)

// Validation failures. Every message is safe to show a caller: none of
// them reveals whether an account exists.
var (
	// ErrNameLength reports a username or slug outside the length bounds.
	ErrNameLength = errors.New("user: name must be 2 to 39 characters")
	// ErrNameFormat reports a name that is not lowercase alphanumeric
	// with single internal hyphens.
	ErrNameFormat = errors.New("user: name may contain only letters, numbers, and single hyphens between them")
	// ErrNameReserved reports a name on the reserved list.
	ErrNameReserved = errors.New("user: that name is reserved")
	// ErrEmailFormat reports an address that is not a deliverable form.
	ErrEmailFormat = errors.New("user: enter a valid email address")
	// ErrEmailLength reports an address longer than the RFC 5321 limit.
	ErrEmailLength = errors.New("user: email address is too long")
	// ErrEmailDomainNotAllowed reports an address whose domain is outside
	// the configured allow list or inside the block list.
	ErrEmailDomainNotAllowed = errors.New("user: that email domain is not allowed")
	// ErrPasswordLength reports a password shorter than the policy.
	ErrPasswordLength = errors.New("user: password is too short")
	// ErrPasswordComposition reports a password missing a required
	// character class.
	ErrPasswordComposition = errors.New("user: password must include the required character types")
	// ErrDomainFormat reports a custom domain that is not a hostname.
	ErrDomainFormat = errors.New("user: enter a valid domain name")
)

// namePattern is the PART 34 username rule: lowercase alphanumeric,
// hyphens allowed only between other characters.
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// emailLocalPattern and emailDomainPattern implement the PART 34 email
// rules. They are deliberately narrower than RFC 5322: an address that
// cannot survive a round trip through a mail log is not worth accepting.
var (
	emailLocalPattern  = regexp.MustCompile(`^[a-z0-9.+_-]+$`)
	emailDomainPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]\.[a-z]{2,}$`)
)

// domainPattern matches a custom domain label sequence, optionally
// wildcarded, per PART 36's apex, subdomain, and wildcard forms.
var domainPattern = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)

// NormalizeName lowercases and trims a username or organization slug.
// Names are compared and stored lowercased, so every lookup path must
// run its input through this first.
func NormalizeName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// NormalizeEmail lowercases and trims an address. Addresses are
// case-insensitive per PART 34 and are stored lowercased.
func NormalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// ValidateName checks a username or organization slug against the PART
// 34 rules and the shared reserved namespace, returning the normalized
// value. Both account kinds use this one function because they occupy
// the same namespace and a divergence between them would let an
// organization claim a name a user could not.
func ValidateName(raw string) (string, error) {
	name := NormalizeName(raw)
	if len(name) < MinNameLength || len(name) > MaxNameLength {
		return "", ErrNameLength
	}
	if !namePattern.MatchString(name) {
		return "", ErrNameFormat
	}
	if strings.Contains(name, "--") {
		return "", ErrNameFormat
	}
	if IsBlocked(name) {
		return "", ErrNameReserved
	}
	return name, nil
}

// ValidateEmail checks an address against the PART 34 rules and returns
// the normalized value.
func ValidateEmail(raw string) (string, error) {
	email := NormalizeEmail(raw)
	if email == "" || len(email) > MaxEmailLength {
		return "", ErrEmailLength
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", ErrEmailFormat
	}
	local, domain := email[:at], email[at+1:]
	if len(local) > MaxEmailLocalLength || len(domain) > MaxEmailDomainLength {
		return "", ErrEmailLength
	}
	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") {
		return "", ErrEmailFormat
	}
	if strings.Contains(email, "..") {
		return "", ErrEmailFormat
	}
	if !emailLocalPattern.MatchString(local) {
		return "", ErrEmailFormat
	}
	if !emailDomainPattern.MatchString(domain) {
		return "", ErrEmailFormat
	}
	return email, nil
}

// EmailDomainAllowed applies the configured allow and block lists to an
// already-validated address. An empty allow list permits every domain
// that is not blocked, which is the PART 34 default.
func EmailDomainAllowed(email string, allowed, blocked []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, entry := range blocked {
		if strings.EqualFold(strings.TrimSpace(entry), domain) {
			return false
		}
	}
	if len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if strings.EqualFold(strings.TrimSpace(entry), domain) {
			return true
		}
	}
	return false
}

// PasswordPolicy is the configured password strength rule set from
// server.users.auth in PART 34.
type PasswordPolicy struct {
	// MinLength is the shortest accepted password.
	MinLength int
	// RequireUppercase demands at least one uppercase letter.
	RequireUppercase bool
	// RequireLowercase demands at least one lowercase letter.
	RequireLowercase bool
	// RequireNumber demands at least one digit.
	RequireNumber bool
	// RequireSpecial demands at least one non-alphanumeric character.
	RequireSpecial bool
}

// ValidatePassword checks a password against the policy. It never
// reports which specific class was missing beyond the single composition
// error, so a caller cannot use the response to map the policy of a
// server it is attacking any more precisely than the policy already
// published to legitimate users.
func (p PasswordPolicy) ValidatePassword(password string) error {
	minLength := p.MinLength
	if minLength <= 0 {
		minLength = 8
	}
	if len([]rune(password)) < minLength {
		return ErrPasswordLength
	}
	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasNumber = true
		default:
			hasSpecial = true
		}
	}
	if p.RequireUppercase && !hasUpper {
		return ErrPasswordComposition
	}
	if p.RequireLowercase && !hasLower {
		return ErrPasswordComposition
	}
	if p.RequireNumber && !hasNumber {
		return ErrPasswordComposition
	}
	if p.RequireSpecial && !hasSpecial {
		return ErrPasswordComposition
	}
	return nil
}

// Identifier kinds a login form accepts, per the PART 34 "Login
// Identifier" table.
const (
	// IdentifierID is an all-digit user id.
	IdentifierID = "id"
	// IdentifierEmail is an address, detected by the "@".
	IdentifierEmail = "email"
	// IdentifierUsername is everything else.
	IdentifierUsername = "username"
)

// DetectIdentifier classifies a login identifier: all digits is a user
// id, anything containing "@" is an email, and the rest is a username.
func DetectIdentifier(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return IdentifierUsername
	}
	if strings.Contains(trimmed, "@") {
		return IdentifierEmail
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return IdentifierUsername
		}
	}
	return IdentifierID
}

// ValidateDomain checks a custom domain against the PART 36 rules and
// returns the normalized, lowercased, trailing-dot-free value.
func ValidateDomain(raw string) (string, error) {
	domain := strings.TrimSuffix(NormalizeName(raw), ".")
	if domain == "" || len(domain) > MaxEmailDomainLength {
		return "", ErrDomainFormat
	}
	if !domainPattern.MatchString(domain) {
		return "", ErrDomainFormat
	}
	for _, label := range strings.Split(strings.TrimPrefix(domain, "*."), ".") {
		if label == "" || len(label) > 63 {
			return "", ErrDomainFormat
		}
	}
	return domain, nil
}

// IsWildcardDomain reports whether a validated domain is a wildcard.
func IsWildcardDomain(domain string) bool {
	return strings.HasPrefix(domain, "*.")
}

// IsApexDomain reports whether a validated domain is a registrable apex
// rather than a subdomain. The test is structural — two labels — which
// is what PART 36's allow_apex switch means in practice for the common
// single-label public suffixes.
func IsApexDomain(domain string) bool {
	if IsWildcardDomain(domain) {
		return false
	}
	return strings.Count(domain, ".") == 1
}
