package server

import (
	"net/http"
	"strings"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/common/httputil"
)

// TextWidth is the column width the HTML-to-text converter renders at.
// AI.md PART 14 renders text responses for terminal clients, and 80
// columns is the width every terminal is guaranteed to have.
const TextWidth = 80

// Payload holds the three representations of one response. A handler
// fills in whichever it can and the renderer picks the one the client
// negotiated; an empty Text is derived from HTML by the PART 14
// HTML2TextConverter rather than being left blank.
type Payload struct {
	// JSON is the value serialized for a JSON response.
	JSON any
	// Text is the plain-text body, ending in a single newline.
	Text string
	// HTML is the response body fragment, wrapped into a full document
	// by the renderer.
	HTML string
	// Title names the HTML document. An empty value falls back to the
	// application name.
	Title string
	// Status is the HTTP status code. Zero means 200.
	Status int
}

// Negotiator selects the response format for a request. The two
// implementations are httputil.NegotiateAPI for /api routes and
// httputil.NegotiateFrontend for everything else.
type Negotiator func(*http.Request) httputil.Format

// WriteNegotiated writes a payload in the format the client asked for.
//
// Our own CLI is answered with JSON before the frontend chain runs, per
// the PART 14 handler example: without that check the CLI's request
// would fall through to the frontend chain's HTML default.
func WriteNegotiated(w http.ResponseWriter, r *http.Request, o Options, negotiate Negotiator, p Payload) {
	format := httputil.FormatJSON
	if !httputil.IsOurCLIClient(r) {
		format = negotiate(r)
	}

	status := p.Status
	if status == 0 {
		status = http.StatusOK
	}

	switch format {
	case httputil.FormatText:
		text := p.Text
		if text == "" && p.HTML != "" {
			text = httputil.HTML2TextConverter(p.HTML, TextWidth)
		}
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		w.Header().Set("Content-Type", httputil.FormatText.ContentType())
		w.WriteHeader(status)
		_, _ = w.Write([]byte(text))

	case httputil.FormatHTML:
		w.Header().Set("Content-Type", httputil.FormatHTML.ContentType())
		w.WriteHeader(status)
		_, _ = w.Write([]byte(HTMLDocument(o, p)))

	default:
		w.Header().Set("Content-Type", httputil.FormatJSON.ContentType())
		w.WriteHeader(status)
		_ = apierror.WriteJSON(w, p.JSON)
	}
}

// WriteError renders an error in the format the client asked for. The
// error's internal cause never reaches the response, per the PART 11
// public endpoint safety principle.
func WriteError(w http.ResponseWriter, r *http.Request, o Options, e *apierror.Error) {
	negotiate := httputil.NegotiateFrontend
	if isAPIPath(r.URL.Path, o.Config.APIBasePath()) {
		negotiate = httputil.NegotiateAPI
	}

	envelope := apierror.Response{
		OK:      false,
		Error:   e.Code,
		Message: e.Message,
		Details: e.Details,
	}

	WriteNegotiated(w, r, o, negotiate, Payload{
		JSON:   envelope,
		Text:   e.Code + ": " + e.Message + "\n",
		HTML:   "<h1>" + escapeHTML(e.Message) + "</h1>\n<p>" + escapeHTML(e.Code) + "</p>\n",
		Title:  e.Message,
		Status: e.HTTPStatusCode,
	})
}

// isAPIPath reports whether a path belongs to the API surface, which is
// either the versioned tree or one of its unversioned aliases.
func isAPIPath(path, apiBase string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, apiBase+"/")
}

// pageStyle is the minimal theme every server-rendered page carries so
// that no response is unstyled. It uses custom properties and honors
// prefers-color-scheme, per the PART 16 theming rules the full frontend
// builds on.
const pageStyle = `:root {
  color-scheme: light dark;
  --bg: #f8f8f2;
  --fg: #282a36;
  --accent: #6272a4;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #282a36;
    --fg: #f8f8f2;
    --accent: #8be9fd;
  }
}
body {
  background: var(--bg);
  color: var(--fg);
  font-family: system-ui, sans-serif;
  line-height: 1.5;
  margin: 0 auto;
  max-width: 48rem;
  overflow-wrap: anywhere;
  padding: 1.5rem;
  word-break: break-word;
}
h1 {
  font-size: 1.5rem;
}
a {
  color: var(--accent);
}
table {
  border-collapse: collapse;
  width: 100%;
}
td, th {
  border-bottom: 1px solid var(--accent);
  padding: 0.25rem 0.5rem;
  text-align: left;
}`

// HTMLDocument wraps a body fragment in a complete, mobile-first
// document. Every page the server renders goes through here so the
// viewport, language, and theme are never forgotten on one route.
func HTMLDocument(o Options, p Payload) string {
	title := p.Title
	if title == "" {
		title = o.Config.Server.ApplicationName
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString(`<html lang="` + escapeHTML(o.Config.Server.I18n.DefaultLanguage) + `">` + "\n")
	b.WriteString("<head>\n")
	b.WriteString(`  <meta charset="utf-8">` + "\n")
	b.WriteString(`  <meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	b.WriteString("  <title>" + escapeHTML(title) + "</title>\n")
	b.WriteString("  <style>\n" + pageStyle + "\n  </style>\n")
	b.WriteString("</head>\n")
	b.WriteString("<body>\n")
	b.WriteString(p.HTML)
	if !strings.HasSuffix(p.HTML, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("</body>\n")
	b.WriteString("</html>\n")
	return b.String()
}

// escapeHTML escapes the five characters that can break out of HTML
// text or an attribute value.
var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

// escapeHTML renders a string safe for both element text and quoted
// attribute values.
func escapeHTML(s string) string {
	return htmlEscaper.Replace(s)
}
