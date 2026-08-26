package health

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   int
	}{
		{"healthy", StatusHealthy, http.StatusOK},
		{"degraded", StatusDegraded, http.StatusOK},
		{"restart required", StatusRestartRequired, http.StatusOK},
		{"unhealthy", StatusUnhealthy, http.StatusServiceUnavailable},
		{"maintenance", StatusMaintenance, http.StatusServiceUnavailable},
		{"shutting down", StatusShuttingDown, http.StatusServiceUnavailable},
		{"unknown fails closed", "banana", http.StatusServiceUnavailable},
		{"empty fails closed", "", http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPStatus(tt.status); got != tt.want {
				t.Fatalf("HTTPStatus(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	if got := Check(true); got != CheckOK {
		t.Fatalf("Check(true) = %q, want %q", got, CheckOK)
	}
	if got := Check(false); got != CheckError {
		t.Fatalf("Check(false) = %q, want %q", got, CheckError)
	}
}

// allOK returns a checks value where every populated component passes.
func allOK() ChecksInfo {
	return ChecksInfo{
		Database:    CheckOK,
		Cache:       CheckOK,
		Disk:        CheckOK,
		Scheduler:   CheckOK,
		DNSListener: CheckOK,
		Zones:       CheckOK,
	}
}

func TestOverall(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		mutate func(*ChecksInfo)
		want   string
	}{
		{
			name: "all passing is healthy",
			want: StatusHealthy,
		},
		{
			name:  "pending restart on a healthy instance",
			state: State{PendingRestart: true},
			want:  StatusRestartRequired,
		},
		{
			name:   "failed database is unhealthy",
			mutate: func(c *ChecksInfo) { c.Database = CheckError },
			want:   StatusUnhealthy,
		},
		{
			name:   "failed dns listener is unhealthy",
			mutate: func(c *ChecksInfo) { c.DNSListener = CheckError },
			want:   StatusUnhealthy,
		},
		{
			name:   "failed zone load is unhealthy",
			mutate: func(c *ChecksInfo) { c.Zones = CheckError },
			want:   StatusUnhealthy,
		},
		{
			name:   "failed cache only degrades",
			mutate: func(c *ChecksInfo) { c.Cache = CheckError },
			want:   StatusDegraded,
		},
		{
			name:   "failed scheduler only degrades",
			mutate: func(c *ChecksInfo) { c.Scheduler = CheckError },
			want:   StatusDegraded,
		},
		{
			name:   "failed optional forwarders degrade",
			mutate: func(c *ChecksInfo) { c.Forwarders = CheckError },
			want:   StatusDegraded,
		},
		{
			name:   "absent optional check is ignored",
			mutate: func(c *ChecksInfo) { c.Blocklists = "" },
			want:   StatusHealthy,
		},
		{
			name:   "degraded outranks pending restart",
			state:  State{PendingRestart: true},
			mutate: func(c *ChecksInfo) { c.Disk = CheckError },
			want:   StatusDegraded,
		},
		{
			name:   "unhealthy outranks degraded",
			mutate: func(c *ChecksInfo) { c.Database = CheckError; c.Cache = CheckError },
			want:   StatusUnhealthy,
		},
		{
			name:   "maintenance outranks a failing check",
			state:  State{Maintenance: true},
			mutate: func(c *ChecksInfo) { c.Database = CheckError },
			want:   StatusMaintenance,
		},
		{
			name:   "shutdown outranks maintenance",
			state:  State{ShuttingDown: true, Maintenance: true},
			mutate: func(c *ChecksInfo) { c.Database = CheckError },
			want:   StatusShuttingDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := allOK()
			if tt.mutate != nil {
				tt.mutate(&checks)
			}
			if got := Overall(tt.state, checks); got != tt.want {
				t.Fatalf("Overall() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyClearsStaleRestartReasons(t *testing.T) {
	r := &Response{Checks: allOK(), RestartReason: []string{"server.port"}}
	r.Apply(State{}, []string{"server.port"})
	if r.Status != StatusHealthy {
		t.Fatalf("status = %q, want %q", r.Status, StatusHealthy)
	}
	if r.PendingRestart {
		t.Fatal("pending_restart set without a pending restart")
	}
	if r.RestartReason != nil {
		t.Fatalf("restart_reason = %v, want nil", r.RestartReason)
	}

	r.Apply(State{PendingRestart: true}, []string{"server.port", "server.listen"})
	if r.Status != StatusRestartRequired {
		t.Fatalf("status = %q, want %q", r.Status, StatusRestartRequired)
	}
	if len(r.RestartReason) != 2 {
		t.Fatalf("restart_reason = %v, want two entries", r.RestartReason)
	}
}

func TestApplyCopiesRestartReasons(t *testing.T) {
	reasons := []string{"server.port"}
	r := &Response{Checks: allOK()}
	r.Apply(State{PendingRestart: true}, reasons)
	reasons[0] = "mutated"
	if r.RestartReason[0] != "server.port" {
		t.Fatalf("restart_reason aliased the caller slice: %v", r.RestartReason)
	}
}

// sample returns a fully populated response for the rendering tests.
func sample() *Response {
	return &Response{
		Project: ProjectInfo{
			Name:        "redxt",
			Tagline:     "The Complete DNS Server",
			Description: "Authoritative and recursive DNS",
		},
		Status:    StatusHealthy,
		Version:   "1.0.0",
		GoVersion: "go1.27.0",
		Build:     BuildInfo{Commit: "abc1234", Date: "2026-01-10T10:00:00Z"},
		Uptime:    "2d 5h 30m",
		Mode:      "production",
		Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Cluster: ClusterInfo{
			Enabled:   true,
			Status:    ClusterConnected,
			Primary:   "https://ns1.example.com",
			Nodes:     []string{"https://ns1.example.com", "https://ns2.example.com"},
			NodeCount: 2,
			Role:      RoleMember,
		},
		Checks: allOK(),
		Stats:  StatsInfo{RequestsTotal: 10, Requests24h: 5, ActiveConns: 1, QueriesTotal: 99},
	}
}

func TestTextRendersCanonicalOrder(t *testing.T) {
	out := sample().Text()
	want := []string{
		"project.name: redxt",
		"status: healthy",
		"version: 1.0.0",
		"go_version: go1.27.0",
		"build.commit: abc1234",
		"uptime: 2d 5h 30m",
		"mode: production",
		"timestamp: 2026-01-15T10:30:00Z",
		"cluster.enabled: true",
		"cluster.nodes: https://ns1.example.com, https://ns2.example.com",
		"cluster.node_count: 2",
		"features.geoip: false",
		"features.dnssec: false",
		"checks.database: ok",
		"checks.dns_listener: ok",
		"stats.queries_total: 99",
		"stats.cache_hit_ratio: 0.0000",
	}
	last := -1
	for _, key := range want {
		idx := strings.Index(out, key)
		if idx < 0 {
			t.Fatalf("missing line %q in:\n%s", key, out)
		}
		if idx <= last {
			t.Fatalf("line %q is out of canonical order", key)
		}
		last = idx
	}
}

func TestTextOmitsDisabledOptionalChecks(t *testing.T) {
	out := sample().Text()
	for _, key := range []string{"checks.cluster", "checks.tor", "checks.i2p", "checks.forwarders", "checks.blocklists"} {
		if strings.Contains(out, key) {
			t.Fatalf("unset optional check %q was rendered", key)
		}
	}
}

func TestTextOmitsRestartFieldsWhenNotPending(t *testing.T) {
	out := sample().Text()
	if strings.Contains(out, "pending_restart") || strings.Contains(out, "restart_reason") {
		t.Fatalf("restart fields rendered on a healthy response:\n%s", out)
	}
}

func TestTextSanitizesValues(t *testing.T) {
	r := sample()
	r.Project.Name = "evil\nstatus: healthy\x1b[31m\x00"
	out := r.Text()
	forged := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "status: ") {
			forged++
		}
	}
	if forged != 1 {
		t.Fatalf("injected line was not neutralized, got %d status lines:\n%s", forged, out)
	}
	if strings.ContainsAny(out, "\x00\x1b") {
		t.Fatalf("control bytes survived sanitization:\n%q", out)
	}
}

func TestJSONIsBareAndOrdered(t *testing.T) {
	b, err := json.Marshal(sample())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, `"ok":`) || strings.Contains(out, `"data":`) {
		t.Fatalf("health response was wrapped in the API envelope: %s", out)
	}
	if strings.Contains(out, `"pending_restart"`) || strings.Contains(out, `"restart_reason"`) {
		t.Fatalf("omitempty restart fields were emitted: %s", out)
	}
	order := []string{`"project"`, `"status"`, `"version"`, `"go_version"`, `"build"`, `"uptime"`, `"mode"`, `"timestamp"`, `"cluster"`, `"features"`, `"checks"`, `"stats"`}
	last := -1
	for _, key := range order {
		idx := strings.Index(out, key)
		if idx < 0 {
			t.Fatalf("missing key %q in %s", key, out)
		}
		if idx <= last {
			t.Fatalf("key %q is out of canonical order", key)
		}
		last = idx
	}
}
