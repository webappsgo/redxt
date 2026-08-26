package ssl

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/redxt/src/config"

	"golang.org/x/crypto/acme"
)

// testSSLConfig returns a minimal server.ssl block, optionally with ACME
// issuance turned on.
func testSSLConfig(acmeEnabled bool) config.SSL {
	cfg := config.SSL{Enabled: true, MinVersion: "TLS1.2"}
	cfg.LetsEncrypt.Enabled = acmeEnabled
	cfg.LetsEncrypt.Email = "admin@example.com"
	cfg.LetsEncrypt.Challenge = "http-01"
	return cfg
}

// newTestManager builds a Manager whose certificate roots both live inside a
// temporary directory, so no test reads the host's /etc/letsencrypt.
func newTestManager(t *testing.T, cfg config.SSL, fqdn string) *Manager {
	t.Helper()
	root := t.TempDir()
	manager, err := New(cfg, filepath.Join(root, "ssl"), fqdn)
	if err != nil {
		t.Fatalf("New(%q): %v", fqdn, err)
	}
	manager.locator.SystemRoot = filepath.Join(root, "live")
	return manager
}

// mkdirAllForTest creates a directory tree for a test fixture.
func mkdirAllForTest(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func TestParseMinVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    uint16
		wantErr bool
		reason  string
	}{
		{input: "", want: tls.VersionTLS12, reason: "empty defaults to TLS 1.2"},
		{input: "TLS1.2", want: tls.VersionTLS12, reason: "canonical spelling"},
		{input: "tls1.2", want: tls.VersionTLS12, reason: "case insensitive"},
		{input: " 1.2 ", want: tls.VersionTLS12, reason: "surrounding space is trimmed"},
		{input: "TLS1.3", want: tls.VersionTLS13, reason: "canonical spelling"},
		{input: "tls13", want: tls.VersionTLS13, reason: "compact spelling"},
		{input: "TLS1.1", wantErr: true, reason: "below the supported floor"},
		{input: "SSLv3", wantErr: true, reason: "not a TLS version"},
	}

	for _, tc := range tests {
		t.Run(tc.input+"/"+tc.reason, func(t *testing.T) {
			got, err := ParseMinVersion(tc.input)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidMinVersion) {
					t.Fatalf("ParseMinVersion(%q) error = %v, want ErrInvalidMinVersion", tc.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMinVersion(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseMinVersion(%q) = %d, want %d (%s)", tc.input, got, tc.want, tc.reason)
			}
		})
	}
}

func TestParseChallenge(t *testing.T) {
	tests := []struct {
		input   string
		want    ChallengeType
		wantErr bool
		reason  string
	}{
		{input: "", want: ChallengeHTTP01, reason: "empty defaults to http-01"},
		{input: "http-01", want: ChallengeHTTP01, reason: "canonical spelling"},
		{input: "HTTP-01", want: ChallengeHTTP01, reason: "case insensitive"},
		{input: "tls-alpn-01", want: ChallengeTLSALPN01, reason: "canonical spelling"},
		{input: "tlsalpn01", want: ChallengeTLSALPN01, reason: "compact spelling"},
		{input: "dns-01", want: ChallengeDNS01, reason: "canonical spelling"},
		{input: "dns", want: ChallengeDNS01, reason: "short spelling"},
		{input: "tls-sni-01", wantErr: true, reason: "retired challenge type"},
	}

	for _, tc := range tests {
		t.Run(tc.input+"/"+tc.reason, func(t *testing.T) {
			got, err := ParseChallenge(tc.input)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidChallenge) {
					t.Fatalf("ParseChallenge(%q) error = %v, want ErrInvalidChallenge", tc.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseChallenge(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseChallenge(%q) = %q, want %q (%s)", tc.input, got, tc.want, tc.reason)
			}
		})
	}
}

func TestNewRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.SSL
		fqdn    string
		wantErr error
	}{
		{
			name:    "empty fqdn",
			cfg:     testSSLConfig(false),
			fqdn:    "   ",
			wantErr: ErrNoFQDN,
		},
		{
			name:    "unsupported min version",
			cfg:     config.SSL{MinVersion: "TLS1.0"},
			fqdn:    "api.example.com",
			wantErr: ErrInvalidMinVersion,
		},
		{
			name: "unknown challenge",
			cfg: func() config.SSL {
				cfg := testSSLConfig(true)
				cfg.LetsEncrypt.Challenge = "carrier-pigeon"
				return cfg
			}(),
			fqdn:    "api.example.com",
			wantErr: ErrInvalidChallenge,
		},
		{
			name:    "acme on a host that cannot hold a public certificate",
			cfg:     testSSLConfig(true),
			fqdn:    "redxt.local",
			wantErr: ErrHostNotEligible,
		},
		{
			name:    "acme on an onion address",
			cfg:     testSSLConfig(true),
			fqdn:    "abcdefghij234567.onion",
			wantErr: ErrHostNotEligible,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg, t.TempDir(), tc.fqdn); !errors.Is(err, tc.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewAcceptsDevHostWithoutACME(t *testing.T) {
	if _, err := New(testSSLConfig(false), t.TempDir(), "redxt.local"); err != nil {
		t.Fatalf("New() on a dev host with ACME disabled: %v", err)
	}
}

func TestTLSConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        config.SSL
		wantMin    uint16
		wantALPN   bool
		wantProtos []string
	}{
		{
			name:       "tls 1.2 without acme",
			cfg:        testSSLConfig(false),
			wantMin:    tls.VersionTLS12,
			wantProtos: []string{"h2", "http/1.1"},
		},
		{
			name: "tls 1.3 with the alpn challenge",
			cfg: func() config.SSL {
				cfg := testSSLConfig(true)
				cfg.MinVersion = "TLS1.3"
				cfg.LetsEncrypt.Challenge = "tls-alpn-01"
				return cfg
			}(),
			wantMin:    tls.VersionTLS13,
			wantALPN:   true,
			wantProtos: []string{"h2", "http/1.1", acme.ALPNProto},
		},
		{
			name:       "http challenge does not advertise the acme alpn",
			cfg:        testSSLConfig(true),
			wantMin:    tls.VersionTLS12,
			wantProtos: []string{"h2", "http/1.1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager := newTestManager(t, tc.cfg, "api.example.com")
			got := manager.TLSConfig()

			if got.MinVersion != tc.wantMin {
				t.Errorf("MinVersion = %d, want %d", got.MinVersion, tc.wantMin)
			}
			if len(got.NextProtos) != len(tc.wantProtos) {
				t.Fatalf("NextProtos = %v, want %v", got.NextProtos, tc.wantProtos)
			}
			for i, proto := range tc.wantProtos {
				if got.NextProtos[i] != proto {
					t.Errorf("NextProtos[%d] = %q, want %q", i, got.NextProtos[i], proto)
				}
			}
			if got.GetCertificate == nil {
				t.Error("GetCertificate is nil, so certificates could not reload without a restart")
			}
			if len(got.CipherSuites) != len(ModernCipherSuites()) {
				t.Errorf("CipherSuites has %d entries, want %d", len(got.CipherSuites), len(ModernCipherSuites()))
			}
			if tc.wantALPN && manager.Challenge() != ChallengeTLSALPN01 {
				t.Errorf("Challenge() = %q, want %q", manager.Challenge(), ChallengeTLSALPN01)
			}
		})
	}
}

func TestLoadAndGetCertificate(t *testing.T) {
	const fqdn = "api.example.com"
	manager := newTestManager(t, testSSLConfig(false), fqdn)

	if _, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: fqdn}); !errors.Is(err, ErrNoCertificate) {
		t.Fatalf("GetCertificate() before load error = %v, want ErrNoCertificate", err)
	}

	writePair(t, LocalDir(manager.Locator().SSLRoot, fqdn), LocalCertFile, LocalKeyFile, validSpec(fqdn))
	loaded, err := manager.LoadAt(testNow)
	if err != nil {
		t.Fatalf("LoadAt(): %v", err)
	}
	if loaded.Source != SourceLocal {
		t.Errorf("source = %q, want %q", loaded.Source, SourceLocal)
	}
	if manager.Current() != loaded {
		t.Error("Current() did not return the certificate installed by LoadAt")
	}

	served, err := manager.GetCertificate(&tls.ClientHelloInfo{ServerName: fqdn})
	if err != nil {
		t.Fatalf("GetCertificate(): %v", err)
	}
	if served != &loaded.TLS {
		t.Error("GetCertificate() did not serve the installed certificate")
	}

	// A second Load swaps the served pair without recreating the manager.
	writePair(t, LetsEncryptDir(manager.Locator().SSLRoot, fqdn), FullChainFile, PrivKeyFile, validSpec(fqdn))
	reloaded, err := manager.LoadAt(testNow)
	if err != nil {
		t.Fatalf("LoadAt() after reload: %v", err)
	}
	if reloaded.Source != SourceLetsEncrypt {
		t.Errorf("reloaded source = %q, want %q", reloaded.Source, SourceLetsEncrypt)
	}
	if manager.Current() != reloaded {
		t.Error("Current() still returns the previous certificate after a reload")
	}
}

func TestLoadAtHonoursExplicitPaths(t *testing.T) {
	const fqdn = "api.example.com"

	root := t.TempDir()
	manual := filepath.Join(root, "manual")
	writePair(t, manual, LocalCertFile, LocalKeyFile, validSpec(fqdn))

	cfg := testSSLConfig(false)
	cfg.Cert = filepath.Join(manual, LocalCertFile)
	cfg.Key = filepath.Join(manual, LocalKeyFile)

	manager, err := New(cfg, filepath.Join(root, "ssl"), fqdn)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	manager.locator.SystemRoot = filepath.Join(root, "live")

	cert, err := manager.LoadAt(testNow)
	if err != nil {
		t.Fatalf("LoadAt(): %v", err)
	}
	if cert.CertFile != cfg.Cert {
		t.Errorf("CertFile = %q, want %q", cert.CertFile, cfg.Cert)
	}
	if cert.AutoRenew() {
		t.Error("an explicitly configured certificate must not be auto-renewed")
	}
}

func TestGetCertificateServesALPNChallenge(t *testing.T) {
	const fqdn = "api.example.com"

	cfg := testSSLConfig(true)
	cfg.LetsEncrypt.Challenge = "tls-alpn-01"
	manager := newTestManager(t, cfg, fqdn)

	challengeCert := &tls.Certificate{}
	manager.challengeMu.Lock()
	manager.alpnCerts[fqdn] = challengeCert
	manager.challengeMu.Unlock()

	hello := &tls.ClientHelloInfo{ServerName: fqdn, SupportedProtos: []string{acme.ALPNProto}}
	got, err := manager.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate() for an acme-tls/1 hello: %v", err)
	}
	if got != challengeCert {
		t.Error("GetCertificate() did not serve the pending tls-alpn-01 certificate")
	}

	other := &tls.ClientHelloInfo{ServerName: "other.example.com", SupportedProtos: []string{acme.ALPNProto}}
	if _, err := manager.GetCertificate(other); !errors.Is(err, ErrNoCertificate) {
		t.Fatalf("GetCertificate() for an unknown alpn host error = %v, want ErrNoCertificate", err)
	}
}

func TestHTTP01Handler(t *testing.T) {
	const (
		fqdn  = "api.example.com"
		token = "token-value"
		body  = "token-value.account-thumbprint"
	)

	manager := newTestManager(t, testSSLConfig(true), fqdn)
	armed := ChallengePathPrefix + token
	manager.challengeMu.Lock()
	manager.httpTokens[armed] = body
	manager.challengeMu.Unlock()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "armed token", path: armed, wantStatus: http.StatusOK, wantBody: body},
		{name: "unknown token", path: ChallengePathPrefix + "other", wantStatus: http.StatusNotFound},
		{name: "outside the challenge prefix", path: "/index.html", wantStatus: http.StatusNotFound},
	}

	handler := manager.HTTP01Handler()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if tc.wantBody != "" && recorder.Body.String() != tc.wantBody {
				t.Errorf("body = %q, want %q", recorder.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestDirectoryAndAccountKeyPaths(t *testing.T) {
	tests := []struct {
		name        string
		staging     bool
		wantURL     string
		wantKeyLeaf string
	}{
		{name: "production", staging: false, wantURL: acme.LetsEncryptURL, wantKeyLeaf: filepath.Join("accounts", "production", "account.key")},
		{name: "staging", staging: true, wantURL: StagingDirectoryURL, wantKeyLeaf: filepath.Join("accounts", "staging", "account.key")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testSSLConfig(true)
			cfg.LetsEncrypt.Staging = tc.staging
			manager := newTestManager(t, cfg, "api.example.com")

			if got := manager.DirectoryURL(); got != tc.wantURL {
				t.Errorf("DirectoryURL() = %q, want %q", got, tc.wantURL)
			}
			want := filepath.Join(manager.Locator().SSLRoot, tc.wantKeyLeaf)
			if got := manager.AccountKeyPath(); got != want {
				t.Errorf("AccountKeyPath() = %q, want %q", got, want)
			}
		})
	}
}

func TestIssueRequiresACMEEnabled(t *testing.T) {
	manager := newTestManager(t, testSSLConfig(false), "api.example.com")

	if _, err := manager.Issue(t.Context()); !errors.Is(err, ErrACMEDisabled) {
		t.Fatalf("Issue() with let's encrypt disabled error = %v, want ErrACMEDisabled", err)
	}
	if _, err := manager.RenewAll(t.Context(), testNow); !errors.Is(err, ErrACMEDisabled) {
		t.Fatalf("RenewAll() with let's encrypt disabled error = %v, want ErrACMEDisabled", err)
	}
}

func TestIssueForRejectsIneligibleHost(t *testing.T) {
	manager := newTestManager(t, testSSLConfig(true), "api.example.com")

	tests := []struct {
		host    string
		wantErr error
		reason  string
	}{
		{host: "", wantErr: ErrNoFQDN, reason: "no hostname supplied"},
		{host: "redxt.local", wantErr: ErrHostNotEligible, reason: "dev TLD cannot be publicly validated"},
		{host: "abcdefghij234567.onion", wantErr: ErrHostNotEligible, reason: "overlay address cannot be publicly validated"},
	}

	for _, tc := range tests {
		t.Run(tc.host+"/"+tc.reason, func(t *testing.T) {
			if _, err := manager.IssueFor(t.Context(), tc.host); !errors.Is(err, tc.wantErr) {
				t.Fatalf("IssueFor(%q) error = %v, want %v", tc.host, err, tc.wantErr)
			}
		})
	}
}

func TestPickChallenge(t *testing.T) {
	offered := []*acme.Challenge{
		{Type: "http-01", Token: "http-token"},
		nil,
		{Type: "dns-01", Token: "dns-token"},
	}

	tests := []struct {
		want      ChallengeType
		wantToken string
		wantFound bool
	}{
		{want: ChallengeHTTP01, wantToken: "http-token", wantFound: true},
		{want: ChallengeDNS01, wantToken: "dns-token", wantFound: true},
		{want: ChallengeTLSALPN01, wantFound: false},
	}

	for _, tc := range tests {
		t.Run(tc.want.String(), func(t *testing.T) {
			got := pickChallenge(offered, tc.want)
			if !tc.wantFound {
				if got != nil {
					t.Fatalf("pickChallenge(%q) = %v, want nil", tc.want, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("pickChallenge(%q) = nil, want the offered challenge", tc.want)
			}
			if got.Token != tc.wantToken {
				t.Errorf("token = %q, want %q", got.Token, tc.wantToken)
			}
		})
	}
}

func TestDNS01RecordName(t *testing.T) {
	tests := []struct {
		domain string
		want   string
		reason string
	}{
		{domain: "example.com", want: "_acme-challenge.example.com", reason: "base domain"},
		{domain: "api.example.com", want: "_acme-challenge.api.example.com", reason: "subdomain"},
		{domain: "*.example.com", want: "_acme-challenge.example.com", reason: "wildcard validates on its base"},
		{domain: "API.Example.COM.", want: "_acme-challenge.api.example.com", reason: "case and root dot are normalised"},
		{domain: "", want: "", reason: "no domain"},
	}

	for _, tc := range tests {
		t.Run(tc.domain+"/"+tc.reason, func(t *testing.T) {
			if got := DNS01RecordName(tc.domain); got != tc.want {
				t.Errorf("DNS01RecordName(%q) = %q, want %q", tc.domain, got, tc.want)
			}
		})
	}
}
