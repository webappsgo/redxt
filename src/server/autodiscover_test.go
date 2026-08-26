package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		headers map[string]string
		trusted bool
		want    string
	}{
		{
			name: "plain host",
			host: "example.com",
			want: "http://example.com",
		},
		{
			name: "custom port is kept",
			host: "example.com:8080",
			want: "http://example.com:8080",
		},
		{
			name: "default http port is stripped",
			host: "example.com:80",
			want: "http://example.com",
		},
		{
			name:    "trusted proxy sets scheme and host",
			host:    "internal:8080",
			headers: map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "public.example.com"},
			trusted: true,
			want:    "https://public.example.com",
		},
		{
			name:    "trusted proxy strips the default https port",
			host:    "internal:8080",
			headers: map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "public.example.com:443"},
			trusted: true,
			want:    "https://public.example.com",
		},
		{
			name:    "first value of a proxy chain wins",
			host:    "internal:8080",
			headers: map[string]string{"X-Forwarded-Host": "outer.example.com, inner.example.com"},
			trusted: true,
			want:    "http://outer.example.com",
		},
		{
			name:    "untrusted forwarded headers are ignored",
			host:    "example.com",
			headers: map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "attacker.example.net"},
			trusted: false,
			want:    "http://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			if got := RequestBaseURL(req, tt.trusted); got != tt.want {
				t.Errorf("RequestBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildAutodiscovery(t *testing.T) {
	o := testOptions(t)
	req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	req.Host = "dns.example.com"

	doc := BuildAutodiscovery(o, req)

	if doc.Primary != "http://dns.example.com" {
		t.Errorf("primary = %q, want %q", doc.Primary, "http://dns.example.com")
	}
	if doc.APIVersion != "v1" {
		t.Errorf("api_version = %q, want %q", doc.APIVersion, "v1")
	}
	if doc.Cluster == nil {
		t.Error("cluster is nil, want an empty array so the JSON never omits the field")
	}
	if doc.Timeout != 30 {
		t.Errorf("timeout = %d, want 30", doc.Timeout)
	}
	if doc.Retry < 1 {
		t.Errorf("retry = %d, want at least 1", doc.Retry)
	}
	if doc.RetryDelay < 1 {
		t.Errorf("retry_delay = %d, want at least 1", doc.RetryDelay)
	}
	if doc.Config.Database.Aliases["mariadb"] != "mysql" {
		t.Errorf("database alias mariadb = %q, want %q", doc.Config.Database.Aliases["mariadb"], "mysql")
	}
	if doc.Config.Features["tor"] != false {
		t.Errorf("config.features[tor] = %v, want false with no onion address set", doc.Config.Features["tor"])
	}
}

func TestBuildAutodiscoveryUsesTheSuppliedProviders(t *testing.T) {
	o := testOptions(t)
	o.Cluster = func() []string {
		return []string{"https://a.example.com", "https://b.example.com"}
	}
	o.Features = func() map[string]any {
		return map[string]any{"clustering": true}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	doc := BuildAutodiscovery(o, req)

	if len(doc.Cluster) != 2 {
		t.Errorf("cluster = %v, want two entries", doc.Cluster)
	}
	if doc.Config.Features["clustering"] != true {
		t.Errorf("config.features[clustering] = %v, want true", doc.Config.Features["clustering"])
	}
	if _, ok := doc.Config.Features["tor"]; !ok {
		t.Error("provider features replaced the built-in tor flag, want them merged")
	}
}

func TestAutodiscoverNeverPublishesTheAdminPath(t *testing.T) {
	// AI.md PART 14 is explicit: the admin path's obscurity is the
	// point, so it must never appear in this document.
	o := testOptions(t)
	o.Config.Server.AdminPath = "secret-control-room"

	rec := do(t, o, http.MethodGet, "/api/autodiscover", nil)
	body := rec.Body.String()

	if strings.Contains(body, "secret-control-room") {
		t.Errorf("autodiscover response leaks the admin path:\n%s", body)
	}
	if strings.Contains(body, "admin_path") {
		t.Errorf("autodiscover response contains an admin_path field:\n%s", body)
	}
}

func TestAutodiscoverResponseFormats(t *testing.T) {
	o := testOptions(t)

	t.Run("json by default", func(t *testing.T) {
		rec := do(t, o, http.MethodGet, "/api/autodiscover", map[string]string{"User-Agent": "Mozilla/5.0"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json; charset=utf-8")
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=3600" {
			t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=3600")
		}

		var doc Autodiscovery
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if doc.APIVersion != "v1" {
			t.Errorf("api_version = %q, want %q", doc.APIVersion, "v1")
		}
	})

	t.Run("text on request", func(t *testing.T) {
		rec := do(t, o, http.MethodGet, "/api/autodiscover", map[string]string{"Accept": "text/plain"})
		body := rec.Body.String()

		if !strings.Contains(body, "api_version: v1") {
			t.Errorf("text body missing api_version line:\n%s", body)
		}
		if !strings.HasSuffix(body, "\n") {
			t.Error("text body does not end in a newline")
		}
	})
}
