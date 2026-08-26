package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// listRequest builds a request from the given TCP peer.
func listRequest(ip string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/records", nil)
	req.RemoteAddr = ip + ":41000"
	return req
}

func TestNewCIDRMatcher(t *testing.T) {
	cases := []struct {
		name      string
		entries   []string
		candidate string
		want      bool
		reason    string
	}{
		{
			name:      "cidr block covers a member",
			entries:   []string{"10.0.0.0/8"},
			candidate: "10.4.5.6",
			want:      true,
			reason:    "an operator listing a block means every address inside it",
		},
		{
			name:      "cidr block excludes an outsider",
			entries:   []string{"10.0.0.0/8"},
			candidate: "203.0.113.9",
			reason:    "a block must not widen beyond its mask",
		},
		{
			name:      "bare address matches itself",
			entries:   []string{"203.0.113.9"},
			candidate: "203.0.113.9",
			want:      true,
			reason:    "a bare IP is a single-address block",
		},
		{
			name:      "bare address matches nothing else",
			entries:   []string{"203.0.113.9"},
			candidate: "203.0.113.10",
			reason:    "a single-address block must not cover its neighbour",
		},
		{
			name:      "ipv6 block covers a member",
			entries:   []string{"2001:db8::/32"},
			candidate: "2001:db8::1",
			want:      true,
			reason:    "IPv6 clients must be listable the same way as IPv4",
		},
		{
			name:      "empty entries are skipped",
			entries:   []string{"", "   ", "203.0.113.9"},
			candidate: "203.0.113.9",
			want:      true,
			reason:    "a trailing comma in server.yml must not break the list",
		},
		{
			name:      "unparseable candidate matches nothing",
			entries:   []string{"0.0.0.0/0"},
			candidate: "not-an-ip",
			reason:    "a garbage address must never satisfy even an all-addresses block",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match, err := NewCIDRMatcher(tc.entries)
			if err != nil {
				t.Fatalf("NewCIDRMatcher error = %v: %s", err, tc.reason)
			}
			if got := match(tc.candidate); got != tc.want {
				t.Errorf("match(%q) = %v, want %v: %s", tc.candidate, got, tc.want, tc.reason)
			}
		})
	}
}

func TestNewCIDRMatcherRejectsGarbage(t *testing.T) {
	if _, err := NewCIDRMatcher([]string{"10.0.0.0/8", "definitely not an ip"}); err == nil {
		t.Error("no error for an unparseable entry: a typo in server.yml must surface at startup, not silently narrow the list")
	}
}

func TestAllowlistMarksWithoutBlocking(t *testing.T) {
	cases := []struct {
		name    string
		opts    AllowlistOptions
		ip      string
		wantHit bool
		reason  string
	}{
		{
			name:    "listed address is marked",
			opts:    AllowlistOptions{Enabled: true, Contains: func(ip string) bool { return ip == "203.0.113.9" }},
			ip:      "203.0.113.9",
			wantHit: true,
			reason:  "the later stages read this mark instead of each keeping their own exemption list",
		},
		{
			name:   "unlisted address is not marked",
			opts:   AllowlistOptions{Enabled: true, Contains: func(ip string) bool { return ip == "203.0.113.9" }},
			ip:     "198.51.100.4",
			reason: "an ordinary client must stay subject to the block-list and the limiter",
		},
		{
			name:   "disabled stage marks nothing",
			opts:   AllowlistOptions{Enabled: false, Contains: func(string) bool { return true }},
			ip:     "203.0.113.9",
			reason: "a disabled allow-list must leave every later stage fully in force",
		},
		{
			name:   "nil matcher marks nothing",
			opts:   AllowlistOptions{Enabled: true},
			ip:     "203.0.113.9",
			reason: "an unconfigured list must fail closed rather than trust everyone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marked := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				marked = IsAllowlisted(req.Context())
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			Allowlist(tc.opts)(handler).ServeHTTP(rec, listRequest(tc.ip))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: the allow-list stage must never answer a request itself", rec.Code, http.StatusOK)
			}
			if marked != tc.wantHit {
				t.Errorf("IsAllowlisted = %v, want %v: %s", marked, tc.wantHit, tc.reason)
			}
		})
	}
}

func TestBlocklist(t *testing.T) {
	blocked := func(ip string) bool { return ip == "198.51.100.4" }

	cases := []struct {
		name       string
		opts       BlocklistOptions
		ip         string
		wantStatus int
		reason     string
	}{
		{
			name:       "blocked address is refused",
			opts:       BlocklistOptions{Enabled: true, Contains: blocked},
			ip:         "198.51.100.4",
			wantStatus: http.StatusForbidden,
			reason:     "a blocked client must cost no counter write and reach no route",
		},
		{
			name:       "unblocked address passes",
			opts:       BlocklistOptions{Enabled: true, Contains: blocked},
			ip:         "203.0.113.9",
			wantStatus: http.StatusOK,
			reason:     "an address nobody blocked must be served normally",
		},
		{
			name:       "disabled stage blocks nothing",
			opts:       BlocklistOptions{Enabled: false, Contains: blocked},
			ip:         "198.51.100.4",
			wantStatus: http.StatusOK,
			reason:     "a disabled block-list is a pass-through",
		},
		{
			name:       "nil matcher blocks nothing",
			opts:       BlocklistOptions{Enabled: true},
			ip:         "198.51.100.4",
			wantStatus: http.StatusOK,
			reason:     "an unconfigured block-list must not refuse traffic",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Blocklist(tc.opts)(okHandler).ServeHTTP(rec, listRequest(tc.ip))

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.wantStatus, tc.reason)
			}
		})
	}
}

func TestAllowlistOverridesTheBlocklist(t *testing.T) {
	allow := AllowlistOptions{Enabled: true, Contains: func(ip string) bool { return ip == "10.0.0.5" }}
	block := BlocklistOptions{Enabled: true, Contains: func(ip string) bool { return ip == "10.0.0.5" }}

	rec := httptest.NewRecorder()
	Chain(Allowlist(allow), Blocklist(block))(okHandler).ServeHTTP(rec, listRequest("10.0.0.5"))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: an explicitly allow-listed address must not be caught by a broad CIDR block", rec.Code, http.StatusOK)
	}
}

func TestAccessListsUseTheResolvedClientIP(t *testing.T) {
	req := listRequest("10.0.0.1")
	req.Header.Set(HeaderForwardedHost, "ignored.test")

	seen := ""
	opts := BlocklistOptions{
		Enabled:  true,
		ClientIP: func(*http.Request) string { return "203.0.113.9" },
		Contains: func(ip string) bool {
			seen = ip
			return false
		},
	}

	rec := httptest.NewRecorder()
	Blocklist(opts)(okHandler).ServeHTTP(rec, req)

	if seen != "203.0.113.9" {
		t.Errorf("matched against %q, want %q: behind a reverse proxy the TCP peer is the proxy, not the client", seen, "203.0.113.9")
	}
}
