package security

import (
	"errors"
	"strings"
	"testing"
)

// testParams are deliberately low-cost so the test suite stays fast; they are
// never used outside tests.
func testParams() Argon2Params {
	return Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLength: 32, SaltLength: 16}
}

func TestDefaultParams(t *testing.T) {
	p := DefaultParams()
	want := Argon2Params{Time: 3, Memory: 64 * 1024, Threads: 4, KeyLength: 32, SaltLength: 16}
	if p != want {
		t.Fatalf("DefaultParams() = %+v, want %+v", p, want)
	}
}

func TestHashPasswordRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{name: "ascii", password: "correct horse battery staple"},
		{name: "empty", password: ""},
		{name: "unicode", password: "pässwörd-日本語-🔐"},
		{name: "long", password: strings.Repeat("x", 512)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := HashPasswordWithParams(tt.password, testParams())
			if err != nil {
				t.Fatalf("HashPasswordWithParams: %v", err)
			}
			if !strings.HasPrefix(encoded, "$argon2id$v=19$") {
				t.Fatalf("encoded hash = %q, want $argon2id$v=19$ prefix", encoded)
			}
			if strings.Contains(encoded, tt.password) && tt.password != "" {
				t.Fatal("encoded hash leaks the plaintext password")
			}
			ok, err := VerifyPassword(tt.password, encoded)
			if err != nil {
				t.Fatalf("VerifyPassword: %v", err)
			}
			if !ok {
				t.Fatal("VerifyPassword returned false for the correct password")
			}
		})
	}
}

func TestVerifyPasswordWrongPassword(t *testing.T) {
	encoded, err := HashPasswordWithParams("right-password", testParams())
	if err != nil {
		t.Fatalf("HashPasswordWithParams: %v", err)
	}
	tests := []struct {
		name     string
		password string
	}{
		{name: "different password", password: "wrong-password"},
		{name: "empty password", password: ""},
		{name: "prefix of the password", password: "right"},
		{name: "case difference", password: "Right-Password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyPassword(tt.password, encoded)
			if err != nil {
				t.Fatalf("VerifyPassword returned an error for a wrong password: %v", err)
			}
			if ok {
				t.Fatal("VerifyPassword accepted a wrong password")
			}
		})
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		wantErr error
	}{
		{name: "empty string", encoded: "", wantErr: ErrInvalidHash},
		{name: "not phc", encoded: "plaintext", wantErr: ErrInvalidHash},
		{
			name:    "wrong algorithm",
			encoded: "$argon2i$v=19$m=8192,t=1,p=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrIncompatibleAlgorithm,
		},
		{
			name:    "wrong version",
			encoded: "$argon2id$v=16$m=8192,t=1,p=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrIncompatibleVersion,
		},
		{
			name:    "missing version field",
			encoded: "$argon2id$m=8192,t=1,p=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "unparsable version",
			encoded: "$argon2id$v=abc$m=8192,t=1,p=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "missing cost field",
			encoded: "$argon2id$v=19$m=8192,t=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "unknown cost field",
			encoded: "$argon2id$v=19$m=8192,t=1,x=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "zero cost field",
			encoded: "$argon2id$v=19$m=8192,t=0,p=1$c29tZXNhbHRzYWx0MTY$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "bad base64 salt",
			encoded: "$argon2id$v=19$m=8192,t=1,p=1$not base64!$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "bad base64 digest",
			encoded: "$argon2id$v=19$m=8192,t=1,p=1$c29tZXNhbHRzYWx0MTY$not base64!",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "empty salt",
			encoded: "$argon2id$v=19$m=8192,t=1,p=1$$c29tZWRpZ2VzdA",
			wantErr: ErrInvalidHash,
		},
		{
			name:    "too few segments",
			encoded: "$argon2id$v=19$m=8192,t=1,p=1$c29tZXNhbHRzYWx0MTY",
			wantErr: ErrInvalidHash,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyPassword("anything", tt.encoded)
			if ok {
				t.Fatal("VerifyPassword accepted a malformed hash")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("VerifyPassword error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	first, err := HashPasswordWithParams("same-password", testParams())
	if err != nil {
		t.Fatalf("first HashPasswordWithParams: %v", err)
	}
	second, err := HashPasswordWithParams("same-password", testParams())
	if err != nil {
		t.Fatalf("second HashPasswordWithParams: %v", err)
	}
	if first == second {
		t.Fatal("hashing the same password twice produced identical output; salt is not random")
	}
	ok, err := VerifyPassword("same-password", second)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword on the second hash = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestHashPasswordWithParamsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		params Argon2Params
	}{
		{name: "zero time", params: Argon2Params{Time: 0, Memory: 8 * 1024, Threads: 1, KeyLength: 32, SaltLength: 16}},
		{name: "zero memory", params: Argon2Params{Time: 1, Memory: 0, Threads: 1, KeyLength: 32, SaltLength: 16}},
		{name: "zero threads", params: Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 0, KeyLength: 32, SaltLength: 16}},
		{name: "zero key length", params: Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLength: 0, SaltLength: 16}},
		{name: "zero salt length", params: Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLength: 32, SaltLength: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := HashPasswordWithParams("pw", tt.params); !errors.Is(err, ErrInvalidParams) {
				t.Fatalf("HashPasswordWithParams error = %v, want ErrInvalidParams", err)
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	base := testParams()
	encoded, err := HashPasswordWithParams("pw", base)
	if err != nil {
		t.Fatalf("HashPasswordWithParams: %v", err)
	}
	tests := []struct {
		name   string
		target Argon2Params
		want   bool
	}{
		{name: "same parameters", target: base, want: false},
		{name: "weaker target", target: Argon2Params{Time: 1, Memory: 4 * 1024, Threads: 1, KeyLength: 16, SaltLength: 8}, want: false},
		{name: "more time", target: Argon2Params{Time: 2, Memory: 8 * 1024, Threads: 1, KeyLength: 32, SaltLength: 16}, want: true},
		{name: "more memory", target: Argon2Params{Time: 1, Memory: 16 * 1024, Threads: 1, KeyLength: 32, SaltLength: 16}, want: true},
		{name: "more threads", target: Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 2, KeyLength: 32, SaltLength: 16}, want: true},
		{name: "longer key", target: Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLength: 64, SaltLength: 16}, want: true},
		{name: "longer salt", target: Argon2Params{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLength: 32, SaltLength: 32}, want: true},
		{name: "production defaults", target: DefaultParams(), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NeedsRehash(encoded, tt.target)
			if err != nil {
				t.Fatalf("NeedsRehash: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NeedsRehash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsRehashErrors(t *testing.T) {
	if _, err := NeedsRehash("garbage", DefaultParams()); !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("NeedsRehash on garbage error = %v, want ErrInvalidHash", err)
	}
	encoded, err := HashPasswordWithParams("pw", testParams())
	if err != nil {
		t.Fatalf("HashPasswordWithParams: %v", err)
	}
	if _, err := NeedsRehash(encoded, Argon2Params{}); !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("NeedsRehash with invalid params error = %v, want ErrInvalidParams", err)
	}
}
