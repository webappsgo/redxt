package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testNow is the fixed instant every certificate test is evaluated against,
// so no assertion depends on the wall clock.
var testNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// certSpec describes a self-signed certificate a test needs on disk.
type certSpec struct {
	commonName string
	dnsNames   []string
	notBefore  time.Time
	notAfter   time.Time
}

// writePair generates a self-signed certificate matching spec and writes it
// into dir under the supplied filenames.
func writePair(t *testing.T, dir, certFile, keyFile string, spec certSpec) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: spec.commonName},
		DNSNames:              spec.dnsNames,
		NotBefore:             spec.notBefore,
		NotAfter:              spec.notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: certificatePEMType, Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: ecPrivateKeyPEMType, Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, certFile), certPEM, 0o644); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, keyFile), keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

// validSpec returns a spec covering host and valid around testNow.
func validSpec(host string) certSpec {
	return certSpec{
		commonName: host,
		dnsNames:   []string{host},
		notBefore:  testNow.Add(-24 * time.Hour),
		notAfter:   testNow.Add(60 * 24 * time.Hour),
	}
}

// newTestLocator returns a Locator whose two roots live inside a temporary
// directory, so no test ever reads the real /etc/letsencrypt.
func newTestLocator(t *testing.T) *Locator {
	t.Helper()
	root := t.TempDir()
	return &Locator{
		SystemRoot: filepath.Join(root, "etc", "letsencrypt", "live"),
		SSLRoot:    filepath.Join(root, "config", "ssl"),
	}
}

func TestLocatorCandidatesOrder(t *testing.T) {
	locator := &Locator{SystemRoot: "/system/live", SSLRoot: "/config/ssl"}

	tests := []struct {
		fqdn  string
		dirs  []string
		what  []Source
		title string
	}{
		{
			fqdn: "api.example.com",
			dirs: []string{
				filepath.Join("/system/live", "domain"),
				filepath.Join("/system/live", "example.com"),
				filepath.Join("/system/live", "api.example.com"),
				filepath.Join("/config/ssl", "letsencrypt", "api.example.com"),
				filepath.Join("/config/ssl", "local", "api.example.com"),
			},
			what:  []Source{SourceSystem, SourceSystem, SourceSystem, SourceLetsEncrypt, SourceLocal},
			title: "subdomain has a distinct base domain tier",
		},
		{
			fqdn: "example.com",
			dirs: []string{
				filepath.Join("/system/live", "domain"),
				filepath.Join("/system/live", "example.com"),
				filepath.Join("/config/ssl", "letsencrypt", "example.com"),
				filepath.Join("/config/ssl", "local", "example.com"),
			},
			what:  []Source{SourceSystem, SourceSystem, SourceLetsEncrypt, SourceLocal},
			title: "base domain equals fqdn so the tier collapses",
		},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			got := locator.Candidates(tc.fqdn)
			if len(got) != len(tc.dirs) {
				t.Fatalf("Candidates(%q) returned %d entries, want %d", tc.fqdn, len(got), len(tc.dirs))
			}
			for i, candidate := range got {
				if candidate.Dir != tc.dirs[i] {
					t.Errorf("candidate %d dir = %q, want %q", i, candidate.Dir, tc.dirs[i])
				}
				if candidate.Source != tc.what[i] {
					t.Errorf("candidate %d source = %q, want %q", i, candidate.Source, tc.what[i])
				}
			}
		})
	}
}

func TestDiscoverLookupOrder(t *testing.T) {
	const fqdn = "api.example.com"

	tests := []struct {
		name       string
		setup      func(t *testing.T, l *Locator)
		wantSource Source
		wantDir    func(l *Locator) string
		wantErr    error
	}{
		{
			name:    "nothing on disk",
			setup:   func(t *testing.T, l *Locator) {},
			wantErr: ErrNoCertificate,
		},
		{
			name: "only the local tier",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, LocalDir(l.SSLRoot, fqdn), LocalCertFile, LocalKeyFile, validSpec(fqdn))
			},
			wantSource: SourceLocal,
			wantDir:    func(l *Locator) string { return LocalDir(l.SSLRoot, fqdn) },
		},
		{
			name: "app managed beats local",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, LocalDir(l.SSLRoot, fqdn), LocalCertFile, LocalKeyFile, validSpec(fqdn))
				writePair(t, LetsEncryptDir(l.SSLRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
			},
			wantSource: SourceLetsEncrypt,
			wantDir:    func(l *Locator) string { return LetsEncryptDir(l.SSLRoot, fqdn) },
		},
		{
			name: "system fqdn directory beats app managed",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, LocalDir(l.SSLRoot, fqdn), LocalCertFile, LocalKeyFile, validSpec(fqdn))
				writePair(t, LetsEncryptDir(l.SSLRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
				writePair(t, filepath.Join(l.SystemRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
			},
			wantSource: SourceSystem,
			wantDir:    func(l *Locator) string { return filepath.Join(l.SystemRoot, fqdn) },
		},
		{
			name: "shared system directory beats the fqdn one",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, LetsEncryptDir(l.SSLRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
				writePair(t, filepath.Join(l.SystemRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
				writePair(t, filepath.Join(l.SystemRoot, SharedSystemDirName), FullChainFile, PrivKeyFile, validSpec(fqdn))
			},
			wantSource: SourceSystem,
			wantDir:    func(l *Locator) string { return filepath.Join(l.SystemRoot, SharedSystemDirName) },
		},
		{
			name: "base domain directory is searched before the fqdn one",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, filepath.Join(l.SystemRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
				writePair(t, filepath.Join(l.SystemRoot, "example.com"), FullChainFile, PrivKeyFile, certSpec{
					commonName: "example.com",
					dnsNames:   []string{"*.example.com"},
					notBefore:  testNow.Add(-24 * time.Hour),
					notAfter:   testNow.Add(60 * 24 * time.Hour),
				})
			},
			wantSource: SourceSystem,
			wantDir:    func(l *Locator) string { return filepath.Join(l.SystemRoot, "example.com") },
		},
		{
			name: "a mismatched host falls through to the next tier",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, LetsEncryptDir(l.SSLRoot, fqdn), FullChainFile, PrivKeyFile, validSpec("other.example.org"))
				writePair(t, LocalDir(l.SSLRoot, fqdn), LocalCertFile, LocalKeyFile, validSpec(fqdn))
			},
			wantSource: SourceLocal,
			wantDir:    func(l *Locator) string { return LocalDir(l.SSLRoot, fqdn) },
		},
		{
			name: "an expired certificate falls through to the next tier",
			setup: func(t *testing.T, l *Locator) {
				writePair(t, LetsEncryptDir(l.SSLRoot, fqdn), FullChainFile, PrivKeyFile, certSpec{
					commonName: fqdn,
					dnsNames:   []string{fqdn},
					notBefore:  testNow.Add(-90 * 24 * time.Hour),
					notAfter:   testNow.Add(-24 * time.Hour),
				})
				writePair(t, LocalDir(l.SSLRoot, fqdn), LocalCertFile, LocalKeyFile, validSpec(fqdn))
			},
			wantSource: SourceLocal,
			wantDir:    func(l *Locator) string { return LocalDir(l.SSLRoot, fqdn) },
		},
		{
			name: "a key without its certificate is not usable",
			setup: func(t *testing.T, l *Locator) {
				dir := LetsEncryptDir(l.SSLRoot, fqdn)
				writePair(t, dir, FullChainFile, PrivKeyFile, validSpec(fqdn))
				if err := os.Remove(filepath.Join(dir, FullChainFile)); err != nil {
					t.Fatalf("remove chain: %v", err)
				}
			},
			wantErr: ErrNoCertificate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			locator := newTestLocator(t)
			tc.setup(t, locator)

			cert, err := locator.DiscoverAt(fqdn, testNow)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("DiscoverAt(%q) error = %v, want %v", fqdn, err, tc.wantErr)
				}
				if cert != nil {
					t.Errorf("DiscoverAt(%q) returned a certificate alongside an error", fqdn)
				}
				return
			}
			if err != nil {
				t.Fatalf("DiscoverAt(%q) unexpected error: %v", fqdn, err)
			}
			if cert.Source != tc.wantSource {
				t.Errorf("source = %q, want %q", cert.Source, tc.wantSource)
			}
			if want := tc.wantDir(locator); cert.Dir != want {
				t.Errorf("dir = %q, want %q", cert.Dir, want)
			}
			if want := tc.wantSource.AutoRenew(); cert.AutoRenew() != want {
				t.Errorf("AutoRenew() = %v, want %v", cert.AutoRenew(), want)
			}
			if len(cert.TLS.Certificate) == 0 {
				t.Error("loaded certificate carries no DER chain")
			}
		})
	}
}

func TestSourceAutoRenew(t *testing.T) {
	tests := []struct {
		source Source
		want   bool
		reason string
	}{
		{source: SourceSystem, want: false, reason: "certbot owns /etc/letsencrypt"},
		{source: SourceLetsEncrypt, want: true, reason: "app manages its own tree"},
		{source: SourceLocal, want: false, reason: "user manages local certificates"},
	}

	for _, tc := range tests {
		t.Run(tc.source.String(), func(t *testing.T) {
			if got := tc.source.AutoRenew(); got != tc.want {
				t.Errorf("%q.AutoRenew() = %v, want %v (%s)", tc.source, got, tc.want, tc.reason)
			}
		})
	}
}

func TestHostMatches(t *testing.T) {
	tests := []struct {
		name       string
		commonName string
		dnsNames   []string
		host       string
		want       bool
		reason     string
	}{
		{name: "exact san", dnsNames: []string{"api.example.com"}, host: "api.example.com", want: true, reason: "SAN equals host"},
		{name: "second san", dnsNames: []string{"www.example.com", "api.example.com"}, host: "api.example.com", want: true, reason: "any SAN may match"},
		{name: "cn only", commonName: "api.example.com", host: "api.example.com", want: true, reason: "CN is accepted when no SAN matches"},
		{name: "cn mismatch", commonName: "other.example.com", host: "api.example.com", want: false, reason: "CN covers a different host"},
		{name: "case insensitive", dnsNames: []string{"API.Example.COM"}, host: "api.example.com", want: true, reason: "names compare case insensitively"},
		{name: "trailing dot", dnsNames: []string{"api.example.com"}, host: "api.example.com.", want: true, reason: "root dot is stripped"},
		{name: "wildcard one label", dnsNames: []string{"*.example.com"}, host: "api.example.com", want: true, reason: "wildcard covers one label"},
		{name: "wildcard two labels", dnsNames: []string{"*.example.com"}, host: "a.b.example.com", want: false, reason: "wildcard covers exactly one label"},
		{name: "wildcard bare base", dnsNames: []string{"*.example.com"}, host: "example.com", want: false, reason: "wildcard does not cover the base domain"},
		{name: "suffix is not a match", dnsNames: []string{"example.com"}, host: "notexample.com", want: false, reason: "suffix overlap is not a match"},
		{name: "empty host", dnsNames: []string{"api.example.com"}, host: "", want: false, reason: "no host to match"},
		{name: "no names at all", host: "api.example.com", want: false, reason: "certificate covers nothing"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			leaf := &x509.Certificate{
				Subject:  pkix.Name{CommonName: tc.commonName},
				DNSNames: tc.dnsNames,
			}
			if got := hostMatches(leaf, tc.host); got != tc.want {
				t.Errorf("hostMatches(%q) = %v, want %v (%s)", tc.host, got, tc.want, tc.reason)
			}
		})
	}
}

func TestCertificateValidate(t *testing.T) {
	const fqdn = "api.example.com"

	tests := []struct {
		name      string
		spec      certSpec
		wantErr   error
		wantValid bool
	}{
		{
			name:      "valid and current",
			spec:      validSpec(fqdn),
			wantValid: true,
		},
		{
			name: "expired one second ago",
			spec: certSpec{
				commonName: fqdn,
				dnsNames:   []string{fqdn},
				notBefore:  testNow.Add(-90 * 24 * time.Hour),
				notAfter:   testNow.Add(-time.Second),
			},
			wantErr: ErrExpired,
		},
		{
			name: "expiring in one second is still valid",
			spec: certSpec{
				commonName: fqdn,
				dnsNames:   []string{fqdn},
				notBefore:  testNow.Add(-90 * 24 * time.Hour),
				notAfter:   testNow.Add(time.Second),
			},
			wantValid: true,
		},
		{
			name:    "covers a different host",
			spec:    validSpec("other.example.org"),
			wantErr: ErrHostMismatch,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			locator := newTestLocator(t)
			dir := LocalDir(locator.SSLRoot, fqdn)
			writePair(t, dir, LocalCertFile, LocalKeyFile, tc.spec)

			cert, err := ParseCandidate(localCandidate(locator.SSLRoot, fqdn))
			if err != nil {
				t.Fatalf("ParseCandidate: %v", err)
			}
			err = cert.Validate(fqdn, testNow)
			if tc.wantValid {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestParseCandidateMissingFiles(t *testing.T) {
	locator := newTestLocator(t)

	_, err := ParseCandidate(localCandidate(locator.SSLRoot, "api.example.com"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ParseCandidate on an empty tree error = %v, want os.ErrNotExist", err)
	}
}
