package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corsRequest builds a request carrying an Origin, optionally arriving
// through a reverse proxy that announced a forwarded host.
func corsRequest(method, origin, forwardedHost string) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/records", nil)
	req.RemoteAddr = "203.0.113.9:41000"
	if origin != "" {
		req.Header.Set(HeaderOrigin, origin)
	}
	if forwardedHost != "" {
		req.Header.Set(HeaderForwardedHost, forwardedHost)
	}
	return req
}

func TestResolveCORSTiers(t *testing.T) {
	trustAll := func(string) bool { return true }

	cases := []struct {
		name        string
		opts        CORSOptions
		forwarded   string
		wantEnabled bool
		wantTier    CORSTier
		wantOrigins []string
		reason      string
	}{
		{
			name:        "empty config disables cors outright",
			opts:        CORSOptions{Configured: ""},
			wantEnabled: false,
			wantTier:    CORSTierDisabled,
			reason:      "an empty web.cors is the documented off switch and stops resolution",
		},
		{
			name:        "whitespace only config disables cors",
			opts:        CORSOptions{Configured: "   "},
			wantEnabled: false,
			wantTier:    CORSTierDisabled,
			reason:      "a blank value is an empty value once trimmed",
		},
		{
			name:        "explicit config wins over every later source",
			opts:        CORSOptions{Configured: "https://a.test, https://b.test", Domains: []string{"ignored.test"}, IsTrustedProxy: trustAll},
			forwarded:   "learned.test",
			wantEnabled: true,
			wantTier:    CORSTierConfig,
			wantOrigins: []string{"https://a.test", "https://b.test"},
			reason:      "an operator who names origins must not have them widened by DOMAIN or a proxy",
		},
		{
			name:        "domain expands to https origins",
			opts:        CORSOptions{Configured: CORSWildcard, Domains: []string{"example.test", "alt.example.test"}},
			wantEnabled: true,
			wantTier:    CORSTierDomain,
			wantOrigins: []string{"https://example.test", "https://alt.example.test"},
			reason:      "tier 2 turns each DOMAIN entry into one https origin",
		},
		{
			name:        "trusted proxy host is learned",
			opts:        CORSOptions{Configured: CORSWildcard, IsTrustedProxy: trustAll},
			forwarded:   "Proxy.Example.Test",
			wantEnabled: true,
			wantTier:    CORSTierLearned,
			wantOrigins: []string{"https://proxy.example.test"},
			reason:      "tier 3 learns the public host from a proxy we trust",
		},
		{
			name:        "untrusted peer's forwarded host is ignored",
			opts:        CORSOptions{Configured: CORSWildcard, IsTrustedProxy: func(string) bool { return false }},
			forwarded:   "attacker.test",
			wantEnabled: true,
			wantTier:    CORSTierWildcard,
			wantOrigins: []string{CORSWildcard},
			reason:      "any client can forge X-Forwarded-Host, so only a trusted peer's value counts",
		},
		{
			name:        "nil trust predicate trusts nothing",
			opts:        CORSOptions{Configured: CORSWildcard},
			forwarded:   "attacker.test",
			wantEnabled: true,
			wantTier:    CORSTierWildcard,
			wantOrigins: []string{CORSWildcard},
			reason:      "an unconfigured proxy list must fail closed",
		},
		{
			name:        "no source at all falls back to the wildcard",
			opts:        CORSOptions{Configured: CORSWildcard},
			wantEnabled: true,
			wantTier:    CORSTierWildcard,
			wantOrigins: []string{CORSWildcard},
			reason:      "the shipped default must work with zero configuration",
		},
		{
			name: "stored learned origins join the domain list",
			opts: CORSOptions{
				Configured:     CORSWildcard,
				Domains:        []string{"example.test"},
				LearnedOrigins: func() []string { return []string{"https://edge.example.test", "https://example.test"} },
			},
			wantEnabled: true,
			wantTier:    CORSTierLearned,
			wantOrigins: []string{"https://example.test", "https://edge.example.test"},
			reason:      "previously learned origins extend the list without duplicating a DOMAIN entry",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := ResolveCORS(tc.opts, corsRequest(http.MethodGet, "https://a.test", tc.forwarded))

			if decision.Enabled != tc.wantEnabled {
				t.Fatalf("enabled = %v, want %v: %s", decision.Enabled, tc.wantEnabled, tc.reason)
			}
			if decision.Tier != tc.wantTier {
				t.Errorf("tier = %q, want %q: %s", decision.Tier, tc.wantTier, tc.reason)
			}
			if got := strings.Join(decision.Origins, ","); got != strings.Join(tc.wantOrigins, ",") {
				t.Errorf("origins = %q, want %q: %s", got, strings.Join(tc.wantOrigins, ","), tc.reason)
			}
			wantExplicit := tc.wantEnabled && tc.wantTier != CORSTierWildcard
			if decision.Explicit != wantExplicit {
				t.Errorf("explicit = %v, want %v: credentials ride only on a named allow-list", decision.Explicit, wantExplicit)
			}
		})
	}
}

func TestCORSDecisionAllows(t *testing.T) {
	cases := []struct {
		name      string
		decision  CORSDecision
		origin    string
		wantValue string
		wantOK    bool
		reason    string
	}{
		{
			name:     "disabled matches nothing",
			decision: CORSDecision{Enabled: false, Tier: CORSTierDisabled},
			origin:   "https://a.test",
			reason:   "a disabled policy must never echo an origin back",
		},
		{
			name:      "wildcard matches any origin",
			decision:  CORSDecision{Enabled: true, Tier: CORSTierWildcard, Origins: []string{CORSWildcard}},
			origin:    "https://anything.test",
			wantValue: CORSWildcard,
			wantOK:    true,
			reason:    "the fallback tier answers every origin with the wildcard",
		},
		{
			name:      "named origin matches case-insensitively",
			decision:  CORSDecision{Enabled: true, Tier: CORSTierConfig, Origins: []string{"https://App.Example.Test"}, Explicit: true},
			origin:    "https://app.example.test",
			wantValue: "https://App.Example.Test",
			wantOK:    true,
			reason:    "scheme and host are case-insensitive, so the comparison must be too",
		},
		{
			name:     "unlisted origin is refused",
			decision: CORSDecision{Enabled: true, Tier: CORSTierConfig, Origins: []string{"https://a.test"}, Explicit: true},
			origin:   "https://evil.test",
			reason:   "an origin nobody configured must not be echoed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := tc.decision.Allows(tc.origin)
			if ok != tc.wantOK || value != tc.wantValue {
				t.Errorf("Allows(%q) = (%q, %v), want (%q, %v): %s", tc.origin, value, ok, tc.wantValue, tc.wantOK, tc.reason)
			}
		})
	}
}

func TestCORSMiddlewareActualRequest(t *testing.T) {
	cases := []struct {
		name            string
		opts            CORSOptions
		origin          string
		wantStatus      int
		wantOrigin      string
		wantCredentials string
		wantVary        bool
		reason          string
	}{
		{
			name:            "named origin gets credentials",
			opts:            CORSOptions{Configured: "https://app.example.test"},
			origin:          "https://app.example.test",
			wantStatus:      http.StatusOK,
			wantOrigin:      "https://app.example.test",
			wantCredentials: "true",
			wantVary:        true,
			reason:          "credentials are legal only against a single named origin",
		},
		{
			name:       "wildcard never carries credentials",
			opts:       CORSOptions{Configured: CORSWildcard},
			origin:     "https://anything.test",
			wantStatus: http.StatusOK,
			wantOrigin: CORSWildcard,
			wantVary:   true,
			reason:     "the Fetch standard rejects credentials alongside a wildcard origin",
		},
		{
			name:       "unlisted origin passes through unmarked",
			opts:       CORSOptions{Configured: "https://app.example.test"},
			origin:     "https://evil.test",
			wantStatus: http.StatusOK,
			wantVary:   true,
			reason:     "the browser enforces the block; refusing server-side would break non-browser clients",
		},
		{
			name:       "disabled cors adds no headers",
			opts:       CORSOptions{Configured: ""},
			origin:     "https://app.example.test",
			wantStatus: http.StatusOK,
			reason:     "an operator who turned CORS off must see no CORS headers at all",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			CORS(tc.opts)(okHandler).ServeHTTP(rec, corsRequest(http.MethodGet, tc.origin, ""))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, tc.reason)
			}
			if got := rec.Header().Get(HeaderACAOrigin); got != tc.wantOrigin {
				t.Errorf("%s = %q, want %q: %s", HeaderACAOrigin, got, tc.wantOrigin, tc.reason)
			}
			if got := rec.Header().Get(HeaderACACredentials); got != tc.wantCredentials {
				t.Errorf("%s = %q, want %q: %s", HeaderACACredentials, got, tc.wantCredentials, tc.reason)
			}
			hasVary := rec.Header().Get(HeaderVary) == HeaderOrigin
			if hasVary != tc.wantVary {
				t.Errorf("Vary: Origin present = %v, want %v: a cache must not serve one origin's response to another", hasVary, tc.wantVary)
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	req := corsRequest(http.MethodOptions, "https://app.example.test", "")
	req.Header.Set(HeaderACRequestMethod, http.MethodPut)

	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	CORS(CORSOptions{Configured: "https://app.example.test"})(handler).ServeHTTP(rec, req)

	if reached {
		t.Error("the wrapped handler ran: a preflight is answered by the CORS stage, never by a route")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: a preflight carries no body", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get(HeaderACAMethods); got != CORSAllowMethods {
		t.Errorf("%s = %q, want %q: the browser reads the method list only on a preflight", HeaderACAMethods, got, CORSAllowMethods)
	}
	if got := rec.Header().Get(HeaderACMaxAge); got != CORSMaxAge {
		t.Errorf("%s = %q, want %q: without a max-age every request pays a second round trip", HeaderACMaxAge, got, CORSMaxAge)
	}
	if got := rec.Header().Get(HeaderACAHeaders); got != CORSAllowHeaders() {
		t.Errorf("%s = %q, want the explicit list %q", HeaderACAHeaders, got, CORSAllowHeaders())
	}
	if got := rec.Header().Get(HeaderACAOrigin); got != "https://app.example.test" {
		t.Errorf("%s = %q, want the named origin: a preflight must resolve the same allow-list as the real request", HeaderACAOrigin, got)
	}
}

func TestCORSPreflightWhenDisabled(t *testing.T) {
	req := corsRequest(http.MethodOptions, "https://app.example.test", "")
	req.Header.Set(HeaderACRequestMethod, http.MethodPut)

	rec := httptest.NewRecorder()
	CORS(CORSOptions{Configured: ""})(okHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: a preflight must still be answered, just without an allow-list", rec.Code, http.StatusNoContent)
	}
	if rec.Header().Get(HeaderACAOrigin) != "" {
		t.Error("Access-Control-Allow-Origin is set with CORS disabled: the browser must reject the exchange")
	}
}

func TestCORSOptionsWithoutRequestMethodIsNotAPreflight(t *testing.T) {
	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	CORS(CORSOptions{Configured: CORSWildcard})(handler).ServeHTTP(rec, corsRequest(http.MethodOptions, "https://a.test", ""))

	if !reached {
		t.Error("the handler did not run: a bare OPTIONS is a route's own request, not a preflight")
	}
}

func TestCORSAllowHeadersIsExplicit(t *testing.T) {
	value := CORSAllowHeaders()

	if strings.Contains(value, CORSWildcard) {
		t.Fatalf("value = %q: a wildcard excludes Authorization and is invalid on a credentialed response", value)
	}

	required := []string{
		"Authorization",
		"X-API-Key",
		"X-Api-Key",
		"API-Key",
		"ApiKey",
		"X-Auth-Token",
		"X-Access-Token",
		"X-Token",
		"Token",
		"X-CSRF-Token",
		"X-XSRF-Token",
		"X-Session-ID",
		"X-Service-Token",
		"X-Internal-Token",
		"Content-Type",
		"Accept",
		"X-Requested-With",
	}

	present := map[string]bool{}
	for _, name := range strings.Split(value, ", ") {
		present[name] = true
	}
	for _, name := range required {
		if !present[name] {
			t.Errorf("%q missing from %q: a header the client may send must be advertised or the browser drops it", name, value)
		}
	}
}
