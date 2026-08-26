package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/health"
)

func TestRouterRootLevelEndpoints(t *testing.T) {
	o := testOptions(t)
	o.Routes = Routes{
		SwaggerUI:   stubHandler("swagger-ui"),
		SwaggerSpec: stubHandler("swagger-spec"),
		GraphQLUI:   stubHandler("graphql-ui"),
		GraphQLAPI:  stubHandler("graphql-api"),
		Metrics:     stubHandler("metrics"),
		Admin:       stubHandler("admin"),
		AdminAPI:    stubHandler("admin-api"),
	}

	tests := []struct {
		name   string
		method string
		target string
		want   string
	}{
		{"swagger ui", http.MethodGet, "/server/docs/swagger", "swagger-ui"},
		{"graphql ui", http.MethodGet, "/server/docs/graphql", "graphql-ui"},
		{"swagger spec alias", http.MethodGet, "/api/swagger", "swagger-spec"},
		{"swagger spec versioned", http.MethodGet, "/api/v1/server/swagger", "swagger-spec"},
		{"graphql alias", http.MethodPost, "/api/graphql", "graphql-api"},
		{"graphql versioned", http.MethodPost, "/api/v1/server/graphql", "graphql-api"},
		{"metrics frontend", http.MethodGet, "/server/metrics", "metrics"},
		{"metrics per service", http.MethodGet, "/server/metrics/loki", "metrics"},
		{"metrics root alias", http.MethodGet, "/metrics", "metrics"},
		{"metrics api alias", http.MethodGet, "/api/metrics", "metrics"},
		{"metrics versioned", http.MethodGet, "/api/v1/server/metrics", "metrics"},
		{"admin panel", http.MethodGet, "/server/administration", "admin"},
		{"admin panel page", http.MethodGet, "/server/administration/users", "admin"},
		{"admin api", http.MethodGet, "/api/v1/server/administration/users", "admin-api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, o, tt.method, tt.target, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.target, rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("%s %s body = %q, want %q", tt.method, tt.target, got, tt.want)
			}
		})
	}
}

func TestRouterAliasesAreNotRedirects(t *testing.T) {
	o := testOptions(t)
	o.Routes = Routes{
		SwaggerSpec: stubHandler("swagger-spec"),
		GraphQLAPI:  stubHandler("graphql-api"),
	}

	// AI.md PART 14 forbids implementing an unversioned alias as a
	// redirect to the versioned path; both must answer directly.
	tests := []struct {
		name   string
		method string
		target string
	}{
		{"swagger", http.MethodGet, "/api/swagger"},
		{"graphql", http.MethodPost, "/api/graphql"},
		{"healthz", http.MethodGet, "/api/healthz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, o, tt.method, tt.target, nil)
			if rec.Code >= 300 && rec.Code < 400 {
				t.Fatalf("%s returned redirect status %d", tt.target, rec.Code)
			}
			if location := rec.Header().Get("Location"); location != "" {
				t.Errorf("%s set Location = %q, want no redirect", tt.target, location)
			}
		})
	}
}

func TestRouterUnwiredSurfacesAreNotMounted(t *testing.T) {
	// A PART that is not wired in yet must not answer with an empty
	// success; the route simply does not exist.
	o := testOptions(t)

	targets := []string{
		"/server/docs/swagger",
		"/server/docs/graphql",
		"/api/swagger",
		"/metrics",
		"/server/administration",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			rec := do(t, o, http.MethodGet, target, nil)
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s status = %d, want %d", target, rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestRouterRootHealthzAlias(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    int
	}{
		{"disabled by default", false, http.StatusNotFound},
		{"enabled by config", true, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t)
			o.Config.Server.Healthz.Root.Enabled = tt.enabled

			rec := do(t, o, http.MethodGet, "/healthz", nil)
			if rec.Code != tt.want {
				t.Errorf("GET /healthz status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRouterHealthNegotiation(t *testing.T) {
	o := testOptions(t)

	tests := []struct {
		name        string
		target      string
		headers     map[string]string
		wantType    string
		wantContain string
	}{
		{
			name:        "frontend defaults to html",
			target:      "/server/healthz",
			headers:     map[string]string{"User-Agent": "Mozilla/5.0"},
			wantType:    "text/html; charset=utf-8",
			wantContain: "<table>",
		},
		{
			name:        "frontend honors accept json",
			target:      "/server/healthz",
			headers:     map[string]string{"Accept": "application/json"},
			wantType:    "application/json; charset=utf-8",
			wantContain: `"status"`,
		},
		{
			name:        "frontend honors accept text",
			target:      "/server/healthz",
			headers:     map[string]string{"Accept": "text/plain"},
			wantType:    "text/plain; charset=utf-8",
			wantContain: "project.name:",
		},
		{
			name:        "api defaults to json",
			target:      "/api/healthz",
			headers:     map[string]string{"User-Agent": "Mozilla/5.0"},
			wantType:    "application/json; charset=utf-8",
			wantContain: `"status"`,
		},
		{
			name:        "our cli always gets json",
			target:      "/server/healthz",
			headers:     map[string]string{"User-Agent": "redxt-cli/1.0.0"},
			wantType:    "application/json; charset=utf-8",
			wantContain: `"status"`,
		},
		{
			name:        "curl gets text from an api route",
			target:      "/api/v1/server/healthz",
			headers:     map[string]string{"User-Agent": "curl/8.5.0"},
			wantType:    "text/plain; charset=utf-8",
			wantContain: "project.name:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, o, http.MethodGet, tt.target, tt.headers)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
			if !strings.Contains(rec.Body.String(), tt.wantContain) {
				t.Errorf("body does not contain %q:\n%s", tt.wantContain, rec.Body.String())
			}
		})
	}
}

func TestRouterHealthJSONIsIndentedAndNewlineTerminated(t *testing.T) {
	o := testOptions(t)

	rec := do(t, o, http.MethodGet, "/api/healthz", map[string]string{"Accept": "application/json"})
	body := rec.Body.String()

	if !strings.HasSuffix(body, "\n") {
		t.Error("JSON body does not end in a newline")
	}
	if !strings.Contains(body, "\n  \"") {
		t.Error("JSON body is not indented with two spaces")
	}

	var doc health.Response
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if doc.Status != health.StatusHealthy {
		t.Errorf("status = %q, want %q", doc.Status, health.StatusHealthy)
	}
}

func TestRouterUnhealthyStatusCode(t *testing.T) {
	o := testOptions(t)
	o.HealthProbe = StaticHealthProbe(HealthSnapshot{
		Checks: health.ChecksInfo{
			Database:    health.CheckError,
			Cache:       health.CheckOK,
			Disk:        health.CheckOK,
			Scheduler:   health.CheckOK,
			DNSListener: health.CheckOK,
			Zones:       health.CheckOK,
		},
	})

	rec := do(t, o, http.MethodGet, "/api/healthz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRouterNotFoundUsesTheRequestedFormat(t *testing.T) {
	o := testOptions(t)

	tests := []struct {
		name     string
		target   string
		headers  map[string]string
		wantType string
	}{
		{"api route answers json", "/api/v1/nope", map[string]string{"Accept": "application/json"}, "application/json; charset=utf-8"},
		{"frontend answers html", "/nope", map[string]string{"User-Agent": "Mozilla/5.0"}, "text/html; charset=utf-8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, o, http.MethodGet, tt.target, tt.headers)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestRouterHonorsConfiguredSegments(t *testing.T) {
	o := testOptions(t)
	o.Config.Server.APIVersion = "v2"
	o.Config.Server.AdminPath = "control-room"
	o.Routes = Routes{
		SwaggerSpec: stubHandler("swagger-spec"),
		Admin:       stubHandler("admin"),
	}

	tests := []struct {
		name   string
		target string
		want   int
	}{
		{"configured api version", "/api/v2/server/swagger", http.StatusOK},
		{"old api version is gone", "/api/v1/server/swagger", http.StatusNotFound},
		{"configured admin path", "/server/control-room", http.StatusOK},
		{"default admin path is gone", "/server/administration", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, o, http.MethodGet, tt.target, nil)
			if rec.Code != tt.want {
				t.Errorf("GET %s status = %d, want %d", tt.target, rec.Code, tt.want)
			}
		})
	}
}

// stubHandler returns a handler that writes a fixed body, so a route
// test can prove which handler a pattern reached.
func stubHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}
