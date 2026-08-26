package security

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// newTestCipher builds a Cipher from a freshly generated key at the given
// version.
func newTestCipher(t *testing.T, version int) *Cipher {
	t.Helper()
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	c, err := NewCipher(key, version)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestGenerateEncryptionKey(t *testing.T) {
	first, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("generated key is not base64: %v", err)
	}
	if len(raw) != KeyLength {
		t.Fatalf("generated key length = %d, want %d", len(raw), KeyLength)
	}
	second, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("second GenerateEncryptionKey: %v", err)
	}
	if first == second {
		t.Fatal("two generated encryption keys were identical")
	}
}

func TestNewCipherInvalidInput(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
	longKey := base64.StdEncoding.EncodeToString(make([]byte, 64))
	goodKey := base64.StdEncoding.EncodeToString(make([]byte, KeyLength))
	tests := []struct {
		name    string
		key     string
		version int
		wantErr error
	}{
		{name: "short key", key: shortKey, version: 1, wantErr: ErrInvalidKey},
		{name: "long key", key: longKey, version: 1, wantErr: ErrInvalidKey},
		{name: "empty key", key: "", version: 1, wantErr: ErrInvalidKey},
		{name: "not base64", key: "this is not base64!!", version: 1, wantErr: ErrInvalidKey},
		{name: "zero version", key: goodKey, version: 0, wantErr: ErrInvalidKeyVersion},
		{name: "negative version", key: goodKey, version: -3, wantErr: ErrInvalidKeyVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCipher(tt.key, tt.version)
			if c != nil {
				t.Fatal("NewCipher returned a cipher for invalid input")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewCipher error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCipherRoundTrip(t *testing.T) {
	c := newTestCipher(t, 1)
	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "short", plaintext: "tsig-secret"},
		{name: "empty", plaintext: ""},
		{name: "unicode", plaintext: "clé-privée-🔑"},
		{name: "large", plaintext: strings.Repeat("dnssec-private-key-material ", 500)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := c.EncryptString(tt.plaintext)
			if err != nil {
				t.Fatalf("EncryptString: %v", err)
			}
			if !strings.HasPrefix(encoded, "v1:") {
				t.Fatalf("EncryptString = %q, want a v1: prefix", encoded)
			}
			if tt.plaintext != "" && strings.Contains(encoded, tt.plaintext) {
				t.Fatal("ciphertext leaks the plaintext")
			}
			got, err := c.DecryptString(encoded)
			if err != nil {
				t.Fatalf("DecryptString: %v", err)
			}
			if got != tt.plaintext {
				t.Fatalf("DecryptString = %q, want %q", got, tt.plaintext)
			}
		})
	}
}

func TestCipherVersionInOutput(t *testing.T) {
	c := newTestCipher(t, 7)
	if c.Version() != 7 {
		t.Fatalf("Version() = %d, want 7", c.Version())
	}
	encoded, err := c.EncryptString("value")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if !strings.HasPrefix(encoded, "v7:") {
		t.Fatalf("EncryptString = %q, want a v7: prefix", encoded)
	}
}

func TestCipherNonceIsRandom(t *testing.T) {
	c := newTestCipher(t, 1)
	first, err := c.EncryptString("same plaintext")
	if err != nil {
		t.Fatalf("first EncryptString: %v", err)
	}
	second, err := c.EncryptString("same plaintext")
	if err != nil {
		t.Fatalf("second EncryptString: %v", err)
	}
	if first == second {
		t.Fatal("encrypting the same plaintext twice produced identical ciphertext")
	}
}

func TestCipherDecryptFailures(t *testing.T) {
	c := newTestCipher(t, 1)
	other := newTestCipher(t, 1)
	encoded, err := c.EncryptString("secret value")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	tampered := make([]byte, len(raw))
	copy(tampered, raw)
	tampered[len(tampered)-1] ^= 0xff
	tamperedEncoded := "v1:" + base64.StdEncoding.EncodeToString(tampered)

	tests := []struct {
		name    string
		cipher  *Cipher
		encoded string
		wantErr error
	}{
		{name: "tampered ciphertext", cipher: c, encoded: tamperedEncoded, wantErr: ErrDecryptFailed},
		{name: "wrong key", cipher: other, encoded: encoded, wantErr: ErrDecryptFailed},
		{name: "empty value", cipher: c, encoded: "", wantErr: ErrInvalidCiphertext},
		{name: "malformed version tag", cipher: c, encoded: "x1:" + base64.StdEncoding.EncodeToString(raw), wantErr: ErrInvalidCiphertext},
		{name: "non-numeric version tag", cipher: c, encoded: "vone:" + base64.StdEncoding.EncodeToString(raw), wantErr: ErrInvalidCiphertext},
		{name: "undecodable payload", cipher: c, encoded: "v1:not base64!!", wantErr: ErrInvalidCiphertext},
		{name: "payload too short", cipher: c, encoded: "v1:" + base64.StdEncoding.EncodeToString([]byte("short")), wantErr: ErrInvalidCiphertext},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cipher.Decrypt(tt.encoded)
			if got != nil {
				t.Fatal("Decrypt returned plaintext for an invalid value")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Decrypt error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCipherVersionMismatch(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	v1, err := NewCipher(key, 1)
	if err != nil {
		t.Fatalf("NewCipher v1: %v", err)
	}
	v2, err := NewCipher(key, 2)
	if err != nil {
		t.Fatalf("NewCipher v2: %v", err)
	}
	encoded, err := v1.EncryptString("secret")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if _, err := v2.Decrypt(encoded); !errors.Is(err, ErrKeyVersionMismatch) {
		t.Fatalf("Decrypt error = %v, want ErrKeyVersionMismatch", err)
	}
}

func TestCipherBarePayloadCompatibility(t *testing.T) {
	c := newTestCipher(t, 1)
	encoded, err := c.EncryptString("legacy value")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	bare := strings.TrimPrefix(encoded, "v1:")
	got, err := c.DecryptString(bare)
	if err != nil {
		t.Fatalf("DecryptString on a bare payload: %v", err)
	}
	if got != "legacy value" {
		t.Fatalf("DecryptString = %q, want %q", got, "legacy value")
	}

	v2 := newTestCipher(t, 2)
	if _, err := v2.Decrypt(bare); !errors.Is(err, ErrKeyVersionMismatch) {
		t.Fatalf("bare payload under a v2 cipher error = %v, want ErrKeyVersionMismatch", err)
	}
}

func TestKeyRing(t *testing.T) {
	oldCipher := newTestCipher(t, 1)
	newCipher := newTestCipher(t, 2)
	stale := newTestCipher(t, 3)

	oldValue, err := oldCipher.EncryptString("written before rotation")
	if err != nil {
		t.Fatalf("EncryptString with the previous key: %v", err)
	}

	ring := NewKeyRing(newCipher, oldCipher)
	if ring.Current() != newCipher {
		t.Fatal("Current() did not return the current cipher")
	}

	fresh, err := ring.EncryptString("written after rotation")
	if err != nil {
		t.Fatalf("KeyRing.EncryptString: %v", err)
	}
	if !strings.HasPrefix(fresh, "v2:") {
		t.Fatalf("KeyRing.EncryptString = %q, want the current key version prefix v2:", fresh)
	}

	tests := []struct {
		name    string
		encoded string
		want    string
		wantErr bool
	}{
		{name: "current key", encoded: fresh, want: "written after rotation"},
		{name: "previous key inside the grace window", encoded: oldValue, want: "written before rotation"},
		{name: "unknown key", encoded: mustEncrypt(t, stale, "unreachable"), wantErr: true},
		{name: "garbage", encoded: "not-a-ciphertext", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ring.DecryptString(tt.encoded)
			if tt.wantErr {
				if !errors.Is(err, ErrDecryptFailed) {
					t.Fatalf("KeyRing.DecryptString error = %v, want ErrDecryptFailed", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("KeyRing.DecryptString: %v", err)
			}
			if got != tt.want {
				t.Fatalf("KeyRing.DecryptString = %q, want %q", got, tt.want)
			}
		})
	}
}

// mustEncrypt seals a value with the given cipher or fails the test.
func mustEncrypt(t *testing.T, c *Cipher, plaintext string) string {
	t.Helper()
	encoded, err := c.EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	return encoded
}
