// Package httputil implements the client-type detection, content
// negotiation and HTML-to-terminal-text rendering described in AI.md
// PART 14 ("Content Negotiation" and "Client Type Detection & Response").
//
// The spec writes the detection helpers with lowercase names because it
// shows them living inside a single handler file. They are exported here
// because the router and the handlers that consume them live in other
// packages.
package httputil

import (
	"net/http"
	"strings"
)

// CLIUserAgentPrefix is the User-Agent prefix sent by our own client
// binary, redxt-cli. Our client is INTERACTIVE: it receives JSON and
// renders its own TUI/GUI, so it must never be given pre-formatted text.
const CLIUserAgentPrefix = "redxt-cli/"

// textBrowserAgents lists the User-Agent fragments identifying text-mode
// browsers, per the AI.md PART 14 isTextBrowser table. Text browsers are
// INTERACTIVE but have no JavaScript, so they get the no-JS HTML page.
// Matching is case-insensitive; links announces itself with a trailing
// space rather than a slash, so both spellings are listed.
var textBrowserAgents = []string{
	// Lynx - classic text browser.
	"lynx/",
	// w3m - text browser with table support.
	"w3m/",
	// Links - text browser, space-separated version.
	"links ",
	// Links - alternative slash-separated version.
	"links/",
	// ELinks - enhanced links.
	"elinks/",
	// Browsh - modern text browser.
	"browsh/",
	// Carbonyl - Chromium rendered in the terminal.
	"carbonyl/",
	// NetSurf - lightweight browser with limited JS.
	"netsurf",
}

// httpToolAgents lists the User-Agent fragments identifying non-interactive
// HTTP tools, per the AI.md PART 14 isHttpTool list. These clients fetch and
// dump, so they receive HTML2TextConverter output. Matching is
// case-insensitive.
var httpToolAgents = []string{
	"curl/",
	"wget/",
	"httpie/",
	"libcurl/",
	"python-requests/",
	"go-http-client/",
	"axios/",
	"node-fetch/",
}

// graphicalBrowserAgents lists the User-Agent fragments identifying full
// graphical browsers. They are the "browser detection" step of the frontend
// negotiation chain in AI.md PART 14 "Content Negotiation Priority".
// Matching is case-insensitive.
var graphicalBrowserAgents = []string{
	"mozilla/",
	"chrome/",
	"chromium/",
	"safari/",
	"firefox/",
	"edge/",
	"edg/",
	"opera/",
	"opr/",
	"webkit/",
}

// userAgent returns the request User-Agent lowercased for matching.
// A nil request is treated as an empty User-Agent so that detection can
// never panic inside a handler.
func userAgent(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.ToLower(r.Header.Get("User-Agent"))
}

// containsAny reports whether s contains any of the given fragments.
func containsAny(s string, fragments []string) bool {
	for _, fragment := range fragments {
		if strings.Contains(s, fragment) {
			return true
		}
	}
	return false
}

// IsOurCLIClient reports whether the request came from our own client
// binary, identified by the CLIUserAgentPrefix User-Agent prefix.
//
// Our client is INTERACTIVE: it receives JSON and renders its own output,
// so it is explicitly not a non-interactive client.
func IsOurCLIClient(r *http.Request) bool {
	return strings.HasPrefix(userAgent(r), strings.ToLower(CLIUserAgentPrefix))
}

// IsTextBrowser reports whether the request came from a text-mode browser
// such as lynx, w3m, links, elinks, browsh, carbonyl or netsurf.
//
// Text browsers are INTERACTIVE - they navigate and submit forms - but they
// do not run JavaScript, so they receive the no-JS HTML alternative rather
// than converted text.
func IsTextBrowser(r *http.Request) bool {
	return containsAny(userAgent(r), textBrowserAgents)
}

// IsHTTPTool reports whether the request came from a non-interactive HTTP
// tool such as curl, wget, httpie, libcurl, python-requests, the Go HTTP
// client, axios or node-fetch. A request with no User-Agent at all is also
// treated as an HTTP tool, since interactive clients always send one.
func IsHTTPTool(r *http.Request) bool {
	ua := userAgent(r)
	if containsAny(ua, httpToolAgents) {
		return true
	}
	return ua == ""
}

// IsNonInteractiveClient reports whether the client needs pre-formatted
// text because it cannot render anything itself.
//
// Per AI.md PART 14 this is true ONLY for HTTP tools. Our own client and
// text browsers are interactive and are checked first so that a client
// whose User-Agent matches both lists is still treated as interactive.
func IsNonInteractiveClient(r *http.Request) bool {
	// Our client is INTERACTIVE - it receives JSON.
	if IsOurCLIClient(r) {
		return false
	}

	// Text browsers are INTERACTIVE - they render no-JS HTML themselves.
	if IsTextBrowser(r) {
		return false
	}

	// HTTP tools are NON-INTERACTIVE - they need pre-formatted text.
	return IsHTTPTool(r)
}

// IsBrowser reports whether the request came from a browser of any kind,
// graphical or text-mode. It is the User-Agent step of the frontend
// negotiation chain, where every browser is served HTML.
func IsBrowser(r *http.Request) bool {
	if IsTextBrowser(r) {
		return true
	}
	ua := userAgent(r)
	// HTTP tools must never be classified as browsers even though some of
	// them can be told to send a Mozilla-style User-Agent string.
	if containsAny(ua, httpToolAgents) {
		return false
	}
	return containsAny(ua, graphicalBrowserAgents)
}
