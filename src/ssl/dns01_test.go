package ssl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/security"
)

// AES-256-GCM framing sizes, used to assert that stored credentials really
// are sealed rather than merely encoded.
const (
	gcmNonceLen = 12
	gcmTagLen   = 16
)

// newTestCredentialStore builds a store on a fixed key, so no assertion in
// this file depends on random material.
func newTestCredentialStore(t *testing.T) *CredentialStore {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), security.KeyLength))
	cipher, err := security.NewCipher(key, 1)
	if err != nil {
		t.Fatalf("security.NewCipher(): %v", err)
	}
	store, err := NewCredentialStore(cipher)
	if err != nil {
		t.Fatalf("NewCredentialStore(): %v", err)
	}
	return store
}

func TestNewCredentialStoreRequiresCipher(t *testing.T) {
	if _, err := NewCredentialStore(nil); !errors.Is(err, ErrNoCipher) {
		t.Fatalf("NewCredentialStore(nil) error = %v, want ErrNoCipher", err)
	}
}

func TestCredentialRoundTrip(t *testing.T) {
	validatedAt := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		provider string
		want     string
		fields   map[string]string
	}{
		{
			name:     "single field token provider",
			provider: "cloudflare",
			want:     "cloudflare",
			fields:   map[string]string{"api_token": "token-value"},
		},
		{
			name:     "multi field provider",
			provider: "route53",
			want:     "route53",
			fields: map[string]string{
				"access_key_id":     "key-id",
				"secret_access_key": "secret-value",
				"hosted_zone_id":    "zone-id",
			},
		},
		{
			name:     "provider name is normalised",
			provider: "  DigitalOcean  ",
			want:     "digitalocean",
			fields:   map[string]string{"api_token": "token-value"},
		},
	}

	store := newTestCredentialStore(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record, err := store.Seal(tc.provider, tc.fields, validatedAt)
			if err != nil {
				t.Fatalf("Seal(%q): %v", tc.provider, err)
			}
			if record.Provider != tc.want {
				t.Errorf("Provider = %q, want %q", record.Provider, tc.want)
			}
			if !record.ValidatedAt.Equal(validatedAt) {
				t.Errorf("ValidatedAt = %s, want %s", record.ValidatedAt, validatedAt)
			}
			if record.CredentialsEncrypted == "" {
				t.Fatal("CredentialsEncrypted is empty")
			}
			if !strings.HasPrefix(record.CredentialsEncrypted, "v1:") {
				t.Errorf("CredentialsEncrypted = %q, want the versioned v1: prefix", record.CredentialsEncrypted)
			}

			// AES-256-GCM output is a 12-byte nonce, the sealed plaintext
			// and a 16-byte tag, so anything shorter is not encrypted.
			sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(record.CredentialsEncrypted, "v1:"))
			if err != nil {
				t.Fatalf("decode ciphertext: %v", err)
			}
			plaintext, err := json.Marshal(tc.fields)
			if err != nil {
				t.Fatalf("encode expected plaintext: %v", err)
			}
			if want := len(plaintext) + gcmNonceLen + gcmTagLen; len(sealed) != want {
				t.Errorf("sealed length = %d, want %d", len(sealed), want)
			}

			opened, err := store.Open(record)
			if err != nil {
				t.Fatalf("Open(): %v", err)
			}
			if len(opened) != len(tc.fields) {
				t.Fatalf("Open() returned %d fields, want %d", len(opened), len(tc.fields))
			}
			for key, want := range tc.fields {
				if opened[key] != want {
					t.Errorf("field %q = %q, want %q", key, opened[key], want)
				}
			}
		})
	}
}

func TestSealRejectsBadInput(t *testing.T) {
	store := newTestCredentialStore(t)

	tests := []struct {
		name     string
		provider string
		fields   map[string]string
		wantErr  error
	}{
		{name: "empty provider", provider: "   ", fields: map[string]string{"api_token": "t"}, wantErr: ErrNoProvider},
		{name: "provider with a path separator", provider: "../escape", fields: map[string]string{"api_token": "t"}, wantErr: ErrNoProvider},
		{name: "dot provider", provider: ".", fields: map[string]string{"api_token": "t"}, wantErr: ErrNoProvider},
		{name: "parent provider", provider: "..", fields: map[string]string{"api_token": "t"}, wantErr: ErrNoProvider},
		{name: "no fields", provider: "cloudflare", fields: nil, wantErr: ErrNoCredentials},
		{name: "empty field map", provider: "cloudflare", fields: map[string]string{}, wantErr: ErrNoCredentials},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.Seal(tc.provider, tc.fields, time.Time{}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Seal(%q) error = %v, want %v", tc.provider, err, tc.wantErr)
			}
		})
	}
}

func TestOpenRejectsBadRecords(t *testing.T) {
	store := newTestCredentialStore(t)

	tests := []struct {
		name    string
		record  *ProviderCredential
		wantErr error
	}{
		{name: "nil record", record: nil, wantErr: ErrNoProvider},
		{name: "no provider", record: &ProviderCredential{CredentialsEncrypted: "v1:abc"}, wantErr: ErrNoProvider},
		{name: "no ciphertext", record: &ProviderCredential{Provider: "cloudflare"}, wantErr: ErrNoCredentials},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.Open(tc.record); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Open() error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	tampered := &ProviderCredential{Provider: "cloudflare", CredentialsEncrypted: "v1:bm90LWEtdmFsaWQtY2lwaGVydGV4dA=="}
	if _, err := store.Open(tampered); err == nil {
		t.Error("Open() on a corrupt ciphertext error = nil, want an authentication failure")
	}
}

func TestCredentialPath(t *testing.T) {
	root := filepath.Join("var", "ssl")

	tests := []struct {
		provider string
		want     string
		reason   string
	}{
		{provider: "cloudflare", want: filepath.Join(root, dns01Subdir, "cloudflare.json"), reason: "plain provider"},
		{provider: "Route53", want: filepath.Join(root, dns01Subdir, "route53.json"), reason: "case is normalised"},
		{provider: "../etc", want: filepath.Join(root, dns01Subdir, ".json"), reason: "a traversal attempt collapses inside the directory"},
	}

	for _, tc := range tests {
		t.Run(tc.provider+"/"+tc.reason, func(t *testing.T) {
			if got := CredentialPath(root, tc.provider); got != tc.want {
				t.Errorf("CredentialPath(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}

func TestSaveAndLoadRecord(t *testing.T) {
	root := t.TempDir()
	store := newTestCredentialStore(t)
	validatedAt := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	fields := map[string]string{"api_token": "token-value", "zone_id": "zone-value"}

	sealed, err := store.Seal("cloudflare", fields, validatedAt)
	if err != nil {
		t.Fatalf("Seal(): %v", err)
	}
	if err := store.Save(root, sealed); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	path := CredentialPath(root, "cloudflare")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if perm := info.Mode().Perm(); perm != keyFilePerm {
		t.Errorf("permissions on %s = %o, want %o", path, perm, keyFilePerm)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Error("stored record does not end with a trailing newline")
	}

	// Only the three PART 15 fields may reach disk; a fourth key would mean
	// credential material escaped the ciphertext.
	stored := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if len(stored) != 3 {
		t.Fatalf("stored record has %d fields, want exactly 3", len(stored))
	}
	for _, key := range []string{"provider", "credentials_encrypted", "validated_at"} {
		if _, ok := stored[key]; !ok {
			t.Errorf("stored record is missing %q", key)
		}
	}

	loaded, err := LoadRecord(root, "CloudFlare")
	if err != nil {
		t.Fatalf("LoadRecord(): %v", err)
	}
	if loaded.Provider != "cloudflare" {
		t.Errorf("Provider = %q, want %q", loaded.Provider, "cloudflare")
	}
	if !loaded.ValidatedAt.Equal(validatedAt) {
		t.Errorf("ValidatedAt = %s, want %s", loaded.ValidatedAt, validatedAt)
	}
	if loaded.CredentialsEncrypted != sealed.CredentialsEncrypted {
		t.Error("stored ciphertext did not survive the round trip")
	}

	opened, err := store.Open(loaded)
	if err != nil {
		t.Fatalf("Open() on the loaded record: %v", err)
	}
	for key, want := range fields {
		if opened[key] != want {
			t.Errorf("field %q = %q, want %q", key, opened[key], want)
		}
	}
}

func TestSaveAndLoadRecordRejectBadInput(t *testing.T) {
	root := t.TempDir()
	store := newTestCredentialStore(t)

	if err := store.Save(root, nil); !errors.Is(err, ErrNoProvider) {
		t.Errorf("Save(nil) error = %v, want ErrNoProvider", err)
	}
	if err := store.Save(root, &ProviderCredential{Provider: "cloudflare"}); !errors.Is(err, ErrNoCredentials) {
		t.Errorf("Save() without a ciphertext error = %v, want ErrNoCredentials", err)
	}
	if _, err := LoadRecord(root, "  "); !errors.Is(err, ErrNoProvider) {
		t.Errorf("LoadRecord() without a provider error = %v, want ErrNoProvider", err)
	}
	if _, err := LoadRecord(root, "missing"); err == nil {
		t.Error("LoadRecord() for an absent provider error = nil, want a read failure")
	}
}
