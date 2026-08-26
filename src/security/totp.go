package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters. These are the RFC 6238 defaults, and they are what
// every authenticator app assumes when a provisioning URI omits them.
const (
	// TOTPDigits is the code length.
	TOTPDigits = 6
	// TOTPPeriod is the length of one time step.
	TOTPPeriod = 30 * time.Second
	// TOTPSecretBytes is the seed length. RFC 4226 requires at least 128
	// bits and recommends 160, which is what a 20-byte seed gives.
	TOTPSecretBytes = 20
	// TOTPSkew is how many steps on each side of the current one are
	// accepted, covering ordinary clock drift between a phone and the
	// server without widening the window enough to matter to an
	// attacker.
	TOTPSkew = 1
)

// ErrInvalidTOTPSecret reports a seed that is not valid base32.
var ErrInvalidTOTPSecret = errors.New("security: invalid TOTP secret")

// totpEncoding is unpadded, uppercase base32, which is the encoding
// authenticator apps expect in a provisioning URI.
var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret returns a new base32-encoded seed.
func GenerateTOTPSecret() (string, error) {
	seed := make([]byte, TOTPSecretBytes)
	if _, err := rand.Read(seed); err != nil {
		return "", err
	}
	return totpEncoding.EncodeToString(seed), nil
}

// TOTPCode computes the code for a secret at a point in time.
func TOTPCode(secret string, at time.Time) (string, error) {
	seed, err := totpEncoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", ErrInvalidTOTPSecret
	}

	counter := uint64(at.UTC().Unix()) / uint64(TOTPPeriod/time.Second)

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, seed)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for i := 0; i < TOTPDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", TOTPDigits, truncated%mod), nil
}

// VerifyTOTP checks a submitted code against a secret.
//
// Every candidate step is compared in constant time and all of them are
// evaluated before the result is returned, so the response time does not
// reveal which step matched or how close a wrong code was.
func VerifyTOTP(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != TOTPDigits {
		return false
	}

	match := 0
	for step := -TOTPSkew; step <= TOTPSkew; step++ {
		want, err := TOTPCode(secret, at.Add(time.Duration(step)*TOTPPeriod))
		if err != nil {
			return false
		}
		match |= subtle.ConstantTimeCompare([]byte(want), []byte(code))
	}
	return match == 1
}

// TOTPProvisioningURI renders the otpauth:// URI an authenticator app
// scans. It carries the seed, so it is shown once at enrollment and
// never logged.
func TOTPProvisioningURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)

	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprintf("%d", TOTPDigits))
	query.Set("period", fmt.Sprintf("%d", int(TOTPPeriod/time.Second)))

	return "otpauth://totp/" + label + "?" + query.Encode()
}
