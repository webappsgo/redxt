package geoip

import (
	"net"
	"testing"

	"github.com/webappsgo/redxt/src/config"
)

// TestLookupPrivateIPsShortCircuit checks that every private/internal
// range AI.md PART 20 lists is never looked up: Lookup must return an
// empty Result without touching the (nil, in these cases) readers.
func TestLookupPrivateIPsShortCircuit(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		{"rfc1918-10", "10.1.2.3"},
		{"rfc1918-172-16", "172.16.0.1"},
		{"rfc1918-192-168", "192.168.1.1"},
		{"loopback-v4", "127.0.0.1"},
		{"loopback-v6", "::1"},
		{"link-local-v4", "169.254.1.1"},
		{"link-local-v6", "fe80::1"},
		{"rfc4193-unique-local", "fd00::1"},
		{"unspecified-v4", "0.0.0.0"},
		{"unspecified-v6", "::"},
	}

	s := &Service{cfg: config.GeoIP{Databases: config.GeoIPDatabases{ASN: true, Country: true, City: true}}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.Lookup(net.ParseIP(tc.ip))
			if got != (Result{}) {
				t.Fatalf("Lookup(%s) = %+v, want empty Result", tc.ip, got)
			}
		})
	}
}

// TestLookupNoReadersLoaded checks that a public IP with no readers
// loaded yet (first run, before any Refresh) resolves to an empty
// Result rather than panicking.
func TestLookupNoReadersLoaded(t *testing.T) {
	s := &Service{cfg: config.GeoIP{Databases: config.GeoIPDatabases{ASN: true, Country: true, City: true}}}
	got := s.Lookup(net.ParseIP("8.8.8.8"))
	if got != (Result{}) {
		t.Fatalf("Lookup with no readers = %+v, want empty Result", got)
	}
}

// TestCountryBlockedPrecedence exercises the full AI.md PART 20
// precedence table against an already-resolved country code.
func TestCountryBlockedPrecedence(t *testing.T) {
	cases := []struct {
		name    string
		deny    []string
		allow   []string
		code    string
		blocked bool
	}{
		{"both-empty-allows-everything", nil, nil, "CN", false},
		{"deny-only-blocks-listed", []string{"CN", "RU"}, nil, "CN", true},
		{"deny-only-allows-unlisted", []string{"CN", "RU"}, nil, "US", false},
		{"deny-only-case-insensitive", []string{"cn"}, nil, "CN", true},
		{"allow-only-allows-listed", nil, []string{"US", "CA", "GB"}, "US", false},
		{"allow-only-blocks-unlisted", nil, []string{"US", "CA", "GB"}, "CN", true},
		{"both-set-allow-wins-blocks", []string{"US"}, []string{"US", "CA"}, "US", false},
		{"both-set-allow-wins-still-blocks-unlisted", []string{"CN"}, []string{"US", "CA"}, "CN", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.GeoIP{DenyCountries: tc.deny, AllowCountries: tc.allow}
			got := countryBlocked(cfg, tc.code)
			if got != tc.blocked {
				t.Fatalf("countryBlocked(deny=%v, allow=%v, %q) = %v, want %v", tc.deny, tc.allow, tc.code, got, tc.blocked)
			}
		})
	}
}

// TestBlockedFailsOpenWithoutCountryDatabase checks the two fail-open
// paths AI.md PART 20 requires: country blocking configured but the
// category is disabled, and configured but not loaded yet. Both must
// return false (not blocked), never true.
func TestBlockedFailsOpenWithoutCountryDatabase(t *testing.T) {
	t.Run("database-category-disabled", func(t *testing.T) {
		s := &Service{
			cfg: config.GeoIP{
				DenyCountries: []string{"CN"},
				Databases:     config.GeoIPDatabases{Country: false},
			},
			log: noopLogger{},
		}
		if s.Blocked(net.ParseIP("8.8.8.8")) {
			t.Fatal("Blocked() = true with country database disabled, want fail-open false")
		}
	})

	t.Run("database-not-loaded-yet", func(t *testing.T) {
		s := &Service{
			cfg: config.GeoIP{
				DenyCountries: []string{"CN"},
				Databases:     config.GeoIPDatabases{Country: true},
			},
			log: noopLogger{},
		}
		if s.Blocked(net.ParseIP("8.8.8.8")) {
			t.Fatal("Blocked() = true with country database not loaded, want fail-open false")
		}
	})

	t.Run("no-lists-configured-skips-database-check-entirely", func(t *testing.T) {
		s := &Service{cfg: config.GeoIP{}, log: noopLogger{}}
		if s.Blocked(net.ParseIP("8.8.8.8")) {
			t.Fatal("Blocked() = true with no lists configured, want false")
		}
	})
}

// TestBlockedNeverBlocksPrivateIPs checks that country blocking never
// applies to private/internal addresses even when deny/allow lists are
// configured.
func TestBlockedNeverBlocksPrivateIPs(t *testing.T) {
	s := &Service{
		cfg: config.GeoIP{
			AllowCountries: []string{"US"},
			Databases:      config.GeoIPDatabases{Country: true},
		},
		log: noopLogger{},
	}
	if s.Blocked(net.ParseIP("192.168.1.1")) {
		t.Fatal("Blocked() = true for a private IP, want false")
	}
}

// TestTargetsSelectsPerEnabledDatabase checks that Refresh's download
// target list matches exactly the categories enabled in
// server.geoip.databases.*, and that City contributes both its IPv4 and
// IPv6 files under a single enable flag.
func TestTargetsSelectsPerEnabledDatabase(t *testing.T) {
	cases := []struct {
		name  string
		dbs   config.GeoIPDatabases
		files []string
	}{
		{"none-enabled", config.GeoIPDatabases{}, nil},
		{"asn-only", config.GeoIPDatabases{ASN: true}, []string{asnFile}},
		{"country-only", config.GeoIPDatabases{Country: true}, []string{countryFile}},
		{"city-only-both-files", config.GeoIPDatabases{City: true}, []string{cityIPv4File, cityIPv6File}},
		{"all-enabled", config.GeoIPDatabases{ASN: true, Country: true, City: true},
			[]string{asnFile, countryFile, cityIPv4File, cityIPv6File}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{cfg: config.GeoIP{Databases: tc.dbs}, urls: defaultDownloadURLs}
			targets := s.targets()
			if len(targets) != len(tc.files) {
				t.Fatalf("targets() = %v, want files %v", targets, tc.files)
			}
			for i, want := range tc.files {
				if targets[i].file != want {
					t.Fatalf("targets()[%d].file = %q, want %q", i, targets[i].file, want)
				}
				if targets[i].url == "" {
					t.Fatalf("targets()[%d].url is empty for file %q", i, want)
				}
			}
		})
	}
}

// TestNewResolvesDefaultDir checks that an empty config.GeoIP.Dir falls
// back to the caller-supplied default (the {data_dir}/security/geoip
// path resolved from src/paths), while a configured Dir always wins.
func TestNewResolvesDefaultDir(t *testing.T) {
	t.Run("empty-dir-uses-default", func(t *testing.T) {
		s := New(config.GeoIP{}, "/data/security/geoip", nil)
		if s.Dir() != "/data/security/geoip" {
			t.Fatalf("Dir() = %q, want default", s.Dir())
		}
	})

	t.Run("configured-dir-wins", func(t *testing.T) {
		s := New(config.GeoIP{Dir: "/custom/geoip"}, "/data/security/geoip", nil)
		if s.Dir() != "/custom/geoip" {
			t.Fatalf("Dir() = %q, want configured value", s.Dir())
		}
	})
}

// TestAttributionConstantsAreVerbatim guards the AI.md PART 20 license
// condition: the exported constants must match the spec's exact text so
// a UI layer rendering them never drifts from what the license requires.
func TestAttributionConstantsAreVerbatim(t *testing.T) {
	if AttributionHTML != `<a href="https://db-ip.com/">IP Geolocation by DB-IP</a>` {
		t.Fatalf("AttributionHTML changed: %q", AttributionHTML)
	}
	if AttributionText != `Country and ASN data licensed CC BY 4.0 by the Number Resource Organization (NRO).` {
		t.Fatalf("AttributionText changed: %q", AttributionText)
	}
}
