package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// KeyLength is the required length in bytes of the AES-256-GCM encryption key
// held in server.security.encryption_key.
const KeyLength = 32

var (
	// ErrInvalidKey means the configured encryption key is not a base64
	// string decoding to exactly KeyLength bytes.
	ErrInvalidKey = errors.New("security: encryption key must be 32 base64-decoded bytes")
	// ErrInvalidCiphertext means the stored value is not a well-formed
	// versioned ciphertext.
	ErrInvalidCiphertext = errors.New("security: invalid ciphertext")
	// ErrKeyVersionMismatch means the ciphertext was written under a
	// different key version than the cipher holds.
	ErrKeyVersionMismatch = errors.New("security: encryption key version mismatch")
	// ErrDecryptFailed means no available key could open the ciphertext. It
	// deliberately reveals nothing about which key was tried.
	ErrDecryptFailed = errors.New("security: unable to decrypt value")
	// ErrInvalidKeyVersion means a key version below 1 was supplied.
	ErrInvalidKeyVersion = errors.New("security: encryption key version must be >= 1")
)

// GenerateEncryptionKey returns a new base64 std-encoded 32-byte AES-256 key,
// which is the exact value written to server.security.encryption_key in
// server.yml on first run. The key itself is never logged.
func GenerateEncryptionKey() (string, error) {
	key := make([]byte, KeyLength)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("security: read random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Cipher performs AES-256-GCM encryption and decryption under a single
// versioned key. It is the canonical at-rest protection for every sensitive
// value redxt stores: DNSSEC private keys, recoverable TSIG secrets, TOTP
// secrets, git-sync credentials, and security report bodies.
//
// A Cipher is safe for concurrent use.
type Cipher struct {
	aead    cipher.AEAD
	version int
}

// NewCipher builds a Cipher from the base64 std-encoded key stored in
// server.yml and the companion server.security.encryption_key_version integer.
func NewCipher(base64Key string, version int) (*Cipher, error) {
	if version < 1 {
		return nil, ErrInvalidKeyVersion
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Key))
	if err != nil {
		return nil, ErrInvalidKey
	}
	if len(key) != KeyLength {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("security: build aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("security: build gcm: %w", err)
	}
	return &Cipher{aead: aead, version: version}, nil
}

// Version returns the key version this Cipher encrypts with.
func (c *Cipher) Version() int {
	return c.version
}

// Encrypt seals plaintext under the cipher's key and returns a storable string
// in the exact format
//
//	v{version}:{base64(nonce || ciphertext || tag)}
//
// A fresh 12-byte crypto/rand nonce is generated per call and prepended to the
// sealed output, so encrypting the same plaintext twice never yields the same
// string. The v{n}: tag lets a rotation grace window identify which key to try
// first without decrypting.
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("security: read random bytes: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return "v" + strconv.Itoa(c.version) + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// EncryptString is Encrypt for string plaintext.
func (c *Cipher) EncryptString(s string) (string, error) {
	return c.Encrypt([]byte(s))
}

// Decrypt opens a value produced by Encrypt.
//
// A value carrying a v{n}: tag whose version differs from this cipher's
// version is rejected with ErrKeyVersionMismatch before any cryptographic work
// is done. A bare base64 payload with no version tag is accepted as version 1
// for compatibility with data written before versioning existed; such a value
// only opens under a version 1 cipher.
func (c *Cipher) Decrypt(encoded string) ([]byte, error) {
	payload, version, err := splitVersionTag(encoded)
	if err != nil {
		return nil, err
	}
	if version != c.version {
		return nil, fmt.Errorf("%w: value is v%d, key is v%d", ErrKeyVersionMismatch, version, c.version)
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: undecodable payload", ErrInvalidCiphertext)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize+c.aead.Overhead() {
		return nil, fmt.Errorf("%w: payload too short", ErrInvalidCiphertext)
	}
	plaintext, err := c.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

// DecryptString is Decrypt returning a string.
func (c *Cipher) DecryptString(encoded string) (string, error) {
	plaintext, err := c.Decrypt(encoded)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// splitVersionTag separates a stored value into its base64 payload and key
// version. Values with no v{n}: tag are treated as version 1.
func splitVersionTag(encoded string) (string, int, error) {
	if encoded == "" {
		return "", 0, fmt.Errorf("%w: empty value", ErrInvalidCiphertext)
	}
	tag, payload, ok := strings.Cut(encoded, ":")
	if !ok {
		return encoded, 1, nil
	}
	digits, ok := strings.CutPrefix(tag, "v")
	if !ok {
		return "", 0, fmt.Errorf("%w: malformed version tag", ErrInvalidCiphertext)
	}
	version, err := strconv.Atoi(digits)
	if err != nil || version < 1 {
		return "", 0, fmt.Errorf("%w: malformed version tag", ErrInvalidCiphertext)
	}
	return payload, version, nil
}

// KeyRing holds the current encryption key plus any previous keys that are
// still inside their 30-day rotation grace window (EncryptionKeyGrace). New
// data is always written under the current key; older data keeps opening until
// it has been re-encrypted.
type KeyRing struct {
	current  *Cipher
	previous []*Cipher
}

// NewKeyRing builds a KeyRing from the current cipher and zero or more
// previous ciphers, which are tried in the order given.
func NewKeyRing(current *Cipher, previous ...*Cipher) *KeyRing {
	ring := &KeyRing{current: current}
	ring.previous = append(ring.previous, previous...)
	return ring
}

// Current returns the cipher new data is encrypted with.
func (k *KeyRing) Current() *Cipher {
	return k.current
}

// Encrypt seals plaintext under the current key.
func (k *KeyRing) Encrypt(plaintext []byte) (string, error) {
	if k.current == nil {
		return "", ErrInvalidKey
	}
	return k.current.Encrypt(plaintext)
}

// EncryptString is Encrypt for string plaintext.
func (k *KeyRing) EncryptString(s string) (string, error) {
	return k.Encrypt([]byte(s))
}

// Decrypt tries the current key first, then each previous key in order. When
// no key opens the value a single generic error is returned: the caller is
// never told which key was tried or how far the attempt got.
func (k *KeyRing) Decrypt(encoded string) ([]byte, error) {
	if k.current != nil {
		if plaintext, err := k.current.Decrypt(encoded); err == nil {
			return plaintext, nil
		}
	}
	for _, c := range k.previous {
		if c == nil {
			continue
		}
		if plaintext, err := c.Decrypt(encoded); err == nil {
			return plaintext, nil
		}
	}
	return nil, ErrDecryptFailed
}

// DecryptString is Decrypt returning a string.
func (k *KeyRing) DecryptString(encoded string) (string, error) {
	plaintext, err := k.Decrypt(encoded)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
