package httputil

import (
	"net/http"
	"testing"
)

// negotiationRequest builds a GET request with the given path, Accept
// header and User-Agent. An empty Accept leaves the header unset.
func negotiationRequest(path, accept, agent string) *http.Request {
	r := requestWithUA(path, agent)
	if accept != "" {
		r.Header.Set("Accept", accept)
	}
	return r
}

func TestNegotiateAPI(t *testing.T) {
	tests := []struct {
		path   string
		accept string
		agent  string
		want   Format
		reason string
	}{
		{path: "/api/v1/joke.txt", accept: "application/json", agent: "Mozilla/5.0 Firefox/121.0", want: FormatText, reason: "priority 1 .txt beats Accept json"},
		{path: "/api/v1/joke.txt", accept: "", agent: "", want: FormatText, reason: "priority 1 .txt alone"},
		{path: "/api/v1/joke", accept: "application/json", agent: "curl/8.5.0", want: FormatJSON, reason: "priority 2 Accept json beats curl"},
		{path: "/api/v1/joke", accept: "text/plain;q=0.2, application/json;q=0.9", agent: "curl/8.5.0", want: FormatJSON, reason: "priority 2 wins over 3 regardless of q values"},
		{path: "/api/v1/joke", accept: "text/plain", agent: "Mozilla/5.0 Firefox/121.0", want: FormatText, reason: "priority 3 Accept text/plain"},
		{path: "/api/v1/joke", accept: "", agent: "curl/8.5.0", want: FormatText, reason: "priority 4 non-interactive client"},
		{path: "/api/v1/joke", accept: "*/*", agent: "Wget/1.21.4", want: FormatText, reason: "priority 4 wildcard accept, non-interactive"},
		{path: "/api/v1/joke", accept: "", agent: "", want: FormatText, reason: "priority 4 empty user-agent"},
		{path: "/api/v1/joke", accept: "", agent: "Mozilla/5.0 Firefox/121.0", want: FormatJSON, reason: "priority 5 default for a browser"},
		{path: "/api/v1/joke", accept: "text/html", agent: "Mozilla/5.0 Firefox/121.0", want: FormatJSON, reason: "priority 5 default, html is not an API format"},
		{path: "/api/v1/joke", accept: "", agent: "redxt-cli/1.0.0", want: FormatJSON, reason: "priority 5 our client is interactive and gets JSON"},
		{path: "/api/v1/joke", accept: "", agent: "Lynx/2.8.9rel.1", want: FormatJSON, reason: "priority 5 text browsers are interactive"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			got := NegotiateAPI(negotiationRequest(tc.path, tc.accept, tc.agent))
			if got != tc.want {
				t.Errorf("NegotiateAPI(path=%q accept=%q ua=%q) = %v, want %v", tc.path, tc.accept, tc.agent, got, tc.want)
			}
		})
	}
}

func TestNegotiateFrontend(t *testing.T) {
	tests := []struct {
		path   string
		accept string
		agent  string
		want   Format
		reason string
	}{
		{path: "/jokes/random", accept: "text/html", agent: "curl/8.5.0", want: FormatHTML, reason: "priority 1 Accept text/html beats curl"},
		{path: "/jokes/random", accept: "text/html,application/xhtml+xml;q=0.9", agent: "", want: FormatHTML, reason: "priority 1 with a multi-entry Accept"},
		{path: "/jokes/random", accept: "text/plain", agent: "Mozilla/5.0 Firefox/121.0", want: FormatText, reason: "priority 2 Accept text/plain beats browser"},
		{path: "/jokes/random", accept: "", agent: "Mozilla/5.0 Firefox/121.0", want: FormatHTML, reason: "priority 3 browser user-agent"},
		{path: "/jokes/random", accept: "", agent: "Lynx/2.8.9rel.1", want: FormatHTML, reason: "priority 3 text browsers get no-JS HTML"},
		{path: "/jokes/random", accept: "", agent: "curl/8.5.0", want: FormatText, reason: "priority 4 curl"},
		{path: "/jokes/random", accept: "*/*", agent: "Wget/1.21.4", want: FormatText, reason: "priority 4 wildcard accept, http tool"},
		{path: "/jokes/random", accept: "", agent: "", want: FormatText, reason: "priority 4 empty user-agent"},
		{path: "/jokes/random", accept: "", agent: "SomeUnknownAgent/1.0", want: FormatHTML, reason: "priority 5 default"},
		{path: "/jokes/random", accept: "application/json", agent: "Mozilla/5.0 Firefox/121.0", want: FormatHTML, reason: "priority 5 default, frontend chain has no JSON step"},
		{path: "/jokes/random.txt", accept: "", agent: "Mozilla/5.0 Firefox/121.0", want: FormatHTML, reason: "frontend routes do not honor the .txt extension"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			got := NegotiateFrontend(negotiationRequest(tc.path, tc.accept, tc.agent))
			if got != tc.want {
				t.Errorf("NegotiateFrontend(path=%q accept=%q ua=%q) = %v, want %v", tc.path, tc.accept, tc.agent, got, tc.want)
			}
		})
	}
}

func TestAcceptsMediaType(t *testing.T) {
	tests := []struct {
		accept    string
		mediaType string
		want      bool
		reason    string
	}{
		{accept: "application/json", mediaType: "application/json", want: true, reason: "exact match"},
		{accept: "application/json; charset=utf-8", mediaType: "application/json", want: true, reason: "parameters stripped"},
		{accept: "text/html, application/json;q=0.8", mediaType: "application/json", want: true, reason: "second entry with q value"},
		{accept: "  TEXT/PLAIN  ", mediaType: "text/plain", want: true, reason: "case and padding insensitive"},
		{accept: "*/*", mediaType: "application/json", want: false, reason: "wildcard is not an explicit request"},
		{accept: "application/jsonp", mediaType: "application/json", want: false, reason: "no prefix matching"},
		{accept: "", mediaType: "text/html", want: false, reason: "missing header"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			if got := acceptsMediaType(tc.accept, tc.mediaType); got != tc.want {
				t.Errorf("acceptsMediaType(%q, %q) = %v, want %v", tc.accept, tc.mediaType, got, tc.want)
			}
		})
	}
}

func TestFormatStringAndContentType(t *testing.T) {
	tests := []struct {
		format      Format
		name        string
		contentType string
	}{
		{format: FormatJSON, name: "json", contentType: "application/json; charset=utf-8"},
		{format: FormatText, name: "text", contentType: "text/plain; charset=utf-8"},
		{format: FormatHTML, name: "html", contentType: "text/html; charset=utf-8"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.format.String(); got != tc.name {
				t.Errorf("Format.String() = %q, want %q", got, tc.name)
			}
			if got := tc.format.ContentType(); got != tc.contentType {
				t.Errorf("Format.ContentType() = %q, want %q", got, tc.contentType)
			}
		})
	}
}

func TestNegotiateWithNilRequest(t *testing.T) {
	// A nil request must not panic; it behaves like a request with no
	// headers, which is a non-interactive client.
	if got := NegotiateAPI(nil); got != FormatText {
		t.Errorf("NegotiateAPI(nil) = %v, want %v", got, FormatText)
	}
	if got := NegotiateFrontend(nil); got != FormatText {
		t.Errorf("NegotiateFrontend(nil) = %v, want %v", got, FormatText)
	}
}
