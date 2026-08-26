package httputil

import (
	"net/http"
	"strings"
)

// Format is the response representation chosen by content negotiation,
// per AI.md PART 14 "Content Negotiation Priority".
type Format int

const (
	// FormatJSON is a JSON response body.
	FormatJSON Format = iota
	// FormatText is a plain-text response body. On API routes this is raw
	// data; on frontend routes it is HTML2TextConverter output.
	FormatText
	// FormatHTML is an HTML response body.
	FormatHTML
)

// String returns the short name of the format, matching the format words
// used by the AI.md PART 14 tables.
func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatText:
		return "text"
	case FormatHTML:
		return "html"
	default:
		return "unknown"
	}
}

// ContentType returns the Content-Type header value for the format,
// charset included. An unrecognized format falls back to plain text
// because dumping an unknown body as text can never be interpreted as
// markup or script by the client.
func (f Format) ContentType() string {
	switch f {
	case FormatJSON:
		return "application/json; charset=utf-8"
	case FormatHTML:
		return "text/html; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

// TextExtension is the path suffix that forces a plain-text response on
// API routes, per the AI.md PART 14 ".txt extension support" rules.
const TextExtension = ".txt"

// acceptsMediaType reports whether the Accept header lists the given media
// type. The header is comma-separated and each entry may carry parameters
// after a semicolon (";q=0.9", ";charset=utf-8"), which are stripped
// before comparison.
//
// Quality values are deliberately ignored for ordering: AI.md PART 14
// fixes the precedence between media types itself, so the caller's stated
// order wins rather than the client's q-values.
func acceptsMediaType(accept, mediaType string) bool {
	for _, entry := range strings.Split(accept, ",") {
		value := entry
		if idx := strings.Index(value, ";"); idx >= 0 {
			value = value[:idx]
		}
		if strings.EqualFold(strings.TrimSpace(value), mediaType) {
			return true
		}
	}
	return false
}

// acceptHeader returns the request Accept header, or an empty string for a
// nil request so negotiation can never panic inside a handler.
func acceptHeader(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.Header.Get("Accept")
}

// requestPath returns the request path, or an empty string for a nil
// request or a request with no parsed URL.
func requestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}

// AcceptsJSON reports whether the request explicitly asked for JSON.
//
// The frontend chain in PART 14 has no JSON step, but a handful of
// frontend routes are documented as answering "JSON for API clients" —
// /server/healthz among them — so those handlers consult this first.
func AcceptsJSON(r *http.Request) bool {
	return acceptsMediaType(acceptHeader(r), "application/json")
}

// NegotiateAPI resolves the response format for an API route
// (/api/{api_version}/*), following the AI.md PART 14 priority table
// exactly:
//
//  1. ".txt" path extension  -> text (always)
//  2. Accept: application/json -> json
//  3. Accept: text/plain       -> text
//  4. Non-interactive client   -> text
//  5. Default                  -> json
//
// API text output is raw data, never HTML2TextConverter output, because an
// API route has no rendered HTML to convert.
func NegotiateAPI(r *http.Request) Format {
	// 1. A .txt extension always wins.
	if strings.HasSuffix(requestPath(r), TextExtension) {
		return FormatText
	}

	accept := acceptHeader(r)

	// 2. An explicit JSON request outranks every remaining signal.
	if acceptsMediaType(accept, "application/json") {
		return FormatJSON
	}

	// 3. An explicit plain-text request.
	if acceptsMediaType(accept, "text/plain") {
		return FormatText
	}

	// 4. curl, wget, httpie and friends, including an empty User-Agent.
	if IsNonInteractiveClient(r) {
		return FormatText
	}

	// 5. API routes default to JSON.
	return FormatJSON
}

// NegotiateFrontend resolves the response format for a frontend route
// (/**), following the AI.md PART 14 priority table exactly:
//
//  1. Accept: text/html       -> html
//  2. Accept: text/plain      -> text
//  3. Browser User-Agent      -> html
//  4. CLI/curl User-Agent     -> text
//  5. Default                 -> html
//
// Frontend text output is HTML2TextConverter output, not raw data.
//
// The chain has no JSON step because AI.md PART 14 routes our own client
// past negotiation entirely: a handler checks IsOurCLIClient first and
// answers it with JSON, and only then negotiates for everyone else.
func NegotiateFrontend(r *http.Request) Format {
	accept := acceptHeader(r)

	// 1. An explicit HTML request.
	if acceptsMediaType(accept, "text/html") {
		return FormatHTML
	}

	// 2. An explicit plain-text request.
	if acceptsMediaType(accept, "text/plain") {
		return FormatText
	}

	// 3. Any browser, graphical or text-mode, gets HTML.
	if IsBrowser(r) {
		return FormatHTML
	}

	// 4. Non-interactive HTTP tools get converted text.
	if IsHTTPTool(r) {
		return FormatText
	}

	// 5. Frontend routes default to HTML.
	return FormatHTML
}
