package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// testLogger records nothing; the tests assert on responses, not logs.
type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Warnf(string, ...any)  {}
func (testLogger) Errorf(string, ...any) {}

// testConfig returns a validated configuration with a concrete port so
// no test depends on the random first-run port selection.
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Server.Port = 8080
	cfg.Server.Listen = "127.0.0.1"
	return cfg
}

// testOptions returns Options wired for an in-process handler test.
func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Config:      testConfig(t),
		Log:         testLogger{},
		Mode:        "production",
		Started:     time.Now().Add(-time.Hour),
		HealthProbe: StaticHealthProbe(HealthSnapshot{Checks: okChecks()}),
	}
}

func TestDisplayHost(t *testing.T) {
	tests := []struct {
		name   string
		listen string
		want   string
	}{
		{"a concrete address is used as given", "127.0.0.1", "127.0.0.1"},
		{"a hostname is used as given", "dns.example.com", "dns.example.com"},
		{"an empty address becomes localhost", "", "localhost"},
		{"the ipv4 wildcard becomes localhost", "0.0.0.0", "localhost"},
		{"the ipv6 wildcard becomes localhost", "::", "localhost"},
		{"an ipv6 literal is bracketed", "::1", "[::1]"},
		{"an already bracketed literal is left alone", "[fe80::1]", "[fe80::1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayHost(tt.listen); got != tt.want {
				t.Errorf("displayHost(%q) = %q, want %q", tt.listen, got, tt.want)
			}
		})
	}
}

func TestNewRequiresConfigAndLogger(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"no config", Options{Log: testLogger{}}},
		{"no logger", Options{Config: testConfig(t)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Fatal("New() = nil error, want an error")
			}
		})
	}
}

func TestNewRejectsAConfigWithNoListener(t *testing.T) {
	o := testOptions(t)
	// Port 443 with no TLS provider leaves the HTTPS listener the only
	// candidate, and it cannot be built without certificates.
	o.Config.Server.Port = 443

	if _, err := New(o); err == nil {
		t.Fatal("New() = nil error, want an error for a server with no listener")
	}
}

func TestStartAndShutdownServeRequests(t *testing.T) {
	o := testOptions(t)
	// The same selector the first-run configuration uses picks a port
	// that is free right now, so the test never collides with a port
	// another process already holds.
	o.Config.Server.Port = config.RandomUnusedPort()

	srv, err := New(o)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	urls := srv.URLs()
	if len(urls) != 1 {
		t.Fatalf("URLs() = %v, want exactly one URL", urls)
	}
	if !strings.HasPrefix(urls[0], "http://127.0.0.1:") {
		t.Errorf("URLs()[0] = %q, want an http://127.0.0.1 prefix", urls[0])
	}

	resp, err := http.Get(urls[0] + "/server/healthz")
	if err != nil {
		t.Fatalf("GET /server/healthz error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /server/healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	select {
	case err := <-srv.Err():
		t.Fatalf("Err() reported %v, want no error after a graceful shutdown", err)
	default:
	}
}

// do runs one request against a freshly built router.
func do(t *testing.T, o Options, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	NewRouter(o).ServeHTTP(rec, req)
	return rec
}
