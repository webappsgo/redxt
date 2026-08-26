package security

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fixedRandom is a 32-character alphanumeric run used to build syntactically
// valid tokens in tests.
const fixedRandom = "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"

func TestParseScope(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Scope
		wantErr bool
	}{
		{name: "empty defaults to global", in: "", want: ScopeGlobal},
		{name: "global", in: "global", want: ScopeGlobal},
		{name: "read-write", in: "read-write", want: ScopeReadWrite},
		{name: "read", in: "read", want: ScopeRead},
		{name: "unknown", in: "admin", wantErr: true},
		{name: "wrong case", in: "Global", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScope(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseScope(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseScope(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseScope(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRandomString(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		wantErr bool
	}{
		{name: "token length", n: RandomLength},
		{name: "single character", n: 1},
		{name: "long", n: 512},
		{name: "zero", n: 0, wantErr: true},
		{name: "negative", n: -5, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RandomString(tt.n)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("RandomString(%d) = %q, want error", tt.n, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RandomString(%d) unexpected error: %v", tt.n, err)
			}
			if len(got) != tt.n {
				t.Fatalf("RandomString(%d) length = %d, want %d", tt.n, len(got), tt.n)
			}
			if !isAlphanumeric(got) {
				t.Fatalf("RandomString(%d) = %q, contains characters outside the alphabet", tt.n, got)
			}
		})
	}
}

func TestRandomStringDiffersBetweenCalls(t *testing.T) {
	first, err := RandomString(RandomLength)
	if err != nil {
		t.Fatalf("first RandomString: %v", err)
	}
	second, err := RandomString(RandomLength)
	if err != nil {
		t.Fatalf("second RandomString: %v", err)
	}
	if first == second {
		t.Fatalf("two RandomString calls returned the same value %q", first)
	}
}

func TestRandomStringCoversAlphabet(t *testing.T) {
	seen := make(map[rune]bool, len(alphabet))
	for i := 0; i < 200; i++ {
		s, err := RandomString(RandomLength)
		if err != nil {
			t.Fatalf("RandomString: %v", err)
		}
		for _, r := range s {
			seen[r] = true
		}
	}
	if len(seen) != len(alphabet) {
		t.Fatalf("observed %d distinct characters over 6400 samples, want %d", len(seen), len(alphabet))
	}
}

func TestGenerateToken(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{name: "admin", prefix: PrefixAdmin},
		{name: "user", prefix: PrefixUser},
		{name: "org", prefix: PrefixOrg},
		{name: "admin agent", prefix: PrefixAdminAgent},
		{name: "user agent", prefix: PrefixUserAgent},
		{name: "org agent", prefix: PrefixOrgAgent},
		{name: "unknown prefix", prefix: "svc_", wantErr: true},
		{name: "empty prefix", prefix: "", wantErr: true},
		{name: "missing underscore", prefix: "adm", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateToken(tt.prefix)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GenerateToken(%q) = %q, want error", tt.prefix, got)
				}
				if !errors.Is(err, ErrUnknownTokenType) {
					t.Fatalf("GenerateToken(%q) error = %v, want ErrUnknownTokenType", tt.prefix, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GenerateToken(%q) unexpected error: %v", tt.prefix, err)
			}
			if len(got) != len(tt.prefix)+RandomLength {
				t.Fatalf("GenerateToken(%q) length = %d, want %d", tt.prefix, len(got), len(tt.prefix)+RandomLength)
			}
			info, err := ParseToken(got)
			if err != nil {
				t.Fatalf("ParseToken(%q) unexpected error: %v", got, err)
			}
			if info.Prefix != tt.prefix {
				t.Fatalf("ParseToken(%q) prefix = %q, want %q", got, info.Prefix, tt.prefix)
			}
		})
	}
}

func TestParseToken(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantOwner OwnerType
		wantAgent bool
		wantPfx   string
		wantErr   error
	}{
		{
			name:      "admin token",
			token:     PrefixAdmin + fixedRandom,
			wantOwner: OwnerAdmin,
			wantPfx:   PrefixAdmin,
		},
		{
			name:      "user token",
			token:     PrefixUser + fixedRandom,
			wantOwner: OwnerUser,
			wantPfx:   PrefixUser,
		},
		{
			name:      "org token",
			token:     PrefixOrg + fixedRandom,
			wantOwner: OwnerOrg,
			wantPfx:   PrefixOrg,
		},
		{
			name:      "admin agent token is not a plain admin token",
			token:     PrefixAdminAgent + fixedRandom,
			wantOwner: OwnerAdmin,
			wantAgent: true,
			wantPfx:   PrefixAdminAgent,
		},
		{
			name:      "user agent token",
			token:     PrefixUserAgent + fixedRandom,
			wantOwner: OwnerUser,
			wantAgent: true,
			wantPfx:   PrefixUserAgent,
		},
		{
			name:      "org agent token",
			token:     PrefixOrgAgent + fixedRandom,
			wantOwner: OwnerOrg,
			wantAgent: true,
			wantPfx:   PrefixOrgAgent,
		},
		{
			name:    "random part too short",
			token:   PrefixAdmin + fixedRandom[:31],
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "random part too long",
			token:   PrefixAdmin + fixedRandom + "x",
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "agent random part too short",
			token:   PrefixAdminAgent + fixedRandom[:20],
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "invalid characters",
			token:   PrefixAdmin + "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p-",
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "agent invalid characters",
			token:   PrefixOrgAgent + "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p!",
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "unknown prefix",
			token:   "svc_" + fixedRandom,
			wantErr: ErrUnknownTokenType,
		},
		{
			name:    "no underscore",
			token:   fixedRandom,
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "empty string",
			token:   "",
			wantErr: ErrInvalidTokenFormat,
		},
		{
			name:    "prefix only",
			token:   PrefixAdmin,
			wantErr: ErrInvalidTokenFormat,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseToken(tt.token)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseToken(%q) error = %v, want %v", tt.token, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseToken(%q) unexpected error: %v", tt.token, err)
			}
			if got.OwnerType != tt.wantOwner {
				t.Fatalf("ParseToken(%q) owner = %q, want %q", tt.token, got.OwnerType, tt.wantOwner)
			}
			if got.IsAgent != tt.wantAgent {
				t.Fatalf("ParseToken(%q) isAgent = %v, want %v", tt.token, got.IsAgent, tt.wantAgent)
			}
			if got.Prefix != tt.wantPfx {
				t.Fatalf("ParseToken(%q) prefix = %q, want %q", tt.token, got.Prefix, tt.wantPfx)
			}
			if got.Random != fixedRandom {
				t.Fatalf("ParseToken(%q) random = %q, want %q", tt.token, got.Random, fixedRandom)
			}
		})
	}
}

func TestHashToken(t *testing.T) {
	// Known SHA-256 vector for the empty string.
	const emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "empty string known vector", token: "", want: emptyDigest},
		{
			name:  "abc known vector",
			token: "abc",
			want:  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashToken(tt.token)
			if got != tt.want {
				t.Fatalf("HashToken(%q) = %q, want %q", tt.token, got, tt.want)
			}
			if len(got) != 64 {
				t.Fatalf("HashToken(%q) length = %d, want 64", tt.token, len(got))
			}
			if got != strings.ToLower(got) {
				t.Fatalf("HashToken(%q) = %q, want lowercase hex", tt.token, got)
			}
		})
	}
}

func TestHashTokenIsStable(t *testing.T) {
	token := PrefixAdmin + fixedRandom
	first := HashToken(token)
	second := HashToken(token)
	if first != second {
		t.Fatal("HashToken is not deterministic")
	}
	if HashToken(token) == HashToken(PrefixUser+fixedRandom) {
		t.Fatal("different tokens produced the same hash")
	}
}

func TestTokenPrefixDisplay(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "admin token", token: PrefixAdmin + fixedRandom, want: "adm_a1b2"},
		{name: "agent token", token: PrefixAdminAgent + fixedRandom, want: "adm_agt_"},
		{name: "exactly eight", token: "adm_a1b2", want: "adm_a1b2"},
		{name: "shorter than eight", token: "adm_", want: "adm_"},
		{name: "empty", token: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TokenPrefixDisplay(tt.token); got != tt.want {
				t.Fatalf("TokenPrefixDisplay(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestVerifyTokenHash(t *testing.T) {
	token := PrefixAdmin + fixedRandom
	stored := HashToken(token)
	tests := []struct {
		name   string
		token  string
		stored string
		want   bool
	}{
		{name: "matching token", token: token, stored: stored, want: true},
		{name: "wrong token", token: PrefixUser + fixedRandom, stored: stored, want: false},
		{name: "empty stored hash", token: token, stored: "", want: false},
		{name: "truncated stored hash", token: token, stored: stored[:63], want: false},
		{name: "uppercase stored hash", token: token, stored: strings.ToUpper(stored), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyTokenHash(tt.token, tt.stored); got != tt.want {
				t.Fatalf("VerifyTokenHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpiresAt(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name    string
		option  string
		want    time.Time
		wantSet bool
		wantErr bool
	}{
		{name: "never", option: "never", want: time.Time{}, wantSet: false},
		{name: "7days", option: "7days", want: now.Add(7 * 24 * time.Hour), wantSet: true},
		{name: "1month", option: "1month", want: now.Add(30 * 24 * time.Hour), wantSet: true},
		{name: "6months", option: "6months", want: now.Add(180 * 24 * time.Hour), wantSet: true},
		{name: "1year", option: "1year", want: now.Add(365 * 24 * time.Hour), wantSet: true},
		{name: "unknown option", option: "2weeks", wantErr: true},
		{name: "empty option", option: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, set, err := ExpiresAt(now, tt.option)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExpiresAt(%q) = (%v, %v, nil), want error", tt.option, got, set)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpiresAt(%q) unexpected error: %v", tt.option, err)
			}
			if set != tt.wantSet {
				t.Fatalf("ExpiresAt(%q) set = %v, want %v", tt.option, set, tt.wantSet)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("ExpiresAt(%q) = %v, want %v", tt.option, got, tt.want)
			}
		})
	}
}

func TestGenerateSetupToken(t *testing.T) {
	first, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken: %v", err)
	}
	if len(first) != SetupTokenLength {
		t.Fatalf("GenerateSetupToken length = %d, want %d", len(first), SetupTokenLength)
	}
	for _, r := range first {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("GenerateSetupToken = %q, contains non-hex character %q", first, r)
		}
	}
	second, err := GenerateSetupToken()
	if err != nil {
		t.Fatalf("second GenerateSetupToken: %v", err)
	}
	if first == second {
		t.Fatalf("two setup tokens were identical: %q", first)
	}
}
