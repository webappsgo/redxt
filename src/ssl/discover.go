package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/urlvars"
)

// DefaultSystemLiveRoot is the certbot live directory searched by the first
// two tiers of the PART 15 lookup order.
const DefaultSystemLiveRoot = "/etc/letsencrypt/live"

// SharedSystemDirName is the literal directory name certbot deployments
// commonly use for a shared certificate, checked as lookup tier 1.
const SharedSystemDirName = "domain"

// Certificate filenames. The app-managed and system tiers mirror the certbot
// layout; the user-managed local tier uses the plain cert/key names.
const (
	// FullChainFile is the chain filename used by certbot and by the
	// app-managed letsencrypt tree.
	FullChainFile = "fullchain.pem"
	// PrivKeyFile is the private key filename used by certbot and by the
	// app-managed letsencrypt tree.
	PrivKeyFile = "privkey.pem"
	// LocalCertFile is the chain filename used by the user-managed tree.
	LocalCertFile = "cert.pem"
	// LocalKeyFile is the private key filename used by the user-managed tree.
	LocalKeyFile = "key.pem"
)

// Subdirectories of the configured SSL root.
const (
	letsEncryptSubdir = "letsencrypt"
	localSubdir       = "local"
)

// Source identifies which tier of the PART 15 lookup order produced a
// certificate, and therefore who owns its renewal.
type Source string

const (
	// SourceSystem is /etc/letsencrypt/live/**, managed by certbot. redxt
	// uses these certificates but never renews them.
	SourceSystem Source = "system"
	// SourceLetsEncrypt is {ssl_root}/letsencrypt/{fqdn}/, managed by redxt
	// and auto-renewed RenewalWindow before expiry.
	SourceLetsEncrypt Source = "letsencrypt"
	// SourceLocal is {ssl_root}/local/{fqdn}/, provided by the user and
	// never auto-renewed.
	SourceLocal Source = "local"
)

// String returns the source identifier as written in configuration and logs.
func (s Source) String() string {
	return string(s)
}

// AutoRenew reports whether redxt owns renewal for certificates from this
// source. Only the app-managed letsencrypt tier is auto-renewed.
func (s Source) AutoRenew() bool {
	return s == SourceLetsEncrypt
}

// Candidate is a single entry in the PART 15 certificate lookup order: a
// directory plus the certificate and key filenames expected inside it.
type Candidate struct {
	// Source is the ownership tier this candidate belongs to.
	Source Source
	// Domain is the FQDN the candidate directory is named for.
	Domain string
	// Dir is the directory holding the pair.
	Dir string
	// CertFile is the absolute path of the certificate chain file.
	CertFile string
	// KeyFile is the absolute path of the private key file.
	KeyFile string
}

// Certificate is a certificate/key pair loaded from disk together with the
// metadata needed to decide whether redxt must renew it.
type Certificate struct {
	// Source is the lookup tier the pair was found in.
	Source Source
	// FQDN is the hostname the containing directory is named for.
	FQDN string
	// Dir is the directory holding the pair.
	Dir string
	// CertFile is the absolute path of the certificate chain file.
	CertFile string
	// KeyFile is the absolute path of the private key file.
	KeyFile string
	// NotBefore is the leaf certificate's validity start.
	NotBefore time.Time
	// NotAfter is the leaf certificate's expiry.
	NotAfter time.Time
	// Leaf is the parsed end-entity certificate.
	Leaf *x509.Certificate
	// TLS is the parsed pair ready to be served. It holds private key
	// material and must never be logged or serialised.
	TLS tls.Certificate
}

// AutoRenew reports whether redxt owns renewal for this certificate.
func (c *Certificate) AutoRenew() bool {
	return c.Source.AutoRenew()
}

// Validate applies the PART 15 acceptance rules: the leaf's CN or one of its
// SANs must cover fqdn, and the certificate must not be expired as of now.
func (c *Certificate) Validate(fqdn string, now time.Time) error {
	if c.Leaf == nil {
		return fmt.Errorf("%w: %s has no leaf certificate", ErrInvalidCertificate, c.CertFile)
	}
	if !hostMatches(c.Leaf, fqdn) {
		return fmt.Errorf("%w: %s does not cover %q", ErrHostMismatch, c.CertFile, normalizeHost(fqdn))
	}
	if now.After(c.NotAfter) {
		return fmt.Errorf("%w: %s expired at %s", ErrExpired, c.CertFile, c.NotAfter.UTC().Format(time.RFC3339))
	}
	return nil
}

// Locator resolves certificates for a hostname across the PART 15 lookup
// order. Both roots are configurable so the search can be redirected in
// tests and in container layouts.
type Locator struct {
	// SystemRoot is the certbot live directory, DefaultSystemLiveRoot by
	// default.
	SystemRoot string
	// SSLRoot is the app's own certificate tree, paths.Paths.SSL.
	SSLRoot string
}

// NewLocator returns a Locator searching the standard certbot live directory
// and the supplied app certificate root.
func NewLocator(sslRoot string) *Locator {
	return &Locator{SystemRoot: DefaultSystemLiveRoot, SSLRoot: sslRoot}
}

// Candidates returns the lookup order for fqdn, highest priority first:
// the shared and base-domain system directories, the FQDN system directory,
// the app-managed letsencrypt directory, then the user-managed local one.
func (l *Locator) Candidates(fqdn string) []Candidate {
	host := normalizeHost(fqdn)
	list := make([]Candidate, 0, 5)
	seen := make(map[string]bool, 5)

	add := func(c Candidate) {
		if c.Dir == "" || seen[c.Dir] {
			return
		}
		seen[c.Dir] = true
		list = append(list, c)
	}

	// Tier 1: shared system directories. The literal "domain" directory is
	// the common certbot convention; the registrable base domain covers the
	// equally common wildcard deployment.
	add(systemCandidate(l.SystemRoot, SharedSystemDirName, host))
	if base := urlvars.BaseDomainOf(host); base != "" && base != host {
		add(systemCandidate(l.SystemRoot, base, host))
	}

	// Tier 2: the FQDN-named system directory.
	add(systemCandidate(l.SystemRoot, host, host))

	// Tier 3: app-managed certificates, auto-renewed.
	add(letsEncryptCandidate(l.SSLRoot, host))

	// Tier 4: user-supplied certificates, never auto-renewed.
	add(localCandidate(l.SSLRoot, host))

	return list
}

// Discover returns the first candidate for fqdn that validates as of the
// current time.
func (l *Locator) Discover(fqdn string) (*Certificate, error) {
	return l.DiscoverAt(fqdn, time.Now())
}

// DiscoverAt is Discover evaluated against an explicit instant. It returns an
// error wrapping ErrNoCertificate when no tier yields a usable pair; the
// wrapped text names the first substantive reason a candidate was rejected.
func (l *Locator) DiscoverAt(fqdn string, now time.Time) (*Certificate, error) {
	var firstReason error
	for _, candidate := range l.Candidates(fqdn) {
		cert, err := LoadCertificate(candidate, fqdn, now)
		if err == nil {
			return cert, nil
		}
		if firstReason == nil && !errors.Is(err, os.ErrNotExist) {
			firstReason = err
		}
	}
	if firstReason != nil {
		return nil, fmt.Errorf("%w for %q: %v", ErrNoCertificate, normalizeHost(fqdn), firstReason)
	}
	return nil, fmt.Errorf("%w for %q", ErrNoCertificate, normalizeHost(fqdn))
}

// LetsEncryptDir returns the app-managed certificate directory for fqdn.
func LetsEncryptDir(sslRoot, fqdn string) string {
	return filepath.Join(sslRoot, letsEncryptSubdir, normalizeHost(fqdn))
}

// LocalDir returns the user-managed certificate directory for fqdn.
func LocalDir(sslRoot, fqdn string) string {
	return filepath.Join(sslRoot, localSubdir, normalizeHost(fqdn))
}

// LoadCertificate parses the candidate's files and applies the PART 15
// validation rules for fqdn as of now.
func LoadCertificate(candidate Candidate, fqdn string, now time.Time) (*Certificate, error) {
	cert, err := ParseCandidate(candidate)
	if err != nil {
		return nil, err
	}
	if err := cert.Validate(fqdn, now); err != nil {
		return nil, err
	}
	return cert, nil
}

// ParseCandidate reads and parses a candidate's certificate and key without
// applying the host-match or expiry rules. Renewal uses it so that an already
// expired app-managed certificate is still seen and replaced.
func ParseCandidate(candidate Candidate) (*Certificate, error) {
	certPEM, err := os.ReadFile(candidate.CertFile)
	if err != nil {
		return nil, fmt.Errorf("ssl: read certificate %s: %w", candidate.CertFile, err)
	}
	keyPEM, err := os.ReadFile(candidate.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("ssl: read key %s: %w", candidate.KeyFile, err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: %s and its key do not form a usable pair", ErrInvalidCertificate, candidate.CertFile)
	}
	if len(pair.Certificate) == 0 {
		return nil, fmt.Errorf("%w: %s contains no certificate", ErrInvalidCertificate, candidate.CertFile)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("%w: %s leaf is unparseable", ErrInvalidCertificate, candidate.CertFile)
	}
	pair.Leaf = leaf
	return &Certificate{
		Source:    candidate.Source,
		FQDN:      candidate.Domain,
		Dir:       candidate.Dir,
		CertFile:  candidate.CertFile,
		KeyFile:   candidate.KeyFile,
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
		Leaf:      leaf,
		TLS:       pair,
	}, nil
}

// systemCandidate builds a certbot-layout candidate under root/dirName that
// is expected to cover domain.
func systemCandidate(root, dirName, domain string) Candidate {
	if root == "" || dirName == "" {
		return Candidate{}
	}
	dir := filepath.Join(root, dirName)
	return Candidate{
		Source:   SourceSystem,
		Domain:   domain,
		Dir:      dir,
		CertFile: filepath.Join(dir, FullChainFile),
		KeyFile:  filepath.Join(dir, PrivKeyFile),
	}
}

// letsEncryptCandidate builds the app-managed candidate for fqdn.
func letsEncryptCandidate(sslRoot, fqdn string) Candidate {
	host := normalizeHost(fqdn)
	if sslRoot == "" || host == "" {
		return Candidate{}
	}
	dir := LetsEncryptDir(sslRoot, host)
	return Candidate{
		Source:   SourceLetsEncrypt,
		Domain:   host,
		Dir:      dir,
		CertFile: filepath.Join(dir, FullChainFile),
		KeyFile:  filepath.Join(dir, PrivKeyFile),
	}
}

// localCandidate builds the user-managed candidate for fqdn.
func localCandidate(sslRoot, fqdn string) Candidate {
	host := normalizeHost(fqdn)
	if sslRoot == "" || host == "" {
		return Candidate{}
	}
	dir := LocalDir(sslRoot, host)
	return Candidate{
		Source:   SourceLocal,
		Domain:   host,
		Dir:      dir,
		CertFile: filepath.Join(dir, LocalCertFile),
		KeyFile:  filepath.Join(dir, LocalKeyFile),
	}
}

// hostMatches reports whether the leaf certificate's SANs or CN cover fqdn.
// SANs are checked first, matching the order a TLS client applies.
func hostMatches(leaf *x509.Certificate, fqdn string) bool {
	want := normalizeHost(fqdn)
	if leaf == nil || want == "" {
		return false
	}
	for _, name := range leaf.DNSNames {
		if nameMatches(normalizeHost(name), want) {
			return true
		}
	}
	return nameMatches(normalizeHost(leaf.Subject.CommonName), want)
}

// nameMatches compares one already-normalised certificate name against a
// wanted host, honouring a single leading wildcard label.
func nameMatches(name, want string) bool {
	if name == "" || want == "" {
		return false
	}
	if name == want {
		return true
	}
	if !strings.HasPrefix(name, "*.") {
		return false
	}
	suffix := name[1:]
	if !strings.HasSuffix(want, suffix) {
		return false
	}
	label := want[:len(want)-len(suffix)]
	return label != "" && !strings.Contains(label, ".")
}

// normalizeHost lowercases a hostname and strips surrounding whitespace and
// the trailing root dot so comparisons are stable.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
