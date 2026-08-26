package ssl

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNeedsRenewal(t *testing.T) {
	tests := []struct {
		name      string
		source    Source
		notAfter  time.Time
		want      bool
		reason    string
		nilTarget bool
	}{
		{
			name:     "app managed far from expiry",
			source:   SourceLetsEncrypt,
			notAfter: testNow.Add(RenewalWindow + time.Hour),
			want:     false,
			reason:   "one hour outside the seven day window",
		},
		{
			name:     "app managed exactly at the window edge",
			source:   SourceLetsEncrypt,
			notAfter: testNow.Add(RenewalWindow),
			want:     true,
			reason:   "the window boundary is inclusive",
		},
		{
			name:     "app managed inside the window",
			source:   SourceLetsEncrypt,
			notAfter: testNow.Add(RenewalWindow - time.Hour),
			want:     true,
			reason:   "one hour inside the seven day window",
		},
		{
			name:     "app managed already expired",
			source:   SourceLetsEncrypt,
			notAfter: testNow.Add(-time.Hour),
			want:     true,
			reason:   "an expired app certificate must be replaced",
		},
		{
			name:     "system certificate inside the window",
			source:   SourceSystem,
			notAfter: testNow.Add(time.Hour),
			want:     false,
			reason:   "certbot renews /etc/letsencrypt",
		},
		{
			name:     "local certificate inside the window",
			source:   SourceLocal,
			notAfter: testNow.Add(time.Hour),
			want:     false,
			reason:   "the user renews local certificates",
		},
		{
			name:      "no certificate",
			nilTarget: true,
			want:      false,
			reason:    "nothing to renew",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cert *Certificate
			if !tc.nilTarget {
				cert = &Certificate{Source: tc.source, FQDN: "api.example.com", NotAfter: tc.notAfter}
			}
			if got := NeedsRenewal(cert, testNow); got != tc.want {
				t.Errorf("NeedsRenewal() = %v, want %v (%s)", got, tc.want, tc.reason)
			}
		})
	}
}

func TestNextRenewalCheck(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before the daily check",
			now:  time.Date(2026, time.March, 1, 2, 59, 59, 0, time.UTC),
			want: time.Date(2026, time.March, 1, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "exactly at the daily check",
			now:  time.Date(2026, time.March, 1, 3, 0, 0, 0, time.UTC),
			want: time.Date(2026, time.March, 2, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "after the daily check",
			now:  time.Date(2026, time.March, 1, 3, 0, 1, 0, time.UTC),
			want: time.Date(2026, time.March, 2, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "late evening rolls to the next day",
			now:  time.Date(2026, time.March, 31, 23, 45, 0, 0, time.UTC),
			want: time.Date(2026, time.April, 1, 3, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextRenewalCheck(tc.now); !got.Equal(tc.want) {
				t.Errorf("NextRenewalCheck(%s) = %s, want %s", tc.now, got, tc.want)
			}
		})
	}
}

func TestManagedCertificates(t *testing.T) {
	manager := newTestManager(t, testSSLConfig(false), "api.example.com")
	root := manager.Locator().SSLRoot

	if certs, err := manager.ManagedCertificates(); err != nil || len(certs) != 0 {
		t.Fatalf("ManagedCertificates() on an empty tree = %d certs, %v; want 0, nil", len(certs), err)
	}

	writePair(t, LetsEncryptDir(root, "api.example.com"), FullChainFile, PrivKeyFile, validSpec("api.example.com"))
	writePair(t, LetsEncryptDir(root, "www.example.com"), FullChainFile, PrivKeyFile, validSpec("www.example.com"))
	writePair(t, LocalDir(root, "local.example.com"), LocalCertFile, LocalKeyFile, validSpec("local.example.com"))

	certs, err := manager.ManagedCertificates()
	if err != nil {
		t.Fatalf("ManagedCertificates() unexpected error: %v", err)
	}
	if len(certs) != 2 {
		t.Fatalf("ManagedCertificates() returned %d certificates, want 2", len(certs))
	}

	found := make(map[string]Source, len(certs))
	for _, cert := range certs {
		found[cert.FQDN] = cert.Source
	}
	for _, host := range []string{"api.example.com", "www.example.com"} {
		source, ok := found[host]
		if !ok {
			t.Errorf("ManagedCertificates() missing %q", host)
			continue
		}
		if source != SourceLetsEncrypt {
			t.Errorf("%q source = %q, want %q", host, source, SourceLetsEncrypt)
		}
	}
}

func TestManagedCertificatesReportsUnreadableEntries(t *testing.T) {
	manager := newTestManager(t, testSSLConfig(false), "api.example.com")
	root := manager.Locator().SSLRoot

	writePair(t, LetsEncryptDir(root, "api.example.com"), FullChainFile, PrivKeyFile, validSpec("api.example.com"))
	if err := mkdirAllForTest(filepath.Join(root, letsEncryptSubdir, "broken.example.com")); err != nil {
		t.Fatalf("create broken directory: %v", err)
	}

	certs, err := manager.ManagedCertificates()
	if err == nil {
		t.Error("ManagedCertificates() error = nil, want a report for the unreadable directory")
	}
	if len(certs) != 1 {
		t.Fatalf("ManagedCertificates() returned %d certificates, want the one that parsed", len(certs))
	}
}
