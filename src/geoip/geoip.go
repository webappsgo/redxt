// Package geoip implements AI.md PART 20: built-in GeoIP support backed
// by sapics/ip-location-db MMDB files served over the jsDelivr CDN.
//
// GeoIP is a risk signal only — never the sole access-control gate. A
// request from a blocked country still passes through every other layer
// of the security pipeline (rate limiting, authentication, authorization,
// input validation, audit logging), and a request from an allowed
// country skips none of them either. Country/ASN/city data is trivially
// bypassed via VPN, residential proxy, or Tor, so any blocking decision
// built on this package is advisory, not authoritative.
//
// The ASN database's autonomous_system_organization field is a BGP/RIR
// derived AS holder name, not an RDAP/WHOIS registrant record — it must
// never be labeled "WHOIS" in code, config, or UI text.
//
// This package never falls back to MaxMind GeoLite2. GeoLite2 requires
// accepting the MaxMind GeoLite2 EULA in addition to CC BY-SA 4.0
// attribution, which is incompatible with the project's zero-config,
// no-account-required first run.
package geoip

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/urlvars"
)

// AttributionHTML is the verbatim DB-IP attribution notice AI.md PART 20
// requires on every page that displays GeoIP-derived data.
const AttributionHTML = `<a href="https://db-ip.com/">IP Geolocation by DB-IP</a>`

// AttributionText is the verbatim NRO attribution notice AI.md PART 20
// requires alongside AttributionHTML. Both notices are required together
// — the DB-IP notice alone does not cover the NRO-sourced country data,
// and vice versa.
const AttributionText = `Country and ASN data licensed CC BY 4.0 by the Number Resource Organization (NRO).`

// Logger is the narrow logging seam this package needs, satisfied by
// src/logging's logger without importing it directly.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// noopLogger discards every message. It is the zero-value Logger so a
// Service built without one never panics.
type noopLogger struct{}

func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}

// ASN holds the fields the asn.mmdb category decodes, per AI.md PART 20.
type ASN struct {
	Number       uint   `maxminddb:"autonomous_system_number"`
	Organization string `maxminddb:"autonomous_system_organization"`
}

// Country holds the fields the geo-whois-asn-country.mmdb category
// decodes, per AI.md PART 20.
type Country struct {
	CountryCode string `maxminddb:"country_code"`
}

// City holds the fields the dbip-city-ipv4/ipv6.mmdb categories decode,
// per AI.md PART 20.
type City struct {
	City        string  `maxminddb:"city"`
	CountryCode string  `maxminddb:"country_code"`
	State1      string  `maxminddb:"state1"`
	State2      string  `maxminddb:"state2"`
	Postcode    string  `maxminddb:"postcode"`
	Latitude    float64 `maxminddb:"latitude"`
	Longitude   float64 `maxminddb:"longitude"`
	Timezone    string  `maxminddb:"timezone"`
}

// Result is what Lookup returns: whichever fields the loaded databases
// resolved for the address, empty where a category is disabled, not
// loaded yet, or did not match.
type Result struct {
	CountryCode     string
	ASNNumber       uint
	ASNOrganization string
	City            string
	State1          string
	State2          string
	Postcode        string
	Latitude        float64
	Longitude       float64
	Timezone        string
}

// readers bundles the three open MMDB readers a Service holds. A nil
// field means that category is disabled or not downloaded yet.
type readers struct {
	asn     *maxminddb.Reader
	country *maxminddb.Reader
	city4   *maxminddb.Reader
	city6   *maxminddb.Reader
}

// Service is a running GeoIP subsystem: it owns the current set of open
// MMDB readers, refreshes them via Refresh, and answers Lookup and
// Blocked calls concurrently with a refresh swapping readers underneath.
type Service struct {
	cfg    config.GeoIP
	dir    string
	log    Logger
	client *http.Client
	urls   downloadURLs

	mu  sync.RWMutex
	rdr readers

	warnOnce sync.Once
}

// New builds a Service from server.geoip.* config. dir overrides
// cfg.Dir when cfg.Dir is empty, matching config.GeoIP.Dir's documented
// {data_dir}/security/geoip default. A nil logger discards log output.
func New(cfg config.GeoIP, defaultDir string, log Logger) *Service {
	if log == nil {
		log = noopLogger{}
	}
	dir := cfg.Dir
	if dir == "" {
		dir = defaultDir
	}
	return &Service{
		cfg:    cfg,
		dir:    dir,
		log:    log,
		client: http.DefaultClient,
		urls:   defaultDownloadURLs,
	}
}

// Dir returns the directory this Service downloads and reads MMDB files
// from.
func (s *Service) Dir() string {
	return s.dir
}

// Load opens whichever enabled-category MMDB files already exist on
// disk under s.Dir(), without downloading anything. It is safe to call
// before the first Refresh — categories whose file is missing are left
// unloaded and Lookup/Blocked treat them as fail-open.
func (s *Service) Load() error {
	next := readers{}
	var errs []error

	if s.cfg.Databases.ASN {
		if r, err := openIfExists(filepath.Join(s.dir, asnFile)); err != nil {
			errs = append(errs, err)
		} else {
			next.asn = r
		}
	}
	if s.cfg.Databases.Country {
		if r, err := openIfExists(filepath.Join(s.dir, countryFile)); err != nil {
			errs = append(errs, err)
		} else {
			next.country = r
		}
	}
	if s.cfg.Databases.City {
		if r, err := openIfExists(filepath.Join(s.dir, cityIPv4File)); err != nil {
			errs = append(errs, err)
		} else {
			next.city4 = r
		}
		if r, err := openIfExists(filepath.Join(s.dir, cityIPv6File)); err != nil {
			errs = append(errs, err)
		} else {
			next.city6 = r
		}
	}

	s.swap(next)

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// openIfExists opens path with maxminddb, returning a nil reader and a
// nil error when the file does not exist yet (first run before the
// first download completes).
func openIfExists(path string) (*maxminddb.Reader, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	return maxminddb.Open(path)
}

// swap installs next as the current reader set, closing whatever
// readers it replaces.
func (s *Service) swap(next readers) {
	s.mu.Lock()
	old := s.rdr
	s.rdr = next
	s.mu.Unlock()

	closeReader(old.asn)
	closeReader(old.country)
	closeReader(old.city4)
	closeReader(old.city6)
}

// closeReader closes r if it is non-nil, discarding the error since a
// close failure on a reader being replaced or shut down is not
// actionable.
func closeReader(r *maxminddb.Reader) {
	if r != nil {
		_ = r.Close()
	}
}

// Close releases every open MMDB reader. The Service must not be used
// after Close.
func (s *Service) Close() error {
	s.swap(readers{})
	return nil
}

// Lookup resolves ip against the loaded databases. Per AI.md PART 20,
// RFC 1918, RFC 4193, loopback, link-local, and unspecified addresses
// are never looked up and Lookup returns an empty Result immediately.
func (s *Service) Lookup(ip net.IP) Result {
	if !urlvars.IsPublicIP(ip) {
		return Result{}
	}

	s.mu.RLock()
	rdr := s.rdr
	s.mu.RUnlock()

	var res Result

	if rdr.asn != nil {
		var rec ASN
		if err := rdr.asn.Lookup(ip, &rec); err == nil {
			res.ASNNumber = rec.Number
			res.ASNOrganization = rec.Organization
		}
	}

	if rdr.country != nil {
		var rec Country
		if err := rdr.country.Lookup(ip, &rec); err == nil {
			res.CountryCode = rec.CountryCode
		}
	}

	cityReader := rdr.city4
	if ip.To4() == nil {
		cityReader = rdr.city6
	}
	if cityReader != nil {
		var rec City
		if err := cityReader.Lookup(ip, &rec); err == nil {
			res.City = rec.City
			res.State1 = rec.State1
			res.State2 = rec.State2
			res.Postcode = rec.Postcode
			res.Latitude = rec.Latitude
			res.Longitude = rec.Longitude
			res.Timezone = rec.Timezone
			if res.CountryCode == "" {
				res.CountryCode = rec.CountryCode
			}
		}
	}

	return res
}

// Blocked implements the AI.md PART 20 country-blocking precedence
// table:
//
//	both lists empty          -> never blocked
//	allow_countries non-empty -> blocked unless the country is listed (wins over deny)
//	deny_countries non-empty  -> blocked when the country is listed
//
// Private/internal addresses are never blocked (Lookup already returns
// an empty Result for them). A missing or disabled Country database, or
// any lookup error, fails open: not blocked, with a warning logged once
// rather than per request.
func (s *Service) Blocked(ip net.IP) bool {
	if len(s.cfg.DenyCountries) == 0 && len(s.cfg.AllowCountries) == 0 {
		return false
	}
	if !urlvars.IsPublicIP(ip) {
		return false
	}
	if !s.cfg.Databases.Country {
		s.warnOnce.Do(func() {
			s.log.Warnf("geoip: country blocking configured but server.geoip.databases.country is disabled; failing open")
		})
		return false
	}

	s.mu.RLock()
	rdr := s.rdr.country
	s.mu.RUnlock()

	if rdr == nil {
		s.warnOnce.Do(func() {
			s.log.Warnf("geoip: country blocking configured but the country database is not loaded yet; failing open")
		})
		return false
	}

	var rec Country
	if err := rdr.Lookup(ip, &rec); err != nil || rec.CountryCode == "" {
		return false
	}

	return countryBlocked(s.cfg, rec.CountryCode)
}

// countryBlocked applies the AI.md PART 20 precedence table to an
// already-resolved country code: allow_countries wins when both lists
// are set, otherwise deny_countries blocks what it lists and everything
// is allowed when both lists are empty.
func countryBlocked(cfg config.GeoIP, countryCode string) bool {
	if len(cfg.AllowCountries) > 0 {
		return !containsFold(cfg.AllowCountries, countryCode)
	}
	if len(cfg.DenyCountries) > 0 {
		return containsFold(cfg.DenyCountries, countryCode)
	}
	return false
}

// containsFold reports whether code appears in list, comparing
// case-insensitively since config authors may not always type the
// ISO 3166-1 alpha-2 code in uppercase.
func containsFold(list []string, code string) bool {
	for _, c := range list {
		if strings.EqualFold(c, code) {
			return true
		}
	}
	return false
}
