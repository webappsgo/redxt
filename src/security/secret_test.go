package security

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

// newTestSecret decodes a freshly generated secret for use in tests.
func newTestSecret(t *testing.T, name string) Secret {
	t.Helper()
	encoded, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	s, err := DecodeSecret(name, encoded, 1)
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}
	return s
}

func TestGenerateSecretRoundTrip(t *testing.T) {
	encoded, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	s, err := DecodeSecret(SecretInstallation, encoded, 2)
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}
	if s.Name != SecretInstallation {
		t.Fatalf("Name = %q, want %q", s.Name, SecretInstallation)
	}
	if s.Version != 2 {
		t.Fatalf("Version = %d, want 2", s.Version)
	}
	if len(s.Value) != SecretLength {
		t.Fatalf("Value length = %d, want %d", len(s.Value), SecretLength)
	}
	second, err := GenerateSecret()
	if err != nil {
		t.Fatalf("second GenerateSecret: %v", err)
	}
	if encoded == second {
		t.Fatal("two generated secrets were identical")
	}
}

func TestDecodeSecretInvalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "short", value: base64.StdEncoding.EncodeToString(make([]byte, 16))},
		{name: "long", value: base64.StdEncoding.EncodeToString(make([]byte, 64))},
		{name: "one byte short", value: base64.StdEncoding.EncodeToString(make([]byte, 31))},
		{name: "not base64", value: "this is not base64!!"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeSecret(SecretCSRF, tt.value, 1); !errors.Is(err, ErrInvalidSecret) {
				t.Fatalf("DecodeSecret error = %v, want ErrInvalidSecret", err)
			}
		})
	}
}

func TestSecretSignVerify(t *testing.T) {
	s := newTestSecret(t, SecretCookieSigning)
	other := newTestSecret(t, SecretCookieSigning)
	signature := s.Sign("session=abc123")

	if len(signature) != 64 {
		t.Fatalf("Sign length = %d, want 64 hex characters", len(signature))
	}
	if s.Sign("session=abc123") != signature {
		t.Fatal("Sign is not deterministic")
	}

	tests := []struct {
		name      string
		secret    Secret
		message   string
		signature string
		want      bool
	}{
		{name: "correct signature", secret: s, message: "session=abc123", signature: signature, want: true},
		{name: "tampered message", secret: s, message: "session=abc124", signature: signature, want: false},
		{name: "empty signature", secret: s, message: "session=abc123", signature: "", want: false},
		{name: "truncated signature", secret: s, message: "session=abc123", signature: signature[:63], want: false},
		{name: "different secret", secret: other, message: "session=abc123", signature: signature, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.secret.Verify(tt.message, tt.signature); got != tt.want {
				t.Fatalf("Verify() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretDerive(t *testing.T) {
	s := newTestSecret(t, SecretInstallation)
	tests := []struct {
		name    string
		purpose string
		length  int
	}{
		{name: "cluster signing key", purpose: "redxt/cluster-request-signing", length: 32},
		{name: "cookie salt", purpose: "redxt/cookie-salt", length: 16},
		{name: "single byte", purpose: "redxt/tiny", length: 1},
		{name: "multi block", purpose: "redxt/long", length: 100},
		{name: "empty purpose", purpose: "", length: 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Derive(tt.purpose, tt.length)
			if err != nil {
				t.Fatalf("Derive: %v", err)
			}
			if len(got) != tt.length {
				t.Fatalf("Derive length = %d, want %d", len(got), tt.length)
			}
			again, err := s.Derive(tt.purpose, tt.length)
			if err != nil {
				t.Fatalf("second Derive: %v", err)
			}
			if !bytes.Equal(got, again) {
				t.Fatal("Derive is not deterministic for the same purpose")
			}
			// A single random byte is legitimately zero 1/256 of the
			// time, so the all-zero smoke check only has statistical
			// power once there are enough bytes for the odds of an
			// honest all-zero result to be astronomically small.
			if tt.length > 1 && bytes.Equal(got, make([]byte, tt.length)) {
				t.Fatal("Derive returned all zero bytes")
			}
		})
	}
}

func TestSecretDeriveSeparatesPurposes(t *testing.T) {
	s := newTestSecret(t, SecretInstallation)
	first, err := s.Derive("purpose-a", 32)
	if err != nil {
		t.Fatalf("Derive purpose-a: %v", err)
	}
	second, err := s.Derive("purpose-b", 32)
	if err != nil {
		t.Fatalf("Derive purpose-b: %v", err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("different purposes produced identical key material")
	}

	other := newTestSecret(t, SecretInstallation)
	third, err := other.Derive("purpose-a", 32)
	if err != nil {
		t.Fatalf("Derive with a different secret: %v", err)
	}
	if bytes.Equal(first, third) {
		t.Fatal("different secrets produced identical key material for the same purpose")
	}
}

func TestSecretDeriveInvalidLength(t *testing.T) {
	s := newTestSecret(t, SecretInstallation)
	tests := []struct {
		name   string
		length int
	}{
		{name: "zero", length: 0},
		{name: "negative", length: -1},
		{name: "above hkdf ceiling", length: maxDeriveLength + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Derive("purpose", tt.length); !errors.Is(err, ErrInvalidDeriveLength) {
				t.Fatalf("Derive error = %v, want ErrInvalidDeriveLength", err)
			}
		})
	}
}

func TestSecretFingerprint(t *testing.T) {
	// SHA-256 of 32 zero bytes begins with 66687aad.
	zero := Secret{Name: SecretCSRF, Value: make([]byte, SecretLength)}
	if got := zero.Fingerprint(); got != "66687aad" {
		t.Fatalf("Fingerprint() = %q, want %q", got, "66687aad")
	}

	s := newTestSecret(t, SecretCSRF)
	fp := s.Fingerprint()
	if len(fp) != 8 {
		t.Fatalf("Fingerprint length = %d, want 8", len(fp))
	}
	if fp != s.Fingerprint() {
		t.Fatal("Fingerprint is not stable")
	}
	other := newTestSecret(t, SecretCSRF)
	if fp == other.Fingerprint() {
		t.Fatal("two distinct secrets share a fingerprint")
	}
}

func TestSecretExpired(t *testing.T) {
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "no expiry", expiresAt: time.Time{}, want: false},
		{name: "inside grace window", expiresAt: now.Add(time.Hour), want: false},
		{name: "exactly at expiry", expiresAt: now, want: true},
		{name: "past expiry", expiresAt: now.Add(-time.Hour), want: true},
		{name: "full installation grace ahead", expiresAt: now.Add(InstallationGraceWindow), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Secret{Name: SecretInstallation, Value: make([]byte, SecretLength), ExpiresAt: tt.expiresAt}
			if got := s.Expired(now); got != tt.want {
				t.Fatalf("Expired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRotationWindows(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "installation grace", got: InstallationGraceWindow, want: 7 * 24 * time.Hour},
		{name: "cookie signing rotation", got: CookieSigningRotation, want: 90 * 24 * time.Hour},
		{name: "cookie signing grace", got: CookieSigningGrace, want: 7 * 24 * time.Hour},
		{name: "csrf rotation", got: CSRFRotation, want: 180 * 24 * time.Hour},
		{name: "encryption key grace", got: EncryptionKeyGrace, want: 30 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}
