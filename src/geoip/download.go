package geoip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Filenames the databases are written to under Service.Dir(), matching
// the upstream ip-location-db package filenames verbatim so operators
// inspecting the directory can cross-reference AI.md PART 20's table.
const (
	asnFile      = "asn.mmdb"
	countryFile  = "geo-whois-asn-country.mmdb"
	cityIPv4File = "dbip-city-ipv4.mmdb"
	cityIPv6File = "dbip-city-ipv6.mmdb"
)

// downloadURLs holds the jsDelivr CDN URL for each category, per the
// AI.md PART 20 source table.
type downloadURLs struct {
	ASN      string
	Country  string
	CityIPv4 string
	CityIPv6 string
}

// defaultDownloadURLs are the exact jsDelivr CDN URLs AI.md PART 20
// specifies — no API key, account, or license agreement required.
var defaultDownloadURLs = downloadURLs{
	ASN:      "https://cdn.jsdelivr.net/npm/@ip-location-db/asn-mmdb/asn.mmdb",
	Country:  "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb",
	CityIPv4: "https://cdn.jsdelivr.net/npm/@ip-location-db/dbip-city-mmdb/dbip-city-ipv4.mmdb",
	CityIPv6: "https://cdn.jsdelivr.net/npm/@ip-location-db/dbip-city-mmdb/dbip-city-ipv6.mmdb",
}

// downloadTarget pairs a source URL with the filename it lands on under
// Service.Dir().
type downloadTarget struct {
	url  string
	file string
}

// targets returns the download targets for the categories this Service
// has enabled in server.geoip.databases.*. City is two files (IPv4 and
// IPv6) sharing one enable flag.
func (s *Service) targets() []downloadTarget {
	var out []downloadTarget
	if s.cfg.Databases.ASN {
		out = append(out, downloadTarget{s.urls.ASN, asnFile})
	}
	if s.cfg.Databases.Country {
		out = append(out, downloadTarget{s.urls.Country, countryFile})
	}
	if s.cfg.Databases.City {
		out = append(out, downloadTarget{s.urls.CityIPv4, cityIPv4File})
		out = append(out, downloadTarget{s.urls.CityIPv6, cityIPv6File})
	}
	return out
}

// Refresh downloads every enabled category's MMDB file and hot-swaps
// the reader set once every download has succeeded. It is the entry
// point AI.md PART 19's scheduler calls for the geoip_update task; this
// package does not register that task itself.
//
// Each file is downloaded to a temporary file in the same directory,
// then atomically renamed into place, so a failed or partial download
// never replaces a database already serving traffic. If any download
// fails, Refresh returns that error and leaves the previously loaded
// readers untouched.
//
// The post-download reload is best-effort: a reload failure (an
// installed file that fails to parse) is logged and does not fail
// Refresh, since the downloads themselves — the part this method
// guarantees atomicity for — already succeeded; the previous readers,
// if any, stay in place until a later successful reload.
func (s *Service) Refresh(ctx context.Context) error {
	if !s.cfg.Enabled {
		return nil
	}

	targets := s.targets()
	if len(targets) == 0 {
		return nil
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("geoip: create %s: %w", s.dir, err)
	}

	for _, t := range targets {
		if err := s.downloadOne(ctx, t); err != nil {
			return err
		}
	}

	s.log.Infof("geoip: refreshed %d database file(s) in %s", len(targets), s.dir)
	if err := s.Load(); err != nil {
		s.log.Errorf("geoip: reload after refresh: %v", err)
	}
	return nil
}

// downloadOne fetches t.url into a temp file beside t.file's final
// destination, then renames it into place atomically.
func (s *Service) downloadOne(ctx context.Context, t downloadTarget) error {
	dest := filepath.Join(s.dir, t.file)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return fmt.Errorf("geoip: build request for %s: %w", t.url, err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("geoip: fetch %s: %w", t.url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geoip: fetch %s: unexpected status %s", t.url, resp.Status)
	}

	tmp, err := os.CreateTemp(s.dir, t.file+".download-*")
	if err != nil {
		return fmt.Errorf("geoip: create temp file for %s: %w", t.file, err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("geoip: write %s: %w", t.file, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("geoip: close %s: %w", t.file, err)
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("geoip: install %s: %w", t.file, err)
	}

	return nil
}
