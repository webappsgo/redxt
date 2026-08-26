package user

import (
	"errors"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		err   error
	}{
		{name: "simple", input: "johndoe", want: "johndoe"},
		{name: "uppercase is lowered", input: "JohnDoe", want: "johndoe"},
		{name: "surrounding space trimmed", input: "  johndoe  ", want: "johndoe"},
		{name: "internal hyphen", input: "john-doe", want: "john-doe"},
		{name: "digits", input: "user1234", want: "user1234"},
		{name: "minimum length", input: "ab", want: "ab"},
		{name: "too short", input: "a", err: ErrNameLength},
		{name: "too long", input: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", err: ErrNameLength},
		{name: "leading hyphen", input: "-john", err: ErrNameFormat},
		{name: "trailing hyphen", input: "john-", err: ErrNameFormat},
		{name: "consecutive hyphens", input: "john--doe", err: ErrNameFormat},
		{name: "underscore", input: "john_doe", err: ErrNameFormat},
		{name: "dot", input: "john.doe", err: ErrNameFormat},
		{name: "unicode", input: "jöhn", err: ErrNameFormat},
		{name: "reserved exact", input: "support", err: ErrNameReserved},
		{name: "reserved substring", input: "superadmin", err: ErrNameReserved},
		{name: "reserved project name", input: "redxt", err: ErrNameReserved},
		{name: "reserved service subdomain", input: "ddns", err: ErrNameReserved},
		{name: "empty", input: "", err: ErrNameLength},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateName(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ValidateName(%q) error = %v, want %v", tc.input, err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("ValidateName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	long := make([]byte, 250)
	for i := range long {
		long[i] = 'a'
	}

	tests := []struct {
		name  string
		input string
		want  string
		err   error
	}{
		{name: "simple", input: "me@example.com", want: "me@example.com"},
		{name: "case folded", input: "Me@Example.COM", want: "me@example.com"},
		{name: "plus tag", input: "me+tag@example.com", want: "me+tag@example.com"},
		{name: "subdomain", input: "me@mail.example.co.uk", want: "me@mail.example.co.uk"},
		{name: "no at", input: "example.com", err: ErrEmailFormat},
		{name: "no local", input: "@example.com", err: ErrEmailFormat},
		{name: "no domain", input: "me@", err: ErrEmailFormat},
		{name: "no tld", input: "me@example", err: ErrEmailFormat},
		{name: "double dot", input: "me..you@example.com", err: ErrEmailFormat},
		{name: "leading dot", input: ".me@example.com", err: ErrEmailFormat},
		{name: "trailing dot in local", input: "me.@example.com", err: ErrEmailFormat},
		{name: "space", input: "me you@example.com", err: ErrEmailFormat},
		{name: "empty", input: "", err: ErrEmailLength},
		{name: "over length", input: string(long) + "@example.com", err: ErrEmailLength},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateEmail(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ValidateEmail(%q) error = %v, want %v", tc.input, err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("ValidateEmail(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		allowed []string
		blocked []string
		want    bool
	}{
		{name: "no lists", email: "me@example.com", want: true},
		{name: "allowed match", email: "me@example.com", allowed: []string{"example.com"}, want: true},
		{name: "allowed miss", email: "me@other.com", allowed: []string{"example.com"}, want: false},
		{name: "blocked match", email: "me@example.com", blocked: []string{"example.com"}, want: false},
		{name: "block beats allow", email: "me@example.com", allowed: []string{"example.com"}, blocked: []string{"example.com"}, want: false},
		{name: "case insensitive", email: "me@example.com", allowed: []string{"EXAMPLE.COM"}, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := EmailDomainAllowed(tc.email, tc.allowed, tc.blocked); got != tc.want {
				t.Fatalf("EmailDomainAllowed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPasswordPolicy(t *testing.T) {
	strict := PasswordPolicy{
		MinLength:        10,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumber:    true,
		RequireSpecial:   true,
	}

	tests := []struct {
		name     string
		policy   PasswordPolicy
		password string
		err      error
	}{
		{name: "default minimum met", policy: PasswordPolicy{}, password: "abcdefgh"},
		{name: "default minimum missed", policy: PasswordPolicy{}, password: "abcdefg", err: ErrPasswordLength},
		{name: "strict satisfied", policy: strict, password: "Abcdefg1!x"},
		{name: "strict too short", policy: strict, password: "Ab1!xyz", err: ErrPasswordLength},
		{name: "strict no upper", policy: strict, password: "abcdefg1!x", err: ErrPasswordComposition},
		{name: "strict no lower", policy: strict, password: "ABCDEFG1!X", err: ErrPasswordComposition},
		{name: "strict no digit", policy: strict, password: "Abcdefgh!x", err: ErrPasswordComposition},
		{name: "strict no special", policy: strict, password: "Abcdefg1xy", err: ErrPasswordComposition},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.policy.ValidatePassword(tc.password); !errors.Is(err, tc.err) {
				t.Fatalf("ValidatePassword error = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestDetectIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "12345", want: IdentifierID},
		{input: "john@example.com", want: IdentifierEmail},
		{input: "johndoe", want: IdentifierUsername},
		{input: "john123", want: IdentifierUsername},
		{input: "", want: IdentifierUsername},
		{input: "  42  ", want: IdentifierID},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := DetectIdentifier(tc.input); got != tc.want {
				t.Fatalf("DetectIdentifier(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		err   error
	}{
		{name: "apex", input: "example.com", want: "example.com"},
		{name: "subdomain", input: "api.example.com", want: "api.example.com"},
		{name: "wildcard", input: "*.example.com", want: "*.example.com"},
		{name: "trailing dot removed", input: "example.com.", want: "example.com"},
		{name: "uppercase lowered", input: "Example.COM", want: "example.com"},
		{name: "no tld", input: "example", err: ErrDomainFormat},
		{name: "leading hyphen label", input: "-bad.example.com", err: ErrDomainFormat},
		{name: "space", input: "bad domain.com", err: ErrDomainFormat},
		{name: "empty", input: "", err: ErrDomainFormat},
		{name: "interior wildcard", input: "a.*.example.com", err: ErrDomainFormat},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateDomain(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ValidateDomain(%q) error = %v, want %v", tc.input, err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("ValidateDomain(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDomainShape(t *testing.T) {
	tests := []struct {
		domain     string
		isWildcard bool
		isApex     bool
	}{
		{domain: "example.com", isApex: true},
		{domain: "api.example.com"},
		{domain: "*.example.com", isWildcard: true},
	}

	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			if got := IsWildcardDomain(tc.domain); got != tc.isWildcard {
				t.Fatalf("IsWildcardDomain(%q) = %v, want %v", tc.domain, got, tc.isWildcard)
			}
			if got := IsApexDomain(tc.domain); got != tc.isApex {
				t.Fatalf("IsApexDomain(%q) = %v, want %v", tc.domain, got, tc.isApex)
			}
		})
	}
}
