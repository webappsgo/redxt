package startup

import (
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/geoip"
)

// TestGeoIPLookupSeam covers the PART 20 to PART 12 adapter: no service
// leaves the middleware stage a passthrough, and a service with no
// database loaded reports a miss rather than an empty annotation.
func TestGeoIPLookupSeam(t *testing.T) {
	if lookup := (&Server{}).geoIPLookup(); lookup != nil {
		t.Fatalf("geoIPLookup() = non-nil with no GeoIP service")
	}

	s := &Server{GeoIP: geoip.New(config.GeoIP{Enabled: true}, t.TempDir(), nil)}
	lookup := s.geoIPLookup()
	if lookup == nil {
		t.Fatalf("geoIPLookup() = nil with a GeoIP service")
	}

	tests := []struct {
		name    string
		address string
	}{
		{name: "not an address", address: "not-an-ip"},
		{name: "empty address", address: ""},
		{name: "public address with no database", address: "8.8.8.8"},
		{name: "private address is never looked up", address: "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := lookup(tt.address)
			if ok {
				t.Fatalf("lookup(%q) = %+v, true; want a miss", tt.address, result)
			}
		})
	}
}

// TestRefreshGeoIPWithoutAService confirms the scheduler handler is a
// no-op when GeoIP is disabled, so the task can be registered safely.
func TestRefreshGeoIPWithoutAService(t *testing.T) {
	if err := (&Server{}).refreshGeoIP(t.Context()); err != nil {
		t.Fatalf("refreshGeoIP() error = %v", err)
	}
}
