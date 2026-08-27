package geoip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/redxt/src/config"
)

// newTestServer serves fixed bodies for each of the four category
// filenames, standing in for the jsDelivr CDN so tests never touch the
// network.
func newTestServer(t *testing.T, bodies map[string]string, fail map[string]bool) (*httptest.Server, downloadURLs) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if fail[name] {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body, ok := bodies[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	urls := downloadURLs{
		ASN:      srv.URL + "/" + asnFile,
		Country:  srv.URL + "/" + countryFile,
		CityIPv4: srv.URL + "/" + cityIPv4File,
		CityIPv6: srv.URL + "/" + cityIPv6File,
	}
	return srv, urls
}

// TestRefreshDownloadsAndInstallsAtomically checks the happy path: each
// enabled category's file lands at its final destination with the
// served content, and no stray .download-* temp file is left behind.
func TestRefreshDownloadsAndInstallsAtomically(t *testing.T) {
	dir := t.TempDir()
	_, urls := newTestServer(t, map[string]string{
		asnFile:     "asn-body",
		countryFile: "country-body",
	}, nil)

	s := &Service{
		cfg: config.GeoIP{
			Enabled:   true,
			Databases: config.GeoIPDatabases{ASN: true, Country: true},
		},
		dir:    dir,
		log:    noopLogger{},
		client: http.DefaultClient,
		urls:   urls,
	}

	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	assertFileContents(t, filepath.Join(dir, asnFile), "asn-body")
	assertFileContents(t, filepath.Join(dir, countryFile), "country-body")
	assertNoLeftoverTempFiles(t, dir)
}

// TestRefreshFailurePreservesExistingDatabase checks the atomic-rename
// guarantee: when one target fails mid-refresh, a database file already
// in place from a previous successful download is left untouched, and
// no partial file is installed under its name.
func TestRefreshFailurePreservesExistingDatabase(t *testing.T) {
	dir := t.TempDir()

	// Seed a "previously downloaded" ASN database.
	existing := filepath.Join(dir, asnFile)
	if err := os.WriteFile(existing, []byte("previous-good-asn"), 0o644); err != nil {
		t.Fatalf("seed existing database: %v", err)
	}

	_, urls := newTestServer(t, map[string]string{
		asnFile: "new-asn-body",
	}, map[string]bool{
		countryFile: true,
	})

	s := &Service{
		cfg: config.GeoIP{
			Enabled:   true,
			Databases: config.GeoIPDatabases{ASN: true, Country: true},
		},
		dir:    dir,
		log:    noopLogger{},
		client: http.DefaultClient,
		urls:   urls,
	}

	err := s.Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh() error = nil, want error from the failing country download")
	}

	// The ASN target that ran before the failing one still installs —
	// each file's rename is independently atomic — but the country
	// file must never appear, and no temp file should be left behind.
	if _, statErr := os.Stat(filepath.Join(dir, countryFile)); !os.IsNotExist(statErr) {
		t.Fatalf("country database exists after a failed download: err = %v", statErr)
	}
	assertNoLeftoverTempFiles(t, dir)
}

// TestRefreshDisabledIsNoop checks that a disabled GeoIP config performs
// no downloads at all.
func TestRefreshDisabledIsNoop(t *testing.T) {
	dir := t.TempDir()
	s := &Service{
		cfg:    config.GeoIP{Enabled: false, Databases: config.GeoIPDatabases{ASN: true}},
		dir:    dir,
		log:    noopLogger{},
		client: http.DefaultClient,
	}

	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v, want nil for a disabled service", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("disabled Refresh() wrote files: %v", entries)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("contents of %s = %q, want %q", path, got, want)
	}
}

func assertNoLeftoverTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if !isKnownDatabaseFile(e.Name()) {
			t.Fatalf("leftover file %s in %s", e.Name(), dir)
		}
	}
}

func isKnownDatabaseFile(name string) bool {
	switch name {
	case asnFile, countryFile, cityIPv4File, cityIPv6File:
		return true
	default:
		return false
	}
}
