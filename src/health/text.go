package health

import (
	"strconv"
	"strings"
	"time"
)

// textTimeLayout is the ISO 8601 form used for timestamps in the plain
// text rendering.
const textTimeLayout = time.RFC3339

// Text renders the health response in the flattened dot-notation form
// from AI.md PART 13 "Plain Text (Accept: text/plain)". Sections appear
// in the canonical field order, each introduced by a numbered comment,
// and every value is emitted even when empty so that a scraping client
// sees a stable key set.
func (r *Response) Text() string {
	var b strings.Builder

	b.WriteString("# 1. Project\n")
	line(&b, "project.name", r.Project.Name)
	line(&b, "project.tagline", r.Project.Tagline)
	line(&b, "project.description", r.Project.Description)

	b.WriteString("\n# 2. Status\n")
	line(&b, "status", r.Status)
	if r.PendingRestart {
		line(&b, "pending_restart", "true")
		line(&b, "restart_reason", strings.Join(r.RestartReason, ", "))
	}

	b.WriteString("\n# 3. Version & Build\n")
	line(&b, "version", r.Version)
	line(&b, "go_version", r.GoVersion)
	line(&b, "build.commit", r.Build.Commit)
	line(&b, "build.date", r.Build.Date)

	b.WriteString("\n# 4. Runtime\n")
	line(&b, "uptime", r.Uptime)
	line(&b, "mode", r.Mode)
	line(&b, "timestamp", r.Timestamp.UTC().Format(textTimeLayout))

	b.WriteString("\n# 5. Cluster\n")
	line(&b, "cluster.enabled", boolText(r.Cluster.Enabled))
	line(&b, "cluster.status", r.Cluster.Status)
	line(&b, "cluster.primary", r.Cluster.Primary)
	line(&b, "cluster.nodes", strings.Join(r.Cluster.Nodes, ", "))
	line(&b, "cluster.node_count", strconv.Itoa(r.Cluster.NodeCount))
	line(&b, "cluster.role", r.Cluster.Role)

	b.WriteString("\n# 6. Features\n")
	line(&b, "features.tor.enabled", boolText(r.Features.Tor.Enabled))
	line(&b, "features.tor.running", boolText(r.Features.Tor.Running))
	line(&b, "features.tor.status", r.Features.Tor.Status)
	line(&b, "features.tor.hostname", r.Features.Tor.Hostname)
	line(&b, "features.i2p.enabled", boolText(r.Features.I2P.Enabled))
	line(&b, "features.i2p.running", boolText(r.Features.I2P.Running))
	line(&b, "features.i2p.status", r.Features.I2P.Status)
	line(&b, "features.i2p.hostname", r.Features.I2P.Hostname)
	line(&b, "features.i2p.provider", r.Features.I2P.Provider)
	line(&b, "features.geoip", boolText(r.Features.GeoIP))
	line(&b, "features.multi_user", boolText(r.Features.MultiUser))
	line(&b, "features.organizations", boolText(r.Features.Organizations))
	line(&b, "features.custom_domains", boolText(r.Features.CustomDomains))
	line(&b, "features.authoritative", boolText(r.Features.Authoritative))
	line(&b, "features.recursion", boolText(r.Features.Recursion))
	line(&b, "features.forwarding", boolText(r.Features.Forwarding))
	line(&b, "features.dnssec", boolText(r.Features.DNSSEC))
	line(&b, "features.dot", boolText(r.Features.DoT))
	line(&b, "features.doh", boolText(r.Features.DoH))
	line(&b, "features.doq", boolText(r.Features.DoQ))
	line(&b, "features.dnscrypt", boolText(r.Features.DNSCrypt))
	line(&b, "features.filtering", boolText(r.Features.Filtering))
	line(&b, "features.ddns", boolText(r.Features.DDNS))
	line(&b, "features.redirects", boolText(r.Features.Redirects))
	line(&b, "features.data_zones", boolText(r.Features.DataZones))
	line(&b, "features.rdap", boolText(r.Features.RDAP))

	b.WriteString("\n# 7. Checks\n")
	line(&b, "checks.database", r.Checks.Database)
	line(&b, "checks.cache", r.Checks.Cache)
	line(&b, "checks.disk", r.Checks.Disk)
	line(&b, "checks.scheduler", r.Checks.Scheduler)
	optionalLine(&b, "checks.cluster", r.Checks.Cluster)
	optionalLine(&b, "checks.tor", r.Checks.Tor)
	optionalLine(&b, "checks.i2p", r.Checks.I2P)
	line(&b, "checks.dns_listener", r.Checks.DNSListener)
	line(&b, "checks.zones", r.Checks.Zones)
	optionalLine(&b, "checks.forwarders", r.Checks.Forwarders)
	optionalLine(&b, "checks.blocklists", r.Checks.Blocklists)

	b.WriteString("\n# 8. Stats\n")
	line(&b, "stats.requests_total", strconv.FormatInt(r.Stats.RequestsTotal, 10))
	line(&b, "stats.requests_24h", strconv.FormatInt(r.Stats.Requests24h, 10))
	line(&b, "stats.active_connections", strconv.Itoa(r.Stats.ActiveConns))
	line(&b, "stats.queries_total", strconv.FormatInt(r.Stats.QueriesTotal, 10))
	line(&b, "stats.queries_24h", strconv.FormatInt(r.Stats.Queries24h, 10))
	line(&b, "stats.blocked_24h", strconv.FormatInt(r.Stats.Blocked24h, 10))
	line(&b, "stats.zones_total", strconv.FormatInt(r.Stats.ZonesTotal, 10))
	line(&b, "stats.records_total", strconv.FormatInt(r.Stats.RecordsTotal, 10))
	line(&b, "stats.cache_hit_ratio", strconv.FormatFloat(r.Stats.CacheHitRatio, 'f', 4, 64))

	return b.String()
}

// line writes one "key: value" pair, sanitizing the value so that a
// branding string carrying a newline cannot forge additional keys.
func line(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(sanitizeValue(value))
	b.WriteString("\n")
}

// optionalLine writes a pair only when the value is set, matching the
// omitempty behavior of the JSON rendering for checks that belong to
// components this node does not run.
func optionalLine(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	line(b, key, value)
}

// boolText renders a boolean the way the spec's sample output does.
func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// sanitizeValue strips the control characters that could otherwise
// break the one-pair-per-line contract of the text rendering.
func sanitizeValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
