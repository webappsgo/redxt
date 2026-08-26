package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// requestWithUA builds a GET request for the given path carrying the given
// User-Agent. An empty agent leaves the header unset.
func requestWithUA(path, agent string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if agent != "" {
		r.Header.Set("User-Agent", agent)
	}
	return r
}

func TestClientDetection(t *testing.T) {
	tests := []struct {
		agent          string
		ourCLI         bool
		textBrowser    bool
		httpTool       bool
		nonInteractive bool
		browser        bool
		reason         string
	}{
		{agent: "redxt-cli/1.0.0", ourCLI: true, reason: "our own client binary"},
		{agent: "REDXT-CLI/1.0.0 (linux/amd64)", ourCLI: true, reason: "our client, case insensitive"},
		{agent: "Lynx/2.8.9rel.1 libwww-FM/2.14", textBrowser: true, browser: true, reason: "lynx text browser"},
		{agent: "w3m/0.5.3+git20230121", textBrowser: true, browser: true, reason: "w3m text browser"},
		{agent: "Links (2.29; Linux 6.1 x86_64; GNU C 12.2)", textBrowser: true, browser: true, reason: "links, space separated"},
		{agent: "Links/2.29", textBrowser: true, browser: true, reason: "links, slash separated"},
		{agent: "ELinks/0.13.2", textBrowser: true, browser: true, reason: "elinks text browser"},
		{agent: "Browsh/1.6.4", textBrowser: true, browser: true, reason: "browsh text browser"},
		{agent: "Carbonyl/0.0.3", textBrowser: true, browser: true, reason: "carbonyl text browser"},
		{agent: "NetSurf/3.10 (Linux)", textBrowser: true, browser: true, reason: "netsurf, no slash required"},
		{agent: "curl/8.5.0", httpTool: true, nonInteractive: true, reason: "curl http tool"},
		{agent: "Wget/1.21.4", httpTool: true, nonInteractive: true, reason: "wget http tool"},
		{agent: "HTTPie/3.2.2", httpTool: true, nonInteractive: true, reason: "httpie http tool"},
		{agent: "libcurl/8.5.0 nghttp2/1.58.0", httpTool: true, nonInteractive: true, reason: "libcurl http tool"},
		{agent: "python-requests/2.31.0", httpTool: true, nonInteractive: true, reason: "python requests"},
		{agent: "Go-http-client/2.0", httpTool: true, nonInteractive: true, reason: "go http client"},
		{agent: "axios/1.6.2", httpTool: true, nonInteractive: true, reason: "axios"},
		{agent: "node-fetch/3.3.2", httpTool: true, nonInteractive: true, reason: "node-fetch"},
		{agent: "", httpTool: true, nonInteractive: true, reason: "empty user-agent is an http tool"},
		{agent: "Mozilla/5.0 (X11; Linux x86_64) Firefox/121.0", browser: true, reason: "graphical browser"},
		{agent: "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36", browser: true, reason: "graphical browser"},
		{agent: "SomeUnknownAgent/1.0", reason: "unknown agent matches nothing"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			r := requestWithUA("/", tc.agent)

			if got := IsOurCLIClient(r); got != tc.ourCLI {
				t.Errorf("IsOurCLIClient(%q) = %v, want %v", tc.agent, got, tc.ourCLI)
			}
			if got := IsTextBrowser(r); got != tc.textBrowser {
				t.Errorf("IsTextBrowser(%q) = %v, want %v", tc.agent, got, tc.textBrowser)
			}
			if got := IsHTTPTool(r); got != tc.httpTool {
				t.Errorf("IsHTTPTool(%q) = %v, want %v", tc.agent, got, tc.httpTool)
			}
			if got := IsNonInteractiveClient(r); got != tc.nonInteractive {
				t.Errorf("IsNonInteractiveClient(%q) = %v, want %v", tc.agent, got, tc.nonInteractive)
			}
			if got := IsBrowser(r); got != tc.browser {
				t.Errorf("IsBrowser(%q) = %v, want %v", tc.agent, got, tc.browser)
			}
		})
	}
}

func TestNonInteractiveExcludesInteractiveClients(t *testing.T) {
	// A text browser announcing a curl-like fragment stays interactive:
	// the interactive checks run first, per AI.md PART 14.
	r := requestWithUA("/", "Lynx/2.8.9 curl/8.5.0")
	if !IsTextBrowser(r) {
		t.Error("a lynx user-agent must be detected as a text browser")
	}
	if IsNonInteractiveClient(r) {
		t.Error("text browsers are interactive and must never be non-interactive")
	}

	// The same holds for our own client.
	r = requestWithUA("/", "redxt-cli/1.0.0 curl/8.5.0")
	if IsNonInteractiveClient(r) {
		t.Error("our own client is interactive and must never be non-interactive")
	}
}

func TestHTTPToolUserAgentIsNotABrowser(t *testing.T) {
	// curl -A can pretend to be Mozilla; the tool fragment still wins so
	// the client is not served an interactive HTML page.
	r := requestWithUA("/", "Mozilla/5.0 curl/8.5.0")
	if IsBrowser(r) {
		t.Error("an http tool user-agent must not be classified as a browser")
	}
}

func TestDetectionWithNilRequest(t *testing.T) {
	if IsOurCLIClient(nil) {
		t.Error("IsOurCLIClient(nil) must be false")
	}
	if IsTextBrowser(nil) {
		t.Error("IsTextBrowser(nil) must be false")
	}
	if !IsHTTPTool(nil) {
		t.Error("IsHTTPTool(nil) must be true, an absent user-agent is a tool")
	}
	if !IsNonInteractiveClient(nil) {
		t.Error("IsNonInteractiveClient(nil) must be true")
	}
	if IsBrowser(nil) {
		t.Error("IsBrowser(nil) must be false")
	}
}
