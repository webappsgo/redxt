package ssl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/security"
)

// dns01Subdir is the directory under the SSL root holding stored DNS-01
// provider credential records.
const dns01Subdir = "dns01"

// Errors reported by the DNS-01 credential store.
var (
	// ErrNoProvider means a credential record carries no provider
	// identifier.
	ErrNoProvider = errors.New("ssl: dns-01 provider identifier is required")
	// ErrNoCredentials means a credential record carries no fields.
	ErrNoCredentials = errors.New("ssl: dns-01 credentials are required")
	// ErrNoCipher means the store was built without an encryption key.
	ErrNoCipher = errors.New("ssl: dns-01 credential store requires an encryption key")
)

// ProviderCredential is one DNS-01 provider's stored credentials, in the
// shape PART 15 defines. The credential fields themselves exist only as an
// AES-256-GCM ciphertext; plaintext is never written to disk or logged.
type ProviderCredential struct {
	// Provider is the provider identifier, for example cloudflare or
	// route53.
	Provider string `json:"provider"`
	// CredentialsEncrypted is the AES-256-GCM ciphertext of the credential
	// JSON object, in the versioned format security.Cipher produces.
	CredentialsEncrypted string `json:"credentials_encrypted"`
	// ValidatedAt is when the credentials last succeeded against the
	// provider's API.
	ValidatedAt time.Time `json:"validated_at"`
}

// CredentialStore seals and opens DNS-01 provider credentials using the
// server's AES-256-GCM encryption key.
type CredentialStore struct {
	cipher *security.Cipher
}

// NewCredentialStore returns a store backed by an initialised
// security.Cipher, which is the app's single AES-256-GCM implementation.
func NewCredentialStore(cipher *security.Cipher) (*CredentialStore, error) {
	if cipher == nil {
		return nil, ErrNoCipher
	}
	return &CredentialStore{cipher: cipher}, nil
}

// Seal encrypts the provider's credential fields into a storable record.
// Field names and values are provider-specific and are validated by the
// caller before the record is written.
func (s *CredentialStore) Seal(provider string, fields map[string]string, validatedAt time.Time) (*ProviderCredential, error) {
	name := normalizeProvider(provider)
	if name == "" {
		return nil, ErrNoProvider
	}
	if len(fields) == 0 {
		return nil, ErrNoCredentials
	}
	plaintext, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("ssl: encode %q credentials: %w", name, err)
	}
	ciphertext, err := s.cipher.Encrypt(plaintext)
	if err != nil {
		return nil, fmt.Errorf("ssl: encrypt %q credentials: %w", name, err)
	}
	return &ProviderCredential{
		Provider:             name,
		CredentialsEncrypted: ciphertext,
		ValidatedAt:          validatedAt.UTC(),
	}, nil
}

// Open decrypts a stored record back into its provider credential fields.
func (s *CredentialStore) Open(record *ProviderCredential) (map[string]string, error) {
	if record == nil || normalizeProvider(record.Provider) == "" {
		return nil, ErrNoProvider
	}
	if record.CredentialsEncrypted == "" {
		return nil, ErrNoCredentials
	}
	plaintext, err := s.cipher.Decrypt(record.CredentialsEncrypted)
	if err != nil {
		return nil, fmt.Errorf("ssl: decrypt %q credentials: %w", record.Provider, err)
	}
	fields := make(map[string]string)
	if err := json.Unmarshal(plaintext, &fields); err != nil {
		return nil, fmt.Errorf("ssl: decode %q credentials: %w", record.Provider, err)
	}
	return fields, nil
}

// CredentialPath returns the file a provider's record is stored in.
func CredentialPath(sslRoot, provider string) string {
	return filepath.Join(sslRoot, dns01Subdir, normalizeProvider(provider)+".json")
}

// Save writes a sealed record under the SSL root with owner-only
// permissions. Only the ciphertext and its metadata are persisted.
func (s *CredentialStore) Save(sslRoot string, record *ProviderCredential) error {
	if record == nil || normalizeProvider(record.Provider) == "" {
		return ErrNoProvider
	}
	if record.CredentialsEncrypted == "" {
		return ErrNoCredentials
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("ssl: encode %q credential record: %w", record.Provider, err)
	}
	encoded = append(encoded, '\n')
	path := CredentialPath(sslRoot, record.Provider)
	if err := os.MkdirAll(filepath.Dir(path), privatePerm); err != nil {
		return fmt.Errorf("ssl: create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, encoded, keyFilePerm); err != nil {
		return fmt.Errorf("ssl: write %s: %w", path, err)
	}
	if err := os.Chmod(path, keyFilePerm); err != nil {
		return fmt.Errorf("ssl: set permissions on %s: %w", path, err)
	}
	return nil
}

// LoadRecord reads a provider's stored record without decrypting it.
func LoadRecord(sslRoot, provider string) (*ProviderCredential, error) {
	name := normalizeProvider(provider)
	if name == "" {
		return nil, ErrNoProvider
	}
	path := CredentialPath(sslRoot, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ssl: read %s: %w", path, err)
	}
	record := &ProviderCredential{}
	if err := json.Unmarshal(raw, record); err != nil {
		return nil, fmt.Errorf("ssl: decode %s: %w", path, err)
	}
	if normalizeProvider(record.Provider) == "" {
		return nil, ErrNoProvider
	}
	return record, nil
}

// normalizeProvider lowercases a provider identifier and rejects anything
// carrying path separators, so a record name can never escape its directory.
func normalizeProvider(provider string) string {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" || name == "." || name == ".." {
		return ""
	}
	if strings.ContainsAny(name, `/\`) {
		return ""
	}
	return name
}
