package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/common/httputil"
	"github.com/webappsgo/redxt/src/health"
)

// okChecks returns a snapshot in which every required check passes.
func okChecks() health.ChecksInfo {
	return health.ChecksInfo{
		Database:    health.CheckOK,
		Cache:       health.CheckOK,
		Disk:        health.CheckOK,
		Scheduler:   health.CheckOK,
		DNSListener: health.CheckOK,
		Zones:       health.CheckOK,
	}
}

func TestBuildHealthResponseFillsTheStaticHalf(t *testing.T) {
	o := testOptions(t)

	doc := BuildHealthResponse(o, HealthSnapshot{Checks: okChecks()})

	if doc.Project.Name != o.Config.Server.ApplicationName {
		t.Errorf("project.name = %q, want %q", doc.Project.Name, o.Config.Server.ApplicationName)
	}
	if doc.Mode != "production" {
		t.Errorf("mode = %q, want %q", doc.Mode, "production")
	}
	if doc.Uptime == "" {
		t.Error("uptime is empty, want the formatted process uptime")
	}
	if doc.Timestamp.IsZero() {
		t.Error("timestamp is zero, want the time the document was built")
	}
	if doc.Status != health.StatusHealthy {
		t.Errorf("status = %q, want %q", doc.Status, health.StatusHealthy)
	}
}

func TestBuildHealthResponseAppliesTheState(t *testing.T) {
	o := testOptions(t)

	tests := []struct {
		name       string
		snap       HealthSnapshot
		wantStatus string
		wantReason bool
	}{
		{
			name:       "healthy",
			snap:       HealthSnapshot{Checks: okChecks()},
			wantStatus: health.StatusHealthy,
		},
		{
			name: "a failing optional check degrades",
			snap: func() HealthSnapshot {
				c := okChecks()
				c.Cache = health.CheckError
				return HealthSnapshot{Checks: c}
			}(),
			wantStatus: health.StatusDegraded,
		},
		{
			name: "a failing critical check is unhealthy",
			snap: func() HealthSnapshot {
				c := okChecks()
				c.Zones = health.CheckError
				return HealthSnapshot{Checks: c}
			}(),
			wantStatus: health.StatusUnhealthy,
		},
		{
			name: "shutdown outranks the checks",
			snap: HealthSnapshot{
				State:  health.State{ShuttingDown: true},
				Checks: okChecks(),
			},
			wantStatus: health.StatusShuttingDown,
		},
		{
			name: "a pending restart carries its reasons",
			snap: HealthSnapshot{
				State:   health.State{PendingRestart: true},
				Checks:  okChecks(),
				Reasons: []string{"listen address changed"},
			},
			wantStatus: health.StatusRestartRequired,
			wantReason: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := BuildHealthResponse(o, tt.snap)

			if doc.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", doc.Status, tt.wantStatus)
			}
			if got := len(doc.RestartReason) > 0; got != tt.wantReason {
				t.Errorf("restart_reason present = %v, want %v", got, tt.wantReason)
			}
		})
	}
}

func TestHealthHandlerStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		snap HealthSnapshot
		want int
	}{
		{"healthy is 200", HealthSnapshot{Checks: okChecks()}, http.StatusOK},
		{
			name: "degraded is still 200",
			snap: func() HealthSnapshot {
				c := okChecks()
				c.Scheduler = health.CheckError
				return HealthSnapshot{Checks: c}
			}(),
			want: http.StatusOK,
		},
		{
			name: "unhealthy is 503",
			snap: func() HealthSnapshot {
				c := okChecks()
				c.Database = health.CheckError
				return HealthSnapshot{Checks: c}
			}(),
			want: http.StatusServiceUnavailable,
		},
		{
			name: "shutting down is 503",
			snap: HealthSnapshot{State: health.State{ShuttingDown: true}, Checks: okChecks()},
			want: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t)
			o.HealthProbe = StaticHealthProbe(tt.snap)

			req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
			rec := httptest.NewRecorder()
			NewHealthHandler(o, httputil.NegotiateAPI).ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestHealthHandlerWithoutAProbe(t *testing.T) {
	// A server with no probe still answers with a complete document
	// rather than failing; the checks are simply absent.
	o := testOptions(t)
	o.HealthProbe = nil

	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	NewHealthHandler(o, httputil.NegotiateAPI).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"status"`) {
		t.Errorf("body is not a health document:\n%s", rec.Body.String())
	}
}

func TestHealthHTMLOmitsDisabledOptionalChecks(t *testing.T) {
	o := testOptions(t)

	doc := BuildHealthResponse(o, HealthSnapshot{Checks: okChecks()})
	page := healthHTML(doc)

	if !strings.Contains(page, "Check: database") {
		t.Errorf("health page is missing the database check:\n%s", page)
	}
	if strings.Contains(page, "Check: tor") {
		t.Errorf("health page reports a check for a disabled subsystem:\n%s", page)
	}
}

func TestHealthHTMLEscapesTheApplicationName(t *testing.T) {
	o := testOptions(t)
	o.Config.Server.ApplicationName = `<script>alert(1)</script>`

	doc := BuildHealthResponse(o, HealthSnapshot{Checks: okChecks()})
	page := healthHTML(doc)

	if strings.Contains(page, "<script>") {
		t.Errorf("health page did not escape the application name:\n%s", page)
	}
}

func TestSortedCheckNames(t *testing.T) {
	tests := []struct {
		name   string
		checks health.ChecksInfo
		want   []string
	}{
		{
			name:   "required checks only",
			checks: okChecks(),
			want:   []string{"cache", "database", "disk", "dns_listener", "scheduler", "zones"},
		},
		{
			name: "an enabled optional check joins the list in order",
			checks: func() health.ChecksInfo {
				c := okChecks()
				c.Tor = health.CheckOK
				return c
			}(),
			want: []string{"cache", "database", "disk", "dns_listener", "scheduler", "tor", "zones"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortedCheckNames(tt.checks)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("sortedCheckNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaticHealthProbe(t *testing.T) {
	snap := HealthSnapshot{Checks: okChecks()}
	probe := StaticHealthProbe(snap)

	got := probe(context.Background())
	if got.Checks != snap.Checks {
		t.Errorf("probe returned %+v, want %+v", got.Checks, snap.Checks)
	}
}
