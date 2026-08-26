package startup

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/health"
)

// getURL performs a bounded GET so a hung listener fails the test
// instead of stalling the run.
func getURL(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s error = %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestStartBindsTheHTTPListener covers step 18: once Start returns, the
// listener is bound and the PART 13 health document is being served.
func TestStartBindsTheHTTPListener(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	urls := server.listenURLs()
	if len(urls) != 1 {
		t.Fatalf("listenURLs() = %v, want exactly the HTTP listener", urls)
	}
	if !strings.HasPrefix(urls[0], "http://") {
		t.Fatalf("listen URL = %q, want an http:// URL", urls[0])
	}

	resp := getURL(t, urls[0]+"/server/healthz", map[string]string{"Accept": "application/json"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var doc health.Response
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decoding the health document: %v", err)
	}
	if doc.Status != health.StatusHealthy {
		t.Errorf("status = %q, want %q", doc.Status, health.StatusHealthy)
	}
	if doc.Checks.Database != health.CheckOK {
		t.Errorf("checks.database = %q, want %q", doc.Checks.Database, health.CheckOK)
	}
}

// TestStartAdvertisesTheBoundPort proves the announced URL carries the
// port the kernel actually assigned rather than the configured value,
// which matters on the first run, where the port is chosen at random.
func TestStartAdvertisesTheBoundPort(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	want := ":" + strconv.Itoa(server.Config.Server.Port)
	if got := server.listenURLs()[0]; !strings.HasSuffix(got, want) {
		t.Errorf("listen URL = %q, want it to end in %q", got, want)
	}
}

// TestShutdownClosesTheListener covers the teardown half: the port must
// be released before the databases behind it close.
func TestShutdownClosesTheListener(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))
	url := server.listenURLs()[0] + "/server/healthz"

	if err := server.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if server.HTTP != nil {
		t.Error("HTTP server is still held after shutdown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatalf("the listener still answered after shutdown with status %d", resp.StatusCode)
	}
}

// TestHealthProbeReportsTheDatabases covers the probe's one live check.
func TestHealthProbeReportsTheDatabases(t *testing.T) {
	tests := []struct {
		name  string
		close bool
		want  string
	}{
		{name: "both stores open", close: false, want: health.CheckOK},
		{name: "a closed store fails the check", close: true, want: health.CheckError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := startForTest(t, testArgs(t))
			if tt.close {
				if err := server.UsersDB.Close(); err != nil {
					t.Fatalf("closing the users database: %v", err)
				}
			}

			snap := server.healthProbe(context.Background())
			if snap.Checks.Database != tt.want {
				t.Errorf("checks.database = %q, want %q", snap.Checks.Database, tt.want)
			}
		})
	}
}

// TestHealthProbeReportsShutdown proves the document flips to the
// shutting-down state while connections drain.
func TestHealthProbeReportsShutdown(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	if snap := server.healthProbe(context.Background()); snap.State.ShuttingDown {
		t.Error("a running server reports itself as shutting down")
	}

	server.shuttingDown = true
	if snap := server.healthProbe(context.Background()); !snap.State.ShuttingDown {
		t.Error("a draining server does not report itself as shutting down")
	}
}

// TestListenURLsFallsBackToTheConfiguration covers the case where the
// banner is drawn without a bound listener, which happens only if step
// 18 was skipped: the configured address is still reported rather than
// nothing at all.
func TestListenURLsFallsBackToTheConfiguration(t *testing.T) {
	server, _ := startForTest(t, testArgs(t, "--address", "127.0.0.1"))
	server.HTTP = nil

	want := "http://127.0.0.1:" + strconv.Itoa(server.Config.Server.Port)
	got := server.listenURLs()
	if len(got) != 1 || got[0] != want {
		t.Errorf("listenURLs() = %v, want [%s]", got, want)
	}
}

// TestTLSProviderIsNilWithoutAManager guards the interface-nil trap: a
// nil manager must not be handed to the server as a non-nil interface.
func TestTLSProviderIsNilWithoutAManager(t *testing.T) {
	server := &Server{}

	if server.tlsProvider() != nil {
		t.Error("tlsProvider() returned a non-nil interface with no manager")
	}
}
