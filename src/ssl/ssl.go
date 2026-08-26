// Package ssl implements AI.md PART 15 "SSL/TLS & Let's Encrypt".
//
// It covers the four-tier certificate lookup order and its validation rules,
// TLS configuration with hot certificate reload, ACME issuance over HTTP-01
// and TLS-ALPN-01 using the low-level golang.org/x/crypto/acme client, the
// seven-day renewal window driven by the scheduler, encrypted storage of
// DNS-01 provider credentials, and the display URL format.
//
// The package never starts a goroutine of its own: renewal is driven by the
// caller through Manager.RenewAll, and private key material is never logged
// or included in any error message.
package ssl

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/urlvars"

	"golang.org/x/crypto/acme"
)

// Filesystem permissions used across the certificate tree. The chain is world
// readable because it is public; keys and the ACME account directory are not.
const (
	certDirPerm  = 0o755
	privatePerm  = 0o700
	certFilePerm = 0o644
	keyFilePerm  = 0o600
)

// Errors reported by this package. All are wrapped, so callers should test
// them with errors.Is rather than comparing strings.
var (
	// ErrNoCertificate means no tier of the lookup order produced a usable
	// certificate for the requested hostname.
	ErrNoCertificate = errors.New("ssl: no usable certificate found")
	// ErrInvalidCertificate means the files exist but do not parse into a
	// usable certificate and key pair.
	ErrInvalidCertificate = errors.New("ssl: invalid certificate")
	// ErrHostMismatch means neither the leaf CN nor any SAN covers the
	// requested hostname.
	ErrHostMismatch = errors.New("ssl: certificate does not match host")
	// ErrExpired means the leaf certificate's NotAfter is in the past.
	ErrExpired = errors.New("ssl: certificate expired")
	// ErrInvalidMinVersion means server.ssl.min_version is neither TLS1.2
	// nor TLS1.3.
	ErrInvalidMinVersion = errors.New("ssl: min_version must be TLS1.2 or TLS1.3")
	// ErrInvalidChallenge means server.ssl.letsencrypt.challenge is not one
	// of http-01, tls-alpn-01 or dns-01.
	ErrInvalidChallenge = errors.New("ssl: challenge must be http-01, tls-alpn-01 or dns-01")
	// ErrChallengeUnavailable means the CA did not offer the configured
	// challenge type for an authorization.
	ErrChallengeUnavailable = errors.New("ssl: configured challenge not offered by the CA")
	// ErrNoFQDN means an empty hostname was supplied.
	ErrNoFQDN = errors.New("ssl: no fqdn supplied")
	// ErrHostNotEligible means the hostname cannot hold a publicly trusted
	// certificate, so ACME issuance is impossible for it.
	ErrHostNotEligible = errors.New("ssl: host is not eligible for a public certificate")
	// ErrACMEDisabled means issuance was requested while
	// server.ssl.letsencrypt.enabled is false.
	ErrACMEDisabled = errors.New("ssl: let's encrypt is disabled")
	// ErrEmptyChain means the CA returned no certificate for a finalized
	// order.
	ErrEmptyChain = errors.New("ssl: certificate authority returned an empty chain")
	// ErrBadAccountKey means the stored ACME account key is not a usable EC
	// private key.
	ErrBadAccountKey = errors.New("ssl: acme account key is unreadable")
)

// ChallengeType is an ACME challenge method supported by PART 15.
type ChallengeType string

const (
	// ChallengeHTTP01 answers on port 80 under ChallengePathPrefix.
	ChallengeHTTP01 ChallengeType = "http-01"
	// ChallengeTLSALPN01 answers on port 443 through the acme-tls/1 ALPN.
	ChallengeTLSALPN01 ChallengeType = "tls-alpn-01"
	// ChallengeDNS01 answers with a DNS TXT record and is the only method
	// that can issue wildcard certificates.
	ChallengeDNS01 ChallengeType = "dns-01"
)

// String returns the challenge identifier as written in server.yml.
func (c ChallengeType) String() string {
	return string(c)
}

// ParseChallenge normalises a configured challenge name. An empty value
// selects HTTP-01, the PART 15 default.
func ParseChallenge(name string) (ChallengeType, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "http", "http01", "http-01":
		return ChallengeHTTP01, nil
	case "alpn", "tlsalpn", "tls-alpn", "tlsalpn01", "tls-alpn-01", "tls-alpn01":
		return ChallengeTLSALPN01, nil
	case "dns", "dns01", "dns-01":
		return ChallengeDNS01, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidChallenge, name)
}

// ParseMinVersion maps server.ssl.min_version onto a crypto/tls version
// constant. An empty value selects TLS 1.2.
func ParseMinVersion(name string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "1.2", "tls1.2", "tls12", "tlsv1.2":
		return tls.VersionTLS12, nil
	case "1.3", "tls1.3", "tls13", "tlsv1.3":
		return tls.VersionTLS13, nil
	}
	return 0, fmt.Errorf("%w: %q", ErrInvalidMinVersion, name)
}

// Manager owns the certificate lifecycle for one server: discovery, the
// tls.Config it serves with, ACME issuance, and renewal. It is safe for
// concurrent use.
type Manager struct {
	cfg        config.SSL
	locator    *Locator
	fqdn       string
	challenge  ChallengeType
	minVersion uint16

	mu      sync.RWMutex
	current *Certificate

	challengeMu sync.RWMutex
	httpTokens  map[string]string
	alpnCerts   map[string]*tls.Certificate
}

// New builds a Manager for fqdn, searching sslRoot (paths.Paths.SSL) for
// app-managed and user-managed certificates. It fails when min_version or
// challenge are invalid, or when ACME is enabled for a hostname that can
// never hold a publicly trusted certificate.
func New(cfg config.SSL, sslRoot, fqdn string) (*Manager, error) {
	host := normalizeHost(fqdn)
	if host == "" {
		return nil, ErrNoFQDN
	}
	minVersion, err := ParseMinVersion(cfg.MinVersion)
	if err != nil {
		return nil, err
	}
	challenge, err := ParseChallenge(cfg.LetsEncrypt.Challenge)
	if err != nil {
		return nil, err
	}
	if cfg.LetsEncrypt.Enabled && !urlvars.IsValidSSLHost(host) {
		return nil, fmt.Errorf("%w: %q", ErrHostNotEligible, host)
	}
	return &Manager{
		cfg:        cfg,
		locator:    NewLocator(sslRoot),
		fqdn:       host,
		challenge:  challenge,
		minVersion: minVersion,
		httpTokens: make(map[string]string),
		alpnCerts:  make(map[string]*tls.Certificate),
	}, nil
}

// FQDN returns the hostname this manager serves certificates for.
func (m *Manager) FQDN() string {
	return m.fqdn
}

// MinVersion returns the negotiated crypto/tls minimum version.
func (m *Manager) MinVersion() uint16 {
	return m.minVersion
}

// Challenge returns the configured ACME challenge type.
func (m *Manager) Challenge() ChallengeType {
	return m.challenge
}

// ACMEEnabled reports whether automatic issuance and renewal are configured.
func (m *Manager) ACMEEnabled() bool {
	return m.cfg.LetsEncrypt.Enabled
}

// Locator exposes the lookup roots so callers can report which directories
// were searched.
func (m *Manager) Locator() *Locator {
	return m.locator
}

// Current returns the active certificate, or nil before the first successful
// Load or Issue.
func (m *Manager) Current() *Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Load resolves and installs the certificate for the manager's FQDN as of the
// current time.
func (m *Manager) Load() (*Certificate, error) {
	return m.LoadAt(time.Now())
}

// LoadAt is Load evaluated against an explicit instant. An explicit
// server.ssl.cert and server.ssl.key pair overrides the lookup order; the
// same validation rules still apply to it. Calling LoadAt again swaps the
// served certificate without restarting the listener.
func (m *Manager) LoadAt(now time.Time) (*Certificate, error) {
	if m.cfg.Cert != "" && m.cfg.Key != "" {
		override := Candidate{
			Source:   SourceLocal,
			Domain:   m.fqdn,
			Dir:      filepath.Dir(m.cfg.Cert),
			CertFile: m.cfg.Cert,
			KeyFile:  m.cfg.Key,
		}
		cert, err := LoadCertificate(override, m.fqdn, now)
		if err != nil {
			return nil, err
		}
		m.install(cert)
		return cert, nil
	}
	cert, err := m.locator.DiscoverAt(m.fqdn, now)
	if err != nil {
		return nil, err
	}
	m.install(cert)
	return cert, nil
}

// install replaces the served certificate under the write lock.
func (m *Manager) install(cert *Certificate) {
	m.mu.Lock()
	m.current = cert
	m.mu.Unlock()
}

// TLSConfig returns the tls.Config the HTTPS listener should use. The
// certificate is resolved per handshake through GetCertificate, so a reload
// takes effect on the next connection without restarting the server.
func (m *Manager) TLSConfig() *tls.Config {
	nextProtos := []string{"h2", "http/1.1"}
	if m.cfg.LetsEncrypt.Enabled && m.challenge == ChallengeTLSALPN01 {
		nextProtos = append(nextProtos, acme.ALPNProto)
	}
	return &tls.Config{
		MinVersion:       m.minVersion,
		CipherSuites:     ModernCipherSuites(),
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384},
		NextProtos:       nextProtos,
		GetCertificate:   m.GetCertificate,
	}
}

// ModernCipherSuites returns the TLS 1.2 suites redxt permits: forward-secret
// ECDHE key exchange with AEAD ciphers only. TLS 1.3 suites are fixed by Go
// and are not configurable.
func ModernCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
}

// GetCertificate resolves the certificate for one handshake. A ClientHello
// offering the acme-tls/1 ALPN is answered with the TLS-ALPN-01 challenge
// certificate for the requested server name; every other handshake gets the
// currently installed certificate.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello == nil {
		return nil, ErrNoCertificate
	}
	for _, proto := range hello.SupportedProtos {
		if proto == acme.ALPNProto {
			return m.challengeCertificate(hello.ServerName)
		}
	}
	m.mu.RLock()
	current := m.current
	m.mu.RUnlock()
	if current == nil {
		return nil, fmt.Errorf("%w for %q", ErrNoCertificate, m.fqdn)
	}
	return &current.TLS, nil
}

// challengeCertificate returns the pending TLS-ALPN-01 certificate for a
// server name.
func (m *Manager) challengeCertificate(serverName string) (*tls.Certificate, error) {
	name := normalizeHost(serverName)
	m.challengeMu.RLock()
	cert, ok := m.alpnCerts[name]
	m.challengeMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: no tls-alpn-01 challenge pending for %q", ErrNoCertificate, name)
	}
	return cert, nil
}

// HTTP01Handler returns the handler that answers HTTP-01 challenges. Mount it
// on the plaintext listener at ChallengePathPrefix; it serves only tokens for
// challenges currently in flight and 404s everything else.
func (m *Manager) HTTP01Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, ChallengePathPrefix) {
			http.NotFound(w, r)
			return
		}
		m.challengeMu.RLock()
		response, ok := m.httpTokens[r.URL.Path]
		m.challengeMu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := io.WriteString(w, response); err != nil {
			return
		}
	})
}
