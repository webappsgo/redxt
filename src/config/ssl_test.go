package config

import (
	"strings"
	"testing"
)

func TestIsRouteSegment(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"v1", true},
		{"administration", true},
		{"admin-panel", true},
		{"v2beta9", true},
		{"", false},
		{"V1", false},
		{"admin_path", false},
		{"admin/panel", false},
		{"admin.panel", false},
		{"-admin", false},
		{"admin-", false},
		{"admin panel", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := IsRouteSegment(tt.in); got != tt.want {
				t.Fatalf("IsRouteSegment(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestAPIBasePathAndAdminBasePath(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		adminPath  string
		wantAPI    string
		wantAdmin  string
	}{
		{name: "defaults", apiVersion: "v1", adminPath: "administration", wantAPI: "/api/v1", wantAdmin: "/server/administration"},
		{name: "custom", apiVersion: "v2", adminPath: "control", wantAPI: "/api/v2", wantAdmin: "/server/control"},
		{name: "unset falls back", apiVersion: "", adminPath: "", wantAPI: "/api/v1", wantAdmin: "/server/administration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.Server.APIVersion = tt.apiVersion
			c.Server.AdminPath = tt.adminPath
			if got := c.APIBasePath(); got != tt.wantAPI {
				t.Fatalf("APIBasePath() = %q, want %q", got, tt.wantAPI)
			}
			if got := c.AdminBasePath(); got != tt.wantAdmin {
				t.Fatalf("AdminBasePath() = %q, want %q", got, tt.wantAdmin)
			}
		})
	}
}

func TestListenerPorts(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		sslPort   int
		sslOn     bool
		wantHTTP  int
		wantHTTPS int
	}{
		{name: "plain http", port: 8090, wantHTTP: 8090},
		{name: "port 443 is https only", port: 443, wantHTTPS: 443},
		{name: "ssl enabled forces https", port: 8090, sslOn: true, wantHTTPS: 8090},
		{name: "dual pair", port: 80, sslPort: 443, sslOn: true, wantHTTP: 80, wantHTTPS: 443},
		{name: "dual pair without ssl enabled", port: 8090, sslPort: 8443, wantHTTP: 8090, wantHTTPS: 8443},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.Server.Port = tt.port
			c.Server.SSL.Port = tt.sslPort
			c.Server.SSL.Enabled = tt.sslOn
			if got := c.HTTPPort(); got != tt.wantHTTP {
				t.Fatalf("HTTPPort() = %d, want %d", got, tt.wantHTTP)
			}
			if got := c.HTTPSPort(); got != tt.wantHTTPS {
				t.Fatalf("HTTPSPort() = %d, want %d", got, tt.wantHTTPS)
			}
		})
	}
}

func TestValidateSSL(t *testing.T) {
	def := DefaultConfig()

	tests := []struct {
		name  string
		in    SSL
		port  int
		check func(t *testing.T, got SSL, warnings []string)
	}{
		{
			name: "defaults are accepted unchanged",
			in:   def.Server.SSL,
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if len(warnings) != 0 {
					t.Fatalf("unexpected warnings: %v", warnings)
				}
				if got.MinVersion != "TLS1.2" || got.LetsEncrypt.Challenge != "http-01" {
					t.Fatalf("defaults changed: %+v", got)
				}
			},
		},
		{
			name: "min version is normalized to upper case",
			in:   SSL{MinVersion: "tls1.3", LetsEncrypt: LetsEncrypt{Challenge: "dns-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.MinVersion != "TLS1.3" {
					t.Fatalf("MinVersion = %q, want TLS1.3", got.MinVersion)
				}
				if len(warnings) != 0 {
					t.Fatalf("unexpected warnings: %v", warnings)
				}
			},
		},
		{
			name: "unknown min version falls back",
			in:   SSL{MinVersion: "SSLv3", LetsEncrypt: LetsEncrypt{Challenge: "http-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.MinVersion != "TLS1.2" {
					t.Fatalf("MinVersion = %q, want TLS1.2", got.MinVersion)
				}
				if len(warnings) != 1 || !strings.Contains(warnings[0], "min_version") {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "unknown challenge falls back",
			in:   SSL{MinVersion: "TLS1.2", LetsEncrypt: LetsEncrypt{Challenge: "email-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.LetsEncrypt.Challenge != "http-01" {
					t.Fatalf("Challenge = %q, want http-01", got.LetsEncrypt.Challenge)
				}
				if len(warnings) != 1 || !strings.Contains(warnings[0], "challenge") {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "half a manual override is discarded",
			in:   SSL{MinVersion: "TLS1.2", Cert: "/tmp/cert.pem", LetsEncrypt: LetsEncrypt{Challenge: "http-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.Cert != "" || got.Key != "" {
					t.Fatalf("Cert/Key = %q/%q, want both empty", got.Cert, got.Key)
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "letsencrypt without an email is disabled",
			in:   SSL{MinVersion: "TLS1.2", LetsEncrypt: LetsEncrypt{Enabled: true, Challenge: "http-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.LetsEncrypt.Enabled {
					t.Fatal("LetsEncrypt.Enabled = true, want false")
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "https port duplicating the http port is dropped",
			in:   SSL{MinVersion: "TLS1.2", Port: 8090, LetsEncrypt: LetsEncrypt{Challenge: "http-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.Port != 0 {
					t.Fatalf("Port = %d, want 0", got.Port)
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "out of range https port is dropped",
			in:   SSL{MinVersion: "TLS1.2", Port: 70000, LetsEncrypt: LetsEncrypt{Challenge: "http-01"}},
			port: 8090,
			check: func(t *testing.T, got SSL, warnings []string) {
				if got.Port != 0 {
					t.Fatalf("Port = %d, want 0", got.Port)
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.Server.Port = tt.port
			c.Server.SSL = tt.in
			c.validateSSL(def)
			tt.check(t, c.Server.SSL, c.warnings)
		})
	}
}

func TestValidateWeb(t *testing.T) {
	def := DefaultConfig()

	tests := []struct {
		name  string
		in    Web
		check func(t *testing.T, got Web, warnings []string)
	}{
		{
			name: "defaults are accepted unchanged",
			in:   def.Server.Web,
			check: func(t *testing.T, got Web, warnings []string) {
				if len(warnings) != 0 {
					t.Fatalf("unexpected warnings: %v", warnings)
				}
				if !got.HSTS.Preload || got.HSTS.MaxAge != DefaultHSTSMaxAge {
					t.Fatalf("HSTS defaults changed: %+v", got.HSTS)
				}
			},
		},
		{
			name: "nil exempt paths fall back to the defaults",
			in:   Web{CORS: "*", CSRF: CSRF{Enabled: true}},
			check: func(t *testing.T, got Web, warnings []string) {
				if len(got.CSRF.ExemptPaths) != len(DefaultCSRFExemptPaths) {
					t.Fatalf("ExemptPaths = %v, want %v", got.CSRF.ExemptPaths, DefaultCSRFExemptPaths)
				}
			},
		},
		{
			name: "relative exempt paths are rejected",
			in:   Web{CORS: "*", CSRF: CSRF{Enabled: true, ExemptPaths: []string{"/api/", "webhooks", "  "}}},
			check: func(t *testing.T, got Web, warnings []string) {
				if len(got.CSRF.ExemptPaths) != 1 || got.CSRF.ExemptPaths[0] != "/api/" {
					t.Fatalf("ExemptPaths = %v, want [/api/]", got.CSRF.ExemptPaths)
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "preload without include_subdomains is dropped",
			in: Web{
				CORS: "*",
				CSRF: CSRF{Enabled: true, ExemptPaths: []string{}},
				HSTS: HSTS{Enabled: true, MaxAge: DefaultHSTSMaxAge, Preload: true},
			},
			check: func(t *testing.T, got Web, warnings []string) {
				if got.HSTS.Preload {
					t.Fatal("Preload = true, want false")
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
			},
		},
		{
			name: "preload with too short a max age is dropped",
			in: Web{
				CORS: "*",
				CSRF: CSRF{Enabled: true, ExemptPaths: []string{}},
				HSTS: HSTS{Enabled: true, MaxAge: 3600, IncludeSubdomains: true, Preload: true},
			},
			check: func(t *testing.T, got Web, warnings []string) {
				if got.HSTS.Preload {
					t.Fatal("Preload = true, want false")
				}
			},
		},
		{
			name: "negative max age falls back",
			in: Web{
				CORS: "*",
				CSRF: CSRF{Enabled: true, ExemptPaths: []string{}},
				HSTS: HSTS{Enabled: true, MaxAge: -1},
			},
			check: func(t *testing.T, got Web, warnings []string) {
				if got.HSTS.MaxAge != DefaultHSTSMaxAge {
					t.Fatalf("MaxAge = %d, want %d", got.HSTS.MaxAge, DefaultHSTSMaxAge)
				}
			},
		},
		{
			name: "cors value is trimmed",
			in:   Web{CORS: "  https://example.com  ", CSRF: CSRF{Enabled: true, ExemptPaths: []string{}}},
			check: func(t *testing.T, got Web, warnings []string) {
				if got.CORS != "https://example.com" {
					t.Fatalf("CORS = %q, want %q", got.CORS, "https://example.com")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.Server.Web = tt.in
			c.validateWeb(def)
			tt.check(t, c.Server.Web, c.warnings)
		})
	}
}

func TestValidateServerRouteSegments(t *testing.T) {
	def := DefaultConfig()

	tests := []struct {
		name          string
		adminPath     string
		apiVersion    string
		wantAdmin     string
		wantAPI       string
		wantWarnCount int
	}{
		{name: "valid", adminPath: "control", apiVersion: "v2", wantAdmin: "control", wantAPI: "v2"},
		{name: "empty is silently defaulted", adminPath: "", apiVersion: "", wantAdmin: DefaultAdminPath, wantAPI: DefaultAPIVersion},
		{name: "invalid warns", adminPath: "Admin_Panel", apiVersion: "V1", wantAdmin: DefaultAdminPath, wantAPI: DefaultAPIVersion, wantWarnCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.Server = def.Server
			// A concrete port keeps validateServer from emitting the
			// unrelated random-port warning this table counts on.
			c.Server.Port = 8090
			c.Server.AdminPath = tt.adminPath
			c.Server.APIVersion = tt.apiVersion
			c.validateServer(def)
			if c.Server.AdminPath != tt.wantAdmin {
				t.Fatalf("AdminPath = %q, want %q", c.Server.AdminPath, tt.wantAdmin)
			}
			if c.Server.APIVersion != tt.wantAPI {
				t.Fatalf("APIVersion = %q, want %q", c.Server.APIVersion, tt.wantAPI)
			}
			if len(c.warnings) != tt.wantWarnCount {
				t.Fatalf("warnings = %v, want %d", c.warnings, tt.wantWarnCount)
			}
		})
	}
}
