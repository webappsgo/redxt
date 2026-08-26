package ssl

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/webappsgo/redxt/src/urlvars"

	"golang.org/x/crypto/acme"
)

// StagingDirectoryURL is the Let's Encrypt staging ACME directory, used when
// server.ssl.letsencrypt.staging is true. Its certificates are untrusted by
// browsers but are not subject to production rate limits.
const StagingDirectoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

// ChallengePathPrefix is the URL prefix an HTTP-01 responder serves.
const ChallengePathPrefix = "/.well-known/acme-challenge/"

// DNS01RecordPrefix is the label prepended to a domain to form the TXT record
// name a DNS-01 challenge is published at.
const DNS01RecordPrefix = "_acme-challenge."

// acmeUserAgent identifies redxt to the CA, as recommended for tools using
// the low-level ACME client.
const acmeUserAgent = "redxt"

// ecPrivateKeyPEMType is the PEM block type for the SEC 1 EC keys redxt
// writes for both ACME accounts and issued certificates.
const ecPrivateKeyPEMType = "EC PRIVATE KEY"

// certificatePEMType is the PEM block type of an X.509 certificate.
const certificatePEMType = "CERTIFICATE"

// DNS01RecordRequiredError reports the TXT record that must exist before a
// DNS-01 challenge can be accepted. redxt does not talk to DNS provider APIs
// during issuance: credentials are stored through CredentialStore and the
// record is published by the caller, which then retries issuance.
type DNS01RecordRequiredError struct {
	// Domain is the identifier being validated.
	Domain string
	// Name is the fully qualified TXT record name to publish.
	Name string
	// Value is the TXT record contents.
	Value string
}

// Error describes the record that must be published.
func (e *DNS01RecordRequiredError) Error() string {
	return fmt.Sprintf("ssl: dns-01 validation of %q requires TXT %s with value %q", e.Domain, e.Name, e.Value)
}

// DNS01RecordName returns the TXT record name a DNS-01 challenge for domain
// is published at. A wildcard identifier validates on its base name.
func DNS01RecordName(domain string) string {
	host := normalizeHost(strings.TrimPrefix(normalizeHost(domain), "*."))
	if host == "" {
		return ""
	}
	return DNS01RecordPrefix + host
}

// DirectoryURL returns the ACME directory endpoint issuance runs against.
func (m *Manager) DirectoryURL() string {
	if m.cfg.LetsEncrypt.Staging {
		return StagingDirectoryURL
	}
	return acme.LetsEncryptURL
}

// AccountKeyPath returns the file holding the ACME account key. Staging and
// production keep separate accounts because the two directories do not share
// registrations.
func (m *Manager) AccountKeyPath() string {
	environment := "production"
	if m.cfg.LetsEncrypt.Staging {
		environment = "staging"
	}
	return filepath.Join(m.locator.SSLRoot, "accounts", environment, "account.key")
}

// Issue obtains a certificate for the manager's own FQDN and installs it.
func (m *Manager) Issue(ctx context.Context) (*Certificate, error) {
	return m.IssueFor(ctx, m.fqdn)
}

// IssueFor obtains a certificate for fqdn from the configured ACME directory,
// writes it into the app-managed tree as fullchain.pem and privkey.pem, and
// installs it when fqdn is the manager's own hostname.
func (m *Manager) IssueFor(ctx context.Context, fqdn string) (*Certificate, error) {
	host := normalizeHost(fqdn)
	if host == "" {
		return nil, ErrNoFQDN
	}
	if !m.cfg.LetsEncrypt.Enabled {
		return nil, ErrACMEDisabled
	}
	if !urlvars.IsValidSSLHost(host) {
		return nil, fmt.Errorf("%w: %q", ErrHostNotEligible, host)
	}

	accountKey, err := m.accountKey()
	if err != nil {
		return nil, err
	}
	client := &acme.Client{
		Key:          accountKey,
		DirectoryURL: m.DirectoryURL(),
		UserAgent:    acmeUserAgent,
	}
	if err := m.registerAccount(ctx, client); err != nil {
		return nil, err
	}

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(host))
	if err != nil {
		return nil, fmt.Errorf("ssl: authorize order for %q: %w", host, err)
	}
	for _, authzURL := range order.AuthzURLs {
		if err := m.solve(ctx, client, authzURL); err != nil {
			return nil, err
		}
	}
	order, err = client.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, fmt.Errorf("ssl: wait for order on %q: %w", host, err)
	}

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ssl: generate certificate key for %q: %w", host, err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: host},
		DNSNames: []string{host},
	}, certKey)
	if err != nil {
		return nil, fmt.Errorf("ssl: build csr for %q: %w", host, err)
	}
	chain, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, fmt.Errorf("ssl: finalize order for %q: %w", host, err)
	}
	if err := writeChain(LetsEncryptDir(m.locator.SSLRoot, host), chain, certKey); err != nil {
		return nil, err
	}

	cert, err := ParseCandidate(letsEncryptCandidate(m.locator.SSLRoot, host))
	if err != nil {
		return nil, err
	}
	if host == m.fqdn {
		m.install(cert)
	}
	return cert, nil
}

// registerAccount creates the ACME account on first use and treats an
// already-registered key as success.
func (m *Manager) registerAccount(ctx context.Context, client *acme.Client) error {
	account := &acme.Account{}
	if email := strings.TrimSpace(m.cfg.LetsEncrypt.Email); email != "" {
		account.Contact = []string{"mailto:" + email}
	}
	if _, err := client.Register(ctx, account, acme.AcceptTOS); err != nil {
		if errors.Is(err, acme.ErrAccountAlreadyExists) {
			return nil
		}
		return fmt.Errorf("ssl: register acme account: %w", err)
	}
	return nil
}

// solve satisfies one authorization using the configured challenge type.
func (m *Manager) solve(ctx context.Context, client *acme.Client, authzURL string) error {
	authz, err := client.GetAuthorization(ctx, authzURL)
	if err != nil {
		return fmt.Errorf("ssl: fetch authorization: %w", err)
	}
	if authz.Status == acme.StatusValid {
		return nil
	}
	challenge := pickChallenge(authz.Challenges, m.challenge)
	if challenge == nil {
		return fmt.Errorf("%w: %s for %q", ErrChallengeUnavailable, m.challenge, authz.Identifier.Value)
	}
	cleanup, err := m.prepareChallenge(client, challenge, authz.Identifier.Value)
	if err != nil {
		return err
	}
	defer cleanup()

	if _, err := client.Accept(ctx, challenge); err != nil {
		return fmt.Errorf("ssl: accept %s challenge for %q: %w", m.challenge, authz.Identifier.Value, err)
	}
	if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
		return fmt.Errorf("ssl: %s validation of %q failed: %w", m.challenge, authz.Identifier.Value, err)
	}
	return nil
}

// pickChallenge returns the offered challenge matching want, or nil.
func pickChallenge(offered []*acme.Challenge, want ChallengeType) *acme.Challenge {
	for _, challenge := range offered {
		if challenge != nil && challenge.Type == string(want) {
			return challenge
		}
	}
	return nil
}

// prepareChallenge arms the responder for one challenge and returns the
// function that disarms it. DNS-01 cannot be armed from inside the process,
// so it reports the record the caller must publish.
func (m *Manager) prepareChallenge(client *acme.Client, challenge *acme.Challenge, identifier string) (func(), error) {
	switch m.challenge {
	case ChallengeHTTP01:
		response, err := client.HTTP01ChallengeResponse(challenge.Token)
		if err != nil {
			return nil, fmt.Errorf("ssl: build http-01 response: %w", err)
		}
		path := client.HTTP01ChallengePath(challenge.Token)
		m.challengeMu.Lock()
		m.httpTokens[path] = response
		m.challengeMu.Unlock()
		return func() {
			m.challengeMu.Lock()
			delete(m.httpTokens, path)
			m.challengeMu.Unlock()
		}, nil

	case ChallengeTLSALPN01:
		cert, err := client.TLSALPN01ChallengeCert(challenge.Token, identifier)
		if err != nil {
			return nil, fmt.Errorf("ssl: build tls-alpn-01 certificate: %w", err)
		}
		name := normalizeHost(identifier)
		m.challengeMu.Lock()
		m.alpnCerts[name] = &cert
		m.challengeMu.Unlock()
		return func() {
			m.challengeMu.Lock()
			delete(m.alpnCerts, name)
			m.challengeMu.Unlock()
		}, nil

	case ChallengeDNS01:
		value, err := client.DNS01ChallengeRecord(challenge.Token)
		if err != nil {
			return nil, fmt.Errorf("ssl: build dns-01 record: %w", err)
		}
		return nil, &DNS01RecordRequiredError{
			Domain: normalizeHost(identifier),
			Name:   DNS01RecordName(identifier),
			Value:  value,
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrInvalidChallenge, m.challenge.String())
}

// accountKey loads the persisted ACME account key, generating and storing a
// new P-256 key on first use. Key material never appears in returned errors.
func (m *Manager) accountKey() (crypto.Signer, error) {
	path := m.AccountKeyPath()
	stored, err := os.ReadFile(path)
	switch {
	case err == nil:
		block, _ := pem.Decode(stored)
		if block == nil || block.Type != ecPrivateKeyPEMType {
			return nil, fmt.Errorf("%w: %s", ErrBadAccountKey, path)
		}
		key, parseErr := x509.ParseECPrivateKey(block.Bytes)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: %s", ErrBadAccountKey, path)
		}
		return key, nil
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, fmt.Errorf("ssl: read acme account key %s: %w", path, err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ssl: generate acme account key: %w", err)
	}
	if err := writePrivateKey(path, key, privatePerm); err != nil {
		return nil, err
	}
	return key, nil
}

// writePrivateKey PEM-encodes an EC key and writes it with owner-only
// permissions, creating its parent directory with dirPerm.
func writePrivateKey(path string, key *ecdsa.PrivateKey, dirPerm os.FileMode) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("ssl: encode private key: %w", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: ecPrivateKeyPEMType, Bytes: der})
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
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

// writeChain writes an issued chain and its key into dir using the certbot
// filenames: fullchain.pem at 0644 and privkey.pem at 0600.
func writeChain(dir string, chain [][]byte, key *ecdsa.PrivateKey) error {
	if len(chain) == 0 {
		return ErrEmptyChain
	}
	var fullchain bytes.Buffer
	for _, der := range chain {
		if err := pem.Encode(&fullchain, &pem.Block{Type: certificatePEMType, Bytes: der}); err != nil {
			return fmt.Errorf("ssl: encode certificate chain: %w", err)
		}
	}
	if err := os.MkdirAll(dir, certDirPerm); err != nil {
		return fmt.Errorf("ssl: create %s: %w", dir, err)
	}
	certPath := filepath.Join(dir, FullChainFile)
	if err := os.WriteFile(certPath, fullchain.Bytes(), certFilePerm); err != nil {
		return fmt.Errorf("ssl: write %s: %w", certPath, err)
	}
	if err := os.Chmod(certPath, certFilePerm); err != nil {
		return fmt.Errorf("ssl: set permissions on %s: %w", certPath, err)
	}
	return writePrivateKey(filepath.Join(dir, PrivKeyFile), key, certDirPerm)
}
