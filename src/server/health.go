package server

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/common/format"
	"github.com/webappsgo/redxt/src/common/httputil"
	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/health"
)

// HealthSnapshot is the live half of the health document: the parts only
// a running subsystem can report. The static half — project identity,
// version, build stamp, uptime, mode, and timestamp — is filled in by
// this package so no caller can forget a field.
type HealthSnapshot struct {
	// State carries the lifecycle flags that override the computed
	// status, such as an in-progress shutdown.
	State health.State
	// Checks reports per-component health.
	Checks health.ChecksInfo
	// Features reports which optional subsystems are active.
	Features health.FeaturesInfo
	// Cluster reports clustering status.
	Cluster health.ClusterInfo
	// Stats reports public-safe aggregate counters.
	Stats health.StatsInfo
	// Reasons explains a pending restart, one entry per reason.
	Reasons []string
}

// HealthProbe collects a live snapshot. The startup sequence supplies
// it, because it is the only place that holds the database handles, the
// scheduler, and the DNS listeners.
type HealthProbe func(ctx context.Context) HealthSnapshot

// BuildHealthResponse assembles the complete health document from a
// probe result and the process-wide static facts.
func BuildHealthResponse(o Options, snap HealthSnapshot) health.Response {
	r := health.Response{
		Project: health.ProjectInfo{
			Name:        o.Config.Server.ApplicationName,
			Tagline:     o.Config.Server.ApplicationTagline,
			Description: o.Config.Server.ApplicationTagline,
		},
		Version:   version.Version(),
		GoVersion: version.GoVersion(),
		Build: health.BuildInfo{
			Commit: version.Commit(),
			Date:   version.BuildDate(),
		},
		Uptime:    format.Uptime(o.Started),
		Mode:      o.Mode,
		Timestamp: time.Now().UTC(),
		Cluster:   snap.Cluster,
		Features:  snap.Features,
		Checks:    snap.Checks,
		Stats:     snap.Stats,
	}
	r.Apply(snap.State, snap.Reasons)
	return r
}

// NegotiateHealthFrontend resolves the format for the frontend health
// routes. AI.md PART 13 documents /server/healthz as "HTML for browsers,
// JSON for API clients, text for CLI", so an explicit JSON request is
// honored before the frontend chain — which has no JSON step — runs.
func NegotiateHealthFrontend(r *http.Request) httputil.Format {
	if httputil.AcceptsJSON(r) {
		return httputil.FormatJSON
	}
	return httputil.NegotiateFrontend(r)
}

// NewHealthHandler answers a health route in the format the negotiator
// selects. The same handler value is mounted on every health path, which
// is what lets the unversioned aliases in AI.md PART 14 be real routes
// rather than redirects.
func NewHealthHandler(o Options, negotiate Negotiator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := HealthSnapshot{}
		if o.HealthProbe != nil {
			snap = o.HealthProbe(r.Context())
		}
		doc := BuildHealthResponse(o, snap)

		WriteNegotiated(w, r, o, negotiate, Payload{
			JSON:   doc,
			Text:   doc.Text(),
			HTML:   healthHTML(doc),
			Title:  doc.Project.Name + " health",
			Status: health.HTTPStatus(doc.Status),
		})
	})
}

// healthHTML renders the health document as a table. Building it here
// rather than in src/health keeps that package free of presentation, and
// the check rows are sorted so the page is byte-stable between requests.
func healthHTML(doc health.Response) string {
	var b strings.Builder

	b.WriteString("<h1>" + escapeHTML(doc.Project.Name) + "</h1>\n")
	b.WriteString("<p>" + escapeHTML(doc.Project.Tagline) + "</p>\n")
	b.WriteString("<table>\n")
	b.WriteString("  <tr><th>Field</th><th>Value</th></tr>\n")

	rows := [][2]string{
		{"Status", doc.Status},
		{"Version", doc.Version},
		{"Commit", doc.Build.Commit},
		{"Built", doc.Build.Date},
		{"Uptime", doc.Uptime},
		{"Mode", doc.Mode},
		{"Timestamp", doc.Timestamp.Format(time.RFC3339)},
	}
	for _, row := range rows {
		b.WriteString("  <tr><td>" + escapeHTML(row[0]) + "</td><td>" +
			escapeHTML(row[1]) + "</td></tr>\n")
	}

	for _, name := range sortedCheckNames(doc.Checks) {
		b.WriteString("  <tr><td>Check: " + escapeHTML(name) + "</td><td>" +
			escapeHTML(checkValue(doc.Checks, name)) + "</td></tr>\n")
	}

	b.WriteString("</table>\n")
	return b.String()
}

// checkNames maps a display name to the corresponding ChecksInfo field.
// A single table keeps the HTML rendering and the name list in step.
func checkValues(c health.ChecksInfo) map[string]string {
	values := map[string]string{
		"database":     c.Database,
		"cache":        c.Cache,
		"disk":         c.Disk,
		"scheduler":    c.Scheduler,
		"dns_listener": c.DNSListener,
		"zones":        c.Zones,
	}
	optional := map[string]string{
		"cluster":    c.Cluster,
		"tor":        c.Tor,
		"i2p":        c.I2P,
		"forwarders": c.Forwarders,
		"blocklists": c.Blocklists,
	}
	// An empty optional check means the subsystem is off, and the JSON
	// document omits it, so the HTML table omits it too.
	for name, value := range optional {
		if value != "" {
			values[name] = value
		}
	}
	return values
}

// sortedCheckNames returns the reported check names in a stable order.
func sortedCheckNames(c health.ChecksInfo) []string {
	values := checkValues(c)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// checkValue returns one check's value by display name.
func checkValue(c health.ChecksInfo, name string) string {
	return checkValues(c)[name]
}

// StaticHealthProbe returns a probe that always reports the same
// snapshot. It exists so tests and the CLI can render a health document
// without a running server.
func StaticHealthProbe(snap HealthSnapshot) HealthProbe {
	return func(context.Context) HealthSnapshot { return snap }
}
