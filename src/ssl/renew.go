package ssl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RenewalWindow is the lead time before expiry at which an app-managed
// certificate is renewed, per the PART 15 renewal rules.
const RenewalWindow = 7 * 24 * time.Hour

// RenewalHour is the local hour of the daily renewal check mandated by
// PART 15. The scheduler drives it; this package starts no timer.
const RenewalHour = 3

// NeedsRenewal reports whether cert must be renewed as of now. Only
// app-managed certificates qualify: certbot owns /etc/letsencrypt and the
// user owns the local tree. An already expired app-managed certificate is
// inside the window and is therefore renewed.
func NeedsRenewal(cert *Certificate, now time.Time) bool {
	if cert == nil || !cert.AutoRenew() {
		return false
	}
	return !now.Before(cert.NotAfter.Add(-RenewalWindow))
}

// NextRenewalCheck returns the next local RenewalHour strictly after now,
// which is when the scheduler should invoke RenewAll.
func NextRenewalCheck(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), RenewalHour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// ManagedCertificates returns every certificate under the app-managed
// letsencrypt tree, one per FQDN directory. Directories that fail to parse
// are skipped and reported through the returned error, which is non-nil
// alongside the certificates that did parse.
func (m *Manager) ManagedCertificates() ([]*Certificate, error) {
	root := filepath.Join(m.locator.SSLRoot, letsEncryptSubdir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("ssl: read %s: %w", root, err)
	}

	certs := make([]*Certificate, 0, len(entries))
	var problems []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cert, parseErr := ParseCandidate(letsEncryptCandidate(m.locator.SSLRoot, entry.Name()))
		if parseErr != nil {
			problems = append(problems, parseErr)
			continue
		}
		certs = append(certs, cert)
	}
	return certs, errors.Join(problems...)
}

// RenewAll renews every app-managed certificate that expires within
// RenewalWindow of now and returns the hostnames it renewed. It is the entry
// point the scheduler calls once daily at RenewalHour; failures for one
// hostname never abort the others.
func (m *Manager) RenewAll(ctx context.Context, now time.Time) ([]string, error) {
	if !m.cfg.LetsEncrypt.Enabled {
		return nil, ErrACMEDisabled
	}
	certs, listErr := m.ManagedCertificates()
	problems := make([]error, 0, len(certs)+1)
	if listErr != nil {
		problems = append(problems, listErr)
	}

	var renewed []string
	for _, cert := range certs {
		if !NeedsRenewal(cert, now) {
			continue
		}
		if _, err := m.IssueFor(ctx, cert.FQDN); err != nil {
			problems = append(problems, fmt.Errorf("ssl: renew %q: %w", cert.FQDN, err))
			continue
		}
		renewed = append(renewed, cert.FQDN)
	}
	return renewed, errors.Join(problems...)
}
