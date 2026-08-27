package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/argon2"

	"github.com/webappsgo/redxt/src/security"
)

// saltLength is the random per-backup Argon2id salt length in bytes.
const saltLength = 16

// encryptArchive derives a 256-bit key from password with Argon2id under a
// fresh random salt, then seals plain with AES-256-GCM. The returned bytes
// are the complete on-disk ".tar.gz.enc" payload: salt || nonce ||
// ciphertext+tag. The key is never returned or logged.
//
// Cost parameters are security.DefaultParams(), the same Argon2id cost this
// codebase already uses for password hashing — this is a KDF use of the
// same primitive, not a password hash, so the PHC-encoded HashPassword
// helper does not apply here.
func encryptArchive(plain []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, ErrPasswordRequired
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("backup: read random salt: %w", err)
	}
	gcm, err := gcmFor(password, salt)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("backup: read random nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	out := make([]byte, 0, len(salt)+len(nonce)+len(sealed))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// decryptArchive reverses encryptArchive. ErrInvalidPassword covers both a
// wrong password and a corrupted file — the two are indistinguishable by
// design, so a decrypt failure never confirms whether a guessed password is
// merely wrong or the ciphertext is damaged.
func decryptArchive(sealed []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, ErrPasswordRequired
	}
	if len(sealed) < saltLength {
		return nil, ErrInvalidPassword
	}
	salt := sealed[:saltLength]
	rest := sealed[saltLength:]
	gcm, err := gcmFor(password, salt)
	if err != nil {
		return nil, err
	}
	if len(rest) < gcm.NonceSize() {
		return nil, ErrInvalidPassword
	}
	nonce := rest[:gcm.NonceSize()]
	ciphertext := rest[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidPassword
	}
	return plain, nil
}

// gcmFor derives the Argon2id key for password+salt and builds the
// AES-256-GCM AEAD over it.
func gcmFor(password string, salt []byte) (cipher.AEAD, error) {
	params := security.DefaultParams()
	key := argon2.IDKey([]byte(password), salt, params.Time, params.Memory, params.Threads, security.KeyLength)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("backup: build aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("backup: build gcm: %w", err)
	}
	return gcm, nil
}
