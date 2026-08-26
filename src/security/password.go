package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2Version is the only Argon2 version this package accepts; it matches
// the version constant exposed by golang.org/x/crypto/argon2.
const argon2Version = 19

// argon2Variant is the algorithm identifier written into and required by the
// PHC encoded hash. IDEA.md mandates Argon2id for user credentials.
const argon2Variant = "argon2id"

var (
	// ErrInvalidHash means the encoded string is not a well-formed PHC
	// argon2id hash.
	ErrInvalidHash = errors.New("security: invalid password hash format")
	// ErrIncompatibleAlgorithm means the encoded hash was produced by an
	// algorithm other than argon2id.
	ErrIncompatibleAlgorithm = errors.New("security: incompatible password hash algorithm")
	// ErrIncompatibleVersion means the encoded hash declares an Argon2
	// version this build cannot verify.
	ErrIncompatibleVersion = errors.New("security: incompatible argon2 version")
	// ErrInvalidParams means the caller supplied Argon2 parameters that are
	// outside the range the library accepts.
	ErrInvalidParams = errors.New("security: invalid argon2 parameters")
)

// Argon2Params holds the cost parameters used to derive a password hash.
type Argon2Params struct {
	// Time is the number of passes over the memory.
	Time uint32
	// Memory is the memory cost in KiB.
	Memory uint32
	// Threads is the degree of parallelism.
	Threads uint8
	// KeyLength is the length in bytes of the derived key.
	KeyLength uint32
	// SaltLength is the length in bytes of the random salt.
	SaltLength uint32
}

// DefaultParams returns the production Argon2id parameters: 3 passes over
// 64 MiB with 4 lanes, producing a 32-byte key from a 16-byte salt.
func DefaultParams() Argon2Params {
	return Argon2Params{
		Time:       3,
		Memory:     64 * 1024,
		Threads:    4,
		KeyLength:  32,
		SaltLength: 16,
	}
}

// valid reports whether the parameters are usable.
func (p Argon2Params) valid() bool {
	return p.Time > 0 && p.Memory > 0 && p.Threads > 0 && p.KeyLength > 0 && p.SaltLength > 0
}

// HashPassword hashes a password with DefaultParams and returns the PHC
// encoded result.
func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultParams())
}

// HashPasswordWithParams hashes a password with the supplied parameters and a
// fresh crypto/rand salt. The result is the standard PHC encoding
//
//	$argon2id$v=19$m=65536,t=3,p=4$<b64salt>$<b64hash>
//
// where both base64 fields use base64.RawStdEncoding (no padding). The
// plaintext password is never stored, logged, or returned.
func HashPasswordWithParams(password string, p Argon2Params) (string, error) {
	if !p.valid() {
		return "", ErrInvalidParams
	}
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("security: read random bytes: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLength)
	return fmt.Sprintf(
		"$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Variant,
		argon2Version,
		p.Memory,
		p.Time,
		p.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether password matches the PHC encoded hash.
//
// A wrong password returns (false, nil); only a malformed or unsupported
// encoded hash returns an error. The digest comparison is constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLength)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash reports whether an encoded hash was produced with parameters
// weaker than p, meaning the password should be re-hashed at the next
// successful login.
func NeedsRehash(encoded string, p Argon2Params) (bool, error) {
	if !p.valid() {
		return false, ErrInvalidParams
	}
	current, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	weaker := current.Time < p.Time ||
		current.Memory < p.Memory ||
		current.Threads < p.Threads ||
		uint32(len(key)) < p.KeyLength ||
		uint32(len(salt)) < p.SaltLength
	return weaker, nil
}

// decodeHash parses a PHC encoded argon2id hash into its parameters, salt, and
// derived key.
func decodeHash(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	if parts[1] != argon2Variant {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: %q", ErrIncompatibleAlgorithm, parts[1])
	}
	rawVersion, ok := strings.CutPrefix(parts[2], "v=")
	if !ok {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: missing version field %q", ErrInvalidHash, parts[2])
	}
	version, err := strconv.ParseUint(rawVersion, 10, 32)
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: unparsable version field %q", ErrInvalidHash, parts[2])
	}
	if version != argon2Version {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: v=%d", ErrIncompatibleVersion, version)
	}
	p, err := parseCostFields(parts[3])
	if err != nil {
		return Argon2Params{}, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: undecodable salt", ErrInvalidHash)
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: undecodable digest", ErrInvalidHash)
	}
	if len(salt) == 0 || len(key) == 0 {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: empty salt or digest", ErrInvalidHash)
	}
	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}

// parseCostFields parses the "m=...,t=...,p=..." segment of a PHC hash.
func parseCostFields(field string) (Argon2Params, error) {
	fields := strings.Split(field, ",")
	if len(fields) != 3 {
		return Argon2Params{}, fmt.Errorf("%w: malformed cost fields %q", ErrInvalidHash, field)
	}
	var (
		p      Argon2Params
		seenM  bool
		seenT  bool
		seenPa bool
	)
	for _, f := range fields {
		name, value, ok := strings.Cut(f, "=")
		if !ok {
			return Argon2Params{}, fmt.Errorf("%w: malformed cost field %q", ErrInvalidHash, f)
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return Argon2Params{}, fmt.Errorf("%w: unparsable cost field %q", ErrInvalidHash, f)
		}
		switch name {
		case "m":
			p.Memory = uint32(n)
			seenM = true
		case "t":
			p.Time = uint32(n)
			seenT = true
		case "p":
			if n > 255 {
				return Argon2Params{}, fmt.Errorf("%w: parallelism out of range %q", ErrInvalidHash, f)
			}
			p.Threads = uint8(n)
			seenPa = true
		default:
			return Argon2Params{}, fmt.Errorf("%w: unknown cost field %q", ErrInvalidHash, name)
		}
	}
	if !seenM || !seenT || !seenPa {
		return Argon2Params{}, fmt.Errorf("%w: missing cost field in %q", ErrInvalidHash, field)
	}
	if p.Memory == 0 || p.Time == 0 || p.Threads == 0 {
		return Argon2Params{}, fmt.Errorf("%w: zero cost field in %q", ErrInvalidHash, field)
	}
	return p, nil
}
