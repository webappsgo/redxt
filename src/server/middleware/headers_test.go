package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/urlvars"
)

func TestSecurityHeadersFixedValues(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(HeadersOptions{})(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/records", nil))

	cases := []struct {
		header string
		want   string
		reason string
	}{
		{HeaderContentTypeOptions, ValueNoSniff, "MIME sniffing turns an uploaded text file into executable HTML"},
		{HeaderFrameOptions, ValueSameOrigin, "clickjacking control for browsers predating frame-ancestors"},
		{HeaderXSSProtection, ValueXSSBlock, "PART 11 requires the legacy filter switch for old browsers"},
		{HeaderReferrerPolicy, ValueReferrerPolicy, "a full Referer leaks paths and query strings to third parties"},
		{HeaderPermittedCrossDomain, ValueCrossDomainNone, "Adobe cross-domain policies must be refused outright"},
		{HeaderOriginAgentCluster, ValueOriginAgentCluster, "origin-keyed clustering isolates this origin's agent"},
		{HeaderCOOP, DefaultCOOP, "isolation is off by default so ordinary embedding keeps working"},
		{HeaderCOEP, DefaultCOEP, "isolation is off by default so ordinary embedding keeps working"},
		{HeaderCORP, DefaultCORP, "resources must stay loadable cross-origin by default"},
	}

	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			if got := rec.Header().Get(tc.header); got != tc.want {
				t.Errorf("%s = %q, want %q: %s", tc.header, got, tc.want, tc.reason)
			}
		})
	}
}

func TestSecurityHeadersAlwaysPresent(t *testing.T) {
	rec := httptest.NewRecorder()
	opts := HeadersOptions{ReportURL: ReportURLFunc("/api/v1")}
	SecurityHeaders(opts)(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/records", nil))

	present := []struct {
		header string
		reason string
	}{
		{HeaderCSP, "the content security policy is the primary XSS control"},
		{HeaderPermissionsPolicy, "browser feature access must be denied by default"},
		{HeaderReportingEndpoints, "the modern Reporting API needs a named endpoint"},
		{HeaderReportTo, "the legacy Reporting API group is still what most browsers read"},
		{HeaderNEL, "network error logging reports failures the server never sees"},
	}

	for _, tc := range present {
		t.Run(tc.header, func(t *testing.T) {
			if rec.Header().Get(tc.header) == "" {
				t.Errorf("%s is empty: %s", tc.header, tc.reason)
			}
		})
	}
}

func TestSecurityHeadersRequestID(t *testing.T) {
	const fixedID = "0f4d9f2e-6a1b-4c3d-9e8f-1a2b3c4d5e6f"

	cases := []struct {
		name    string
		prelude Middleware
		reason  string
	}{
		{
			name: "an earlier stage's header is left alone",
			prelude: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set(urlvars.HeaderRequestID, fixedID)
					next.ServeHTTP(w, req)
				})
			},
			reason: "stage 2 owns the ID and the header stage must never replace it",
		},
		{
			name: "the context value is restored when no stage set the header",
			prelude: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					next.ServeHTTP(w, req.WithContext(urlvars.WithRequestID(req.Context(), fixedID)))
				})
			},
			reason: "PART 11 requires the header on every response even in a partial chain",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Chain(tc.prelude, SecurityHeaders(HeadersOptions{}))(okHandler).
				ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/records", nil))

			if got := rec.Header().Get(urlvars.HeaderRequestID); got != fixedID {
				t.Errorf("%s = %q, want %q: %s", urlvars.HeaderRequestID, got, fixedID, tc.reason)
			}
		})
	}
}

func TestSecurityHeadersCSPReportOnly(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(HeadersOptions{CSPReportOnly: true})(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Header().Get(HeaderCSP) != "" {
		t.Error("Content-Security-Policy is set in report-only mode: a report-only policy must not enforce")
	}
	if rec.Header().Get(HeaderCSPReportOnly) == "" {
		t.Error("Content-Security-Policy-Report-Only is empty: report-only mode must still send the policy")
	}
}

func TestSecurityHeadersHSTS(t *testing.T) {
	cases := []struct {
		name   string
		hsts   config.HSTS
		useTLS bool
		want   string
		reason string
	}{
		{
			name:   "enabled over tls",
			hsts:   config.HSTS{Enabled: true, MaxAge: 63072000},
			useTLS: true,
			want:   "max-age=63072000",
			reason: "PART 11 requires HSTS on every TLS response",
		},
		{
			name:   "subdomains and preload",
			hsts:   config.HSTS{Enabled: true, MaxAge: 63072000, IncludeSubdomains: true, Preload: true},
			useTLS: true,
			want:   "max-age=63072000; includeSubDomains; preload",
			reason: "the preload list requires both directives in this order",
		},
		{
			name:   "enabled over plaintext",
			hsts:   config.HSTS{Enabled: true, MaxAge: 63072000},
			useTLS: false,
			want:   "",
			reason: "RFC 6797 forbids HSTS on a non-TLS response",
		},
		{
			name:   "disabled",
			hsts:   config.HSTS{Enabled: false, MaxAge: 63072000},
			useTLS: true,
			want:   "",
			reason: "a disabled block must emit nothing at all",
		},
		{
			name:   "zero max age is the emergency disable",
			hsts:   config.HSTS{Enabled: true, MaxAge: 0},
			useTLS: true,
			want:   "",
			reason: "max-age=0 would leave the directive present with no effect",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.useTLS {
				req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
			}
			rec := httptest.NewRecorder()
			SecurityHeaders(HeadersOptions{HSTS: tc.hsts})(okHandler).ServeHTTP(rec, req)

			if got := rec.Header().Get(HeaderHSTS); got != tc.want {
				t.Errorf("%s = %q, want %q: %s", HeaderHSTS, got, tc.want, tc.reason)
			}
		})
	}
}

func TestSecurityHeadersClearSiteData(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		want   string
		reason string
	}{
		{"logout", "/server/auth/logout", ValueClearSiteData, "logging out must drop the browser's local state"},
		{"signout", "/signout", ValueClearSiteData, "the signout spelling revokes just as much as logout"},
		{"revoke", "/api/v1/tokens/revoke", ValueClearSiteData, "revoking a token invalidates the caller's cached state"},
		{"account delete", "/account/delete", ValueClearSiteData, "a deleted account must leave nothing behind locally"},
		{"ordinary path", "/records", "", "a normal response must not wipe the browser's storage"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SecurityHeaders(HeadersOptions{})(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if got := rec.Header().Get(HeaderClearSiteData); got != tc.want {
				t.Errorf("%s = %q, want %q: %s", HeaderClearSiteData, got, tc.want, tc.reason)
			}
		})
	}
}

func TestDefaultCSPIncludesLearnedOrigins(t *testing.T) {
	policy := DefaultCSP("/api/v1", []string{"https://app.example.test", "*", ""})

	if !strings.Contains(policy, "connect-src 'self' https://app.example.test") {
		t.Errorf("policy = %q: a learned origin must be appended to connect-src", policy)
	}
	if strings.Contains(policy, "connect-src 'self' https://app.example.test *") {
		t.Errorf("policy = %q: the CORS wildcard must never widen connect-src", policy)
	}
	if !strings.Contains(policy, "report-uri /api/v1/server/reports/csp") {
		t.Errorf("policy = %q: the report-uri fallback must use the configured API base", policy)
	}
	if !strings.Contains(policy, "object-src 'none'") {
		t.Errorf("policy = %q: plugin content must stay blocked", policy)
	}
}

func TestDefaultPermissionsPolicyIsDeterministic(t *testing.T) {
	first := DefaultPermissionsPolicy()
	second := DefaultPermissionsPolicy()

	if first != second {
		t.Fatalf("policy differs between calls:\n%q\n%q: the feature list must be ordered, not a map", first, second)
	}
	if !strings.HasPrefix(first, "accelerometer=()") {
		t.Errorf("policy = %q: the ordered list must start with its first entry", first)
	}
	if !strings.Contains(first, "geolocation=()") {
		t.Errorf("policy = %q: sensor access must be denied outright", first)
	}
	if !strings.Contains(first, "fullscreen=(self)") {
		t.Errorf("policy = %q: features the app itself uses must be scoped to its own origin", first)
	}
}

func TestReportURLFunc(t *testing.T) {
	build := ReportURLFunc("/api/v1")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "redxt.example.test"
	if got, want := build(req), "http://redxt.example.test/api/v1/server/reports/default"; got != want {
		t.Errorf("endpoint = %q, want %q: the endpoint must follow the host the client used", got, want)
	}

	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
	if got, want := build(req), "https://redxt.example.test/api/v1/server/reports/default"; got != want {
		t.Errorf("endpoint = %q, want %q: a TLS request must report to an https endpoint", got, want)
	}
}
