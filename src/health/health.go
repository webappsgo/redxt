// Package health implements the canonical health response defined in
// AI.md PART 13 "Health & Versioning", extended with the DNS-specific
// features, checks, and statistics IDEA.md requires.
//
// The response is served unauthenticated on every health route, so
// every field in this package must be public-safe: no connection
// strings, no tokens, no internal addresses, no filesystem paths.
package health

import (
	"time"
)

// Overall status values from the AI.md PART 13 "Health Status Values &
// HTTP Codes" table.
const (
	// StatusHealthy reports that every check passes.
	StatusHealthy = "healthy"
	// StatusDegraded reports that some non-critical check is failing
	// while the server still answers requests.
	StatusDegraded = "degraded"
	// StatusUnhealthy reports that a critical check is failing.
	StatusUnhealthy = "unhealthy"
	// StatusRestartRequired reports a healthy server holding a config
	// change that only takes effect after a restart.
	StatusRestartRequired = "restart_required"
	// StatusMaintenance reports that maintenance mode is active.
	StatusMaintenance = "maintenance"
	// StatusShuttingDown reports a graceful shutdown in progress.
	StatusShuttingDown = "shutting_down"
)

// Component check values. AI.md PART 13 allows only these two words in
// the checks section so that no failure detail leaks publicly.
const (
	// CheckOK reports a passing component check.
	CheckOK = "ok"
	// CheckError reports a failing component check.
	CheckError = "error"
)

// Cluster connection values for ClusterInfo.Status.
const (
	// ClusterConnected reports a node in contact with its cluster.
	ClusterConnected = "connected"
	// ClusterDisconnected reports a node out of contact.
	ClusterDisconnected = "disconnected"
)

// Cluster role values for ClusterInfo.Role.
const (
	// RolePrimary is the elected primary node.
	RolePrimary = "primary"
	// RoleMember is any non-primary cluster node.
	RoleMember = "member"
)

// Response is the canonical health document. The field order below is
// fixed by AI.md PART 13 "Field Order & Structure" and is mirrored by
// the plain-text renderer and by the frontend; do not reorder it.
type Response struct {
	// Project identification, sourced from the branding config.
	Project ProjectInfo `json:"project"`

	// Status is one of the StatusXxx constants.
	Status string `json:"status"`
	// PendingRestart is set only when a config change needs a restart.
	PendingRestart bool `json:"pending_restart,omitempty"`
	// RestartReason names the settings that changed.
	RestartReason []string `json:"restart_reason,omitempty"`

	// Version is the SemVer application version.
	Version string `json:"version"`
	// GoVersion is the Go runtime version of this build.
	GoVersion string `json:"go_version"`
	// Build carries the build-time stamps.
	Build BuildInfo `json:"build"`

	// Uptime is human readable, for example "2d 5h 30m".
	Uptime string `json:"uptime"`
	// Mode is the resolved application mode.
	Mode string `json:"mode"`
	// Timestamp is the current UTC time.
	Timestamp time.Time `json:"timestamp"`

	// Cluster carries the cluster membership view used by agents and
	// the CLI for failover discovery.
	Cluster ClusterInfo `json:"cluster"`

	// Features reports public feature availability.
	Features FeaturesInfo `json:"features"`

	// Checks reports per-component health as ok or error.
	Checks ChecksInfo `json:"checks"`

	// Stats reports public-safe aggregate counters.
	Stats StatsInfo `json:"stats"`
}

// ProjectInfo carries the branding identity of this instance.
type ProjectInfo struct {
	// Name is the branding title.
	Name string `json:"name"`
	// Tagline is the short branding slogan.
	Tagline string `json:"tagline"`
	// Description is the longer branding description.
	Description string `json:"description"`
}

// BuildInfo carries the build-time stamps from AI.md PART 7.
type BuildInfo struct {
	// Commit is the short git hash.
	Commit string `json:"commit"`
	// Date is the ISO 8601 build timestamp.
	Date string `json:"date"`
}

// ClusterInfo describes cluster membership as seen by this node.
type ClusterInfo struct {
	// Enabled reports whether cluster mode is active.
	Enabled bool `json:"enabled"`
	// Status is ClusterConnected or ClusterDisconnected.
	Status string `json:"status,omitempty"`
	// Primary is the public URL of the elected primary node.
	Primary string `json:"primary,omitempty"`
	// Nodes lists the public URL of every known node.
	Nodes []string `json:"nodes,omitempty"`
	// NodeCount counts healthy, degraded, and offline nodes together.
	NodeCount int `json:"node_count,omitempty"`
	// Role is RolePrimary or RoleMember.
	Role string `json:"role,omitempty"`
}

// FeaturesInfo reports public feature availability. The first three
// fields are the template's non-negotiable features; the remainder are
// the redxt-specific capabilities IDEA.md defines, which an admin can
// enable or disable and which therefore report their actual state.
type FeaturesInfo struct {
	// Tor reports the hidden service state.
	Tor TorInfo `json:"tor"`
	// I2P reports the opt-in eepsite state.
	I2P I2PInfo `json:"i2p"`
	// GeoIP reports whether the optional GeoIP database is loaded.
	GeoIP bool `json:"geoip"`

	// MultiUser reports whether user accounts are enabled.
	MultiUser bool `json:"multi_user"`
	// Organizations reports whether org tenancy is enabled.
	Organizations bool `json:"organizations"`
	// CustomDomains reports whether org custom domains are enabled.
	CustomDomains bool `json:"custom_domains"`

	// Authoritative reports whether the authoritative server role is
	// serving zones.
	Authoritative bool `json:"authoritative"`
	// Recursion reports whether the recursive resolver role is active.
	Recursion bool `json:"recursion"`
	// Forwarding reports whether upstream forwarding is configured.
	Forwarding bool `json:"forwarding"`
	// DNSSEC reports whether signing and validation are enabled.
	DNSSEC bool `json:"dnssec"`
	// DoT reports the DNS-over-TLS listener state.
	DoT bool `json:"dot"`
	// DoH reports the DNS-over-HTTPS listener state.
	DoH bool `json:"doh"`
	// DoQ reports the DNS-over-QUIC listener state.
	DoQ bool `json:"doq"`
	// DNSCrypt reports the DNSCrypt listener state.
	DNSCrypt bool `json:"dnscrypt"`
	// Filtering reports whether blocklist and policy filtering is on.
	Filtering bool `json:"filtering"`
	// DDNS reports whether the dynamic DNS provider surface is on.
	DDNS bool `json:"ddns"`
	// Redirects reports whether the HTTP redirect engine is on.
	Redirects bool `json:"redirects"`
	// DataZones reports whether published data zones are being served.
	DataZones bool `json:"data_zones"`
	// RDAP reports whether the RDAP and WHOIS responders are on.
	RDAP bool `json:"rdap"`
}

// TorInfo describes the Tor hidden service.
type TorInfo struct {
	// Enabled reports that the Tor binary was found and configured.
	Enabled bool `json:"enabled"`
	// Running reports an active hidden service.
	Running bool `json:"running"`
	// Status is healthy, starting, or an error summary.
	Status string `json:"status"`
	// Hostname is the v3 onion address when running.
	Hostname string `json:"hostname"`
}

// I2PInfo describes the opt-in I2P eepsite. Every field holds its zero
// value while the feature is disabled.
type I2PInfo struct {
	// Enabled reports the opt-in setting.
	Enabled bool `json:"enabled"`
	// Running reports an active eepsite.
	Running bool `json:"running"`
	// Status is disabled, healthy, starting, or an error summary.
	Status string `json:"status"`
	// Hostname is the .b32.i2p address when running.
	Hostname string `json:"hostname"`
	// Provider is i2pd, sam, or none.
	Provider string `json:"provider"`
}

// ChecksInfo reports per-component health. Every value is CheckOK or
// CheckError, with no failure detail, because this document is public.
type ChecksInfo struct {
	// Database reports the primary datastore check.
	Database string `json:"database"`
	// Cache reports the cache backend check.
	Cache string `json:"cache"`
	// Disk reports the free space check.
	Disk string `json:"disk"`
	// Scheduler reports the background job runner check.
	Scheduler string `json:"scheduler"`
	// Cluster reports the cluster check when clustering is enabled.
	Cluster string `json:"cluster,omitempty"`
	// Tor reports the hidden service check when Tor is enabled.
	Tor string `json:"tor,omitempty"`
	// I2P reports the eepsite check when I2P is opted in.
	I2P string `json:"i2p,omitempty"`

	// DNSListener reports whether the Do53 listeners are bound.
	DNSListener string `json:"dns_listener"`
	// Zones reports whether every authoritative zone loaded.
	Zones string `json:"zones"`
	// Forwarders reports upstream forwarder reachability when
	// forwarding is configured.
	Forwarders string `json:"forwarders,omitempty"`
	// Blocklists reports the most recent blocklist refresh when
	// filtering is enabled.
	Blocklists string `json:"blocklists,omitempty"`
}

// StatsInfo reports public-safe aggregate counters. Nothing here may
// identify an individual client.
type StatsInfo struct {
	// RequestsTotal counts HTTP requests served since start.
	RequestsTotal int64 `json:"requests_total"`
	// Requests24h counts HTTP requests in the last 24 hours.
	Requests24h int64 `json:"requests_24h"`
	// ActiveConns counts currently open connections.
	ActiveConns int `json:"active_connections"`

	// QueriesTotal counts DNS queries answered since start.
	QueriesTotal int64 `json:"queries_total"`
	// Queries24h counts DNS queries in the last 24 hours.
	Queries24h int64 `json:"queries_24h"`
	// Blocked24h counts filtered DNS queries in the last 24 hours.
	Blocked24h int64 `json:"blocked_24h"`
	// ZonesTotal counts zones served authoritatively.
	ZonesTotal int64 `json:"zones_total"`
	// RecordsTotal counts resource records across all served zones.
	RecordsTotal int64 `json:"records_total"`
	// CacheHitRatio is the resolver cache hit rate from 0 to 1,
	// rounded to four decimal places.
	CacheHitRatio float64 `json:"cache_hit_ratio"`
}
