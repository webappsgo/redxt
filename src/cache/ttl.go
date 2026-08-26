package cache

import "time"

// TTL defaults from the PART 9 "Cache TTL Defaults" table.
const (
	// TTLSession matches the session.timeout default.
	TTLSession = 24 * time.Hour
	// TTLAPIToken is zero, which means no expiry: API tokens are removed from
	// the cache by explicit revocation only, never by elapsed time.
	TTLAPIToken = 0
	// TTLRateLimit is the rolling rate-limit window.
	TTLRateLimit = time.Minute
	// TTLUserProfile balances profile freshness against database load.
	TTLUserProfile = 5 * time.Minute
	// TTLConfig keeps configuration changes propagating quickly.
	TTLConfig = time.Minute
	// TTLStaticHash covers immutable static content hashes.
	TTLStaticHash = 24 * time.Hour
	// TTLGeoIP covers infrequently updated GeoIP data.
	TTLGeoIP = 7 * 24 * time.Hour
	// TTLBlocklist keeps security blocklists current.
	TTLBlocklist = time.Hour
	// TTLPage covers cached dynamic pages.
	TTLPage = 5 * time.Minute
	// TTLAPIResponse covers frequently changing API responses.
	TTLAPIResponse = 30 * time.Second
)
