package startup

import (
	"context"
	"net"
	"path/filepath"
	"strconv"

	"github.com/webappsgo/redxt/src/geoip"
	"github.com/webappsgo/redxt/src/server/middleware"
)

// startGeoIP brings up the AI.md PART 20 lookup service. It never fails
// startup: an absent or unreadable database leaves the service loaded
// with no readers, which makes every lookup a miss until the scheduler's
// geoip_update task installs one.
func (s *Server) startGeoIP() {
	if !s.Config.Server.GeoIP.Enabled {
		return
	}

	service := geoip.New(s.Config.Server.GeoIP, filepath.Join(s.Paths.Security, "geoip"), s.Log)
	if err := service.Load(); err != nil {
		s.Log.Warnf("GeoIP databases are not available yet: %v", err)
	}
	s.GeoIP = service
}

// refreshGeoIP is the scheduler handler for PART 19's geoip_update task.
func (s *Server) refreshGeoIP(ctx context.Context) error {
	if s.GeoIP == nil {
		return nil
	}
	return s.GeoIP.Refresh(ctx)
}

// geoIPLookup adapts the PART 20 service to the PART 12 middleware seam.
// It returns nil when GeoIP is off, which leaves the annotation stage a
// passthrough rather than a per-request no-op.
//
// The result is an annotation and never a verdict: country blocking is a
// separate policy decision the handler layer makes with this signal
// alongside others, exactly as PART 11 requires.
func (s *Server) geoIPLookup() func(string) (middleware.GeoResult, bool) {
	if s.GeoIP == nil {
		return nil
	}

	return func(address string) (middleware.GeoResult, bool) {
		ip := net.ParseIP(address)
		if ip == nil {
			return middleware.GeoResult{}, false
		}

		result := s.GeoIP.Lookup(ip)
		if result.CountryCode == "" && result.ASNNumber == 0 && result.ASNOrganization == "" {
			return middleware.GeoResult{}, false
		}

		asn := ""
		if result.ASNNumber != 0 {
			asn = strconv.FormatUint(uint64(result.ASNNumber), 10)
		}
		return middleware.GeoResult{
			Country:      result.CountryCode,
			ASN:          asn,
			Organization: result.ASNOrganization,
		}, true
	}
}

// geoIPBlocked adapts geoip.Service.Blocked to the PART 12 middleware
// seam. It returns nil when GeoIP is off, which leaves the GeoIP stage
// a pure annotator — the same as an operator who leaves
// deny_countries/allow_countries empty, since Service.Blocked itself
// is a no-op with both lists empty.
func (s *Server) geoIPBlocked() func(string) bool {
	if s.GeoIP == nil {
		return nil
	}

	return func(address string) bool {
		ip := net.ParseIP(address)
		if ip == nil {
			return false
		}
		return s.GeoIP.Blocked(ip)
	}
}
