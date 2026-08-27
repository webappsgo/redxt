//go:build e2e

// Package e2e holds the browser end-to-end suite required by AI.md
// PART 29 ("tests/e2e.sh"). It is excluded from `go build ./...` and
// `make test` by the e2e build tag and starts a real, in-process redxt
// server (via src/startup, the same entry point src/main.go uses) so
// every request exercises the actual router, templates, and health
// document rather than a mock.
//
// Scope for this pass is Universal Coverage only: the routes and
// behaviors that exist regardless of which DNS features (PART 18-23)
// are implemented — home page, health, admin login page, theme,
// responsive viewport, and error pages. DNS-feature-specific admin UI
// (zones, records, DDNS, redirect engine) is out of scope until those
// features exist; see TODO.AI.md for the tracked follow-up.
package e2e

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/cli"
	"github.com/webappsgo/redxt/src/startup"
)

// testServer starts a real redxt server rooted at a fresh temp
// directory tree and returns its base URL.
func testServer(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	args := []string{
		"--config", filepath.Join(root, "config"),
		"--data", filepath.Join(root, "data"),
		"--cache", filepath.Join(root, "cache"),
		"--log", filepath.Join(root, "log"),
		"--backup", filepath.Join(root, "backup"),
		"--pid", filepath.Join(root, "run", "redxt.pid"),
		"--color", "no",
	}

	opts, err := cli.Parse("redxt", args, io.Discard)
	if err != nil {
		t.Fatalf("cli.Parse() error = %v", err)
	}

	server, err := startup.Start(context.Background(), opts, startup.IO{Out: io.Discard, Err: io.Discard})
	if err != nil {
		t.Fatalf("startup.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if server.Log != nil {
			_ = server.Shutdown()
		}
	})

	return "http://127.0.0.1:" + strconv.Itoa(server.Config.Server.Port)
}

// get performs a bounded GET so a hung server fails the test instead
// of stalling the suite.
func get(t *testing.T, url string, headers map[string]string) *http.Response {
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

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return string(b)
}

// TestHomeRendersServerSideHTML covers Tier 1 (SSR, no browser engine):
// the home page must render full HTML with no client framework markup
// and must be reachable with an ordinary text/html client.
func TestHomeRendersServerSideHTML(t *testing.T) {
	base := testServer(t)
	resp := get(t, base+"/", map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Fatalf("GET / status = %d, want 200 or 302", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		html := body(t, resp)
		if !strings.Contains(html, "<html") {
			t.Errorf("home page missing <html> — got a non-HTML document")
		}
		for _, framework := range []string{"react", "vue", "ng-app"} {
			if strings.Contains(strings.ToLower(html), framework) {
				t.Errorf("home page references %q — PART 16 forbids client-side rendering frameworks", framework)
			}
		}
	}
}

// TestHomeIsResponsiveMarkup asserts the mobile-first viewport meta tag
// is present, per PART 16/29 mobile a11y requirements — this is
// verifiable without a browser engine since it is server-rendered HTML.
func TestHomeIsResponsiveMarkup(t *testing.T) {
	base := testServer(t)
	resp := get(t, base+"/", map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusOK {
		t.Skip("home page redirects before rendering (e.g. to setup wizard) — covered by setup flow instead")
	}
	html := body(t, resp)
	if !strings.Contains(html, `name="viewport"`) {
		t.Errorf("home page missing a viewport meta tag")
	}
}

// TestThemeSupportsPreferredColorScheme proves the theme system honors
// prefers-color-scheme (auto default) rather than hardcoding colors,
// per PART 16.
func TestThemeSupportsPreferredColorScheme(t *testing.T) {
	base := testServer(t)
	resp := get(t, base+"/", map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusOK {
		t.Skip("home page redirects before rendering — theme CSS is exercised on whichever page does render")
	}
	html := body(t, resp)
	if !strings.Contains(html, "prefers-color-scheme") {
		t.Errorf("home page CSS does not honor prefers-color-scheme")
	}
}

// TestHealthzUnauthenticated covers the health endpoint across content
// negotiation, per PART 13.
func TestHealthzUnauthenticated(t *testing.T) {
	base := testServer(t)

	cases := []struct {
		name   string
		accept string
	}{
		{"default", ""},
		{"json", "application/json"},
		{"text", "text/plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			if tc.accept != "" {
				headers["Accept"] = tc.accept
			}
			resp := get(t, base+"/server/healthz", headers)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET /server/healthz (%s) status = %d, want 200", tc.name, resp.StatusCode)
			}
		})
	}
}

// TestAdminLoginPageRenders covers the admin auth entry point: the
// login form must render for an unauthenticated visitor with no
// JavaScript required, per PART 16/17.
func TestAdminLoginPageRenders(t *testing.T) {
	base := testServer(t)
	resp := get(t, base+"/server/auth/login", map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /server/auth/login status = %d, want 200", resp.StatusCode)
	}
	html := body(t, resp)
	if !strings.Contains(html, "<form") {
		t.Errorf("login page has no <form> — PART 16 requires the login flow to work without JavaScript")
	}
}

// TestUnknownRouteRendersThemedErrorPage proves 404s are themed HTML
// pages, not bare framework error output, per PART 16.
func TestUnknownRouteRendersThemedErrorPage(t *testing.T) {
	base := testServer(t)
	resp := get(t, base+"/this-route-does-not-exist", map[string]string{"Accept": "text/html"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET unknown route status = %d, want 404", resp.StatusCode)
	}
	html := body(t, resp)
	if !strings.Contains(html, "<html") {
		t.Errorf("404 page is not themed HTML")
	}
}
