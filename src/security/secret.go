package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	// SecretInstallation is the app_secrets row name of the root secret all
	// other derived keying material hangs off.
	SecretInstallation = "installation_secret"
	// SecretCookieSigning is the app_secrets row name of the HMAC-SHA256 key
	// that signs session cookies.
	SecretCookieSigning = "cookie_signing_key"
	// SecretCSRF is the app_secrets row name of the HMAC base for CSRF
	// tokens.
	SecretCSRF = "csrf_token_secret"
)

// SecretLength is the required length in bytes (256 bits) of every
// project-level secret in app_secrets.
const SecretLength = 32

const (
	// InstallationGraceWindow is how long a rotated installation_secret is
	// kept to validate in-flight security report URLs.
	InstallationGraceWindow = 7 * 24 * time.Hour
	// CookieSigningRotation is the automatic rotation interval for
	// cookie_signing_key.
	CookieSigningRotation = 90 * 24 * time.Hour
	// CookieSigningGrace is how long a rotated cookie_signing_key still
	// validates existing session cookies.
	CookieSigningGrace = 7 * 24 * time.Hour
	// CSRFRotation is the automatic rotation interval for csrf_token_secret;
	// it is also rotated on every admin password change.
	CSRFRotation = 180 * 24 * time.Hour
	// EncryptionKeyGrace is how long a rotated server encryption key remains
	// usable for decrypting data that has not yet been re-encrypted.
	EncryptionKeyGrace = 30 * 24 * time.Hour
)

var (
	// ErrInvalidSecret means a stored secret is not a base64 value decoding
	// to exactly SecretLength bytes.
	ErrInvalidSecret = errors.New("security: secret must be 32 base64-decoded bytes")
	// ErrInvalidDeriveLength means a caller asked Derive for a
	// non-positive or unreasonably long amount of key material.
	ErrInvalidDeriveLength = errors.New("security: derive length out of range")
)

// maxDeriveLength is the RFC 5869 HKDF-Expand ceiling: 255 * HashLen.
const maxDeriveLength = 255 * sha256.Size

// GenerateSecret returns a new project-level secret: 32 bytes of crypto/rand
// rendered base64 std-encoded, which is the form stored in the app_secrets
// table. The value is never returned by any API and never logged.
func GenerateSecret() (string, error) {
	buf := make([]byte, SecretLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("security: read random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// Secret is a decoded project-level secret together with the metadata needed
// to honour its rotation grace window.
type Secret struct {
	// Name is the app_secrets row name, for example SecretInstallation.
	Name string
	// Value is the raw 32-byte secret. It never leaves the process.
	Value []byte
	// Version increments on every rotation.
	Version int
	// ExpiresAt is when a superseded secret stops being accepted. The zero
	// time means the secret is current and does not expire.
	ExpiresAt time.Time
}

// DecodeSecret decodes a base64 secret read from app_secrets and verifies it is
// exactly SecretLength bytes.
func DecodeSecret(name, base64Value string, version int) (Secret, error) {
	raw, err := base64.StdEncoding.DecodeString(base64Value)
	if err != nil {
		return Secret{}, ErrInvalidSecret
	}
	if len(raw) != SecretLength {
		return Secret{}, ErrInvalidSecret
	}
	return Secret{Name: name, Value: raw, Version: version}, nil
}

// HMAC returns the raw HMAC-SHA256 of message under the secret.
func (s Secret) HMAC(message []byte) []byte {
	mac := hmac.New(sha256.New, s.Value)
	mac.Write(message)
	return mac.Sum(nil)
}

// Sign returns the lowercase hex HMAC-SHA256 of message, the form used for
// security report identifiers, CSRF tokens, and cookie signatures.
func (s Secret) Sign(message string) string {
	return hex.EncodeToString(s.HMAC([]byte(message)))
}

// Verify reports whether signature is the hex HMAC of message under this
// secret. The comparison is constant time.
func (s Secret) Verify(message, signature string) bool {
	expected := s.Sign(message)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

// Derive returns length bytes of key material bound to purpose, using
// HKDF-SHA256 (RFC 5869) with the secret as input keying material and purpose
// as the info string.
//
// This is how installation_secret produces every other piece of keying
// material the spec calls for — cluster-internal request signing keys, the PGP
// private-key KDF input, cookie signing salts — without ever exposing the root
// secret itself. Distinct purposes yield independent keys, so compromising one
// derived key does not compromise another.
func (s Secret) Derive(purpose string, length int) ([]byte, error) {
	if length <= 0 || length > maxDeriveLength {
		return nil, ErrInvalidDeriveLength
	}
	prk := hkdfExtract(nil, s.Value)
	return hkdfExpand(prk, []byte(purpose), length), nil
}

// hkdfExtract is the RFC 5869 extract step: HMAC-SHA256 of the input keying
// material under the salt (an all-zero block when no salt is supplied).
func hkdfExtract(salt, ikm []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

// hkdfExpand is the RFC 5869 expand step, producing length bytes bound to
// info. Callers must ensure length is within maxDeriveLength.
func hkdfExpand(prk, info []byte, length int) []byte {
	out := make([]byte, 0, length)
	var block []byte
	for counter := byte(1); len(out) < length; counter++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(block)
		mac.Write(info)
		mac.Write([]byte{counter})
		block = mac.Sum(nil)
		out = append(out, block...)
	}
	return out[:length]
}

// Fingerprint returns the first 8 hex characters of the SHA-256 digest of the
// secret value.
//
// This is the ONLY representation of a project-level secret that may appear in
// an admin UI, a log line, or an audit entry — the admin panel shows
// "configured" / "rotated N days ago" / this fingerprint and nothing else. The
// secret value itself never leaves the process.
func (s Secret) Fingerprint() string {
	sum := sha256.Sum256(s.Value)
	return hex.EncodeToString(sum[:])[:8]
}

// Expired reports whether a superseded secret has fallen out of its rotation
// grace window. A secret with a zero ExpiresAt is current and never expires.
func (s Secret) Expired(now time.Time) bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(s.ExpiresAt)
}
