package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/common/httputil"
)

// errNoSuchTable stands in for a database failure whose text must never
// reach a client.
var errNoSuchTable = errors.New("sql: no such table: zones")

func TestWriteNegotiated(t *testing.T) {
	o := testOptions(t)
	payload := Payload{
		JSON:  map[string]string{"answer": "yes"},
		Text:  "answer: yes\n",
		HTML:  "<h1>answer</h1>\n<p>yes</p>\n",
		Title: "Answer",
	}

	tests := []struct {
		name        string
		negotiate   Negotiator
		headers     map[string]string
		wantType    string
		wantContain string
	}{
		{
			name:        "frontend defaults to html",
			negotiate:   httputil.NegotiateFrontend,
			headers:     map[string]string{"User-Agent": "Mozilla/5.0"},
			wantType:    "text/html; charset=utf-8",
			wantContain: "<h1>answer</h1>",
		},
		{
			name:        "api defaults to json for an interactive client",
			negotiate:   httputil.NegotiateAPI,
			headers:     map[string]string{"User-Agent": "Mozilla/5.0"},
			wantType:    "application/json; charset=utf-8",
			wantContain: `"answer": "yes"`,
		},
		{
			name:        "api answers a non-interactive client with text",
			negotiate:   httputil.NegotiateAPI,
			headers:     map[string]string{"User-Agent": "curl/8.5.0"},
			wantType:    "text/plain; charset=utf-8",
			wantContain: "answer: yes",
		},
		{
			name:        "text is honored",
			negotiate:   httputil.NegotiateAPI,
			headers:     map[string]string{"Accept": "text/plain"},
			wantType:    "text/plain; charset=utf-8",
			wantContain: "answer: yes",
		},
		{
			name:        "our cli overrides the frontend chain",
			negotiate:   httputil.NegotiateFrontend,
			headers:     map[string]string{"User-Agent": "redxt-cli/1.0.0"},
			wantType:    "application/json; charset=utf-8",
			wantContain: `"answer": "yes"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/anything", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			WriteNegotiated(rec, req, o, tt.negotiate, payload)

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

func TestWriteNegotiatedDerivesTextFromHTML(t *testing.T) {
	o := testOptions(t)

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Accept", "text/plain")
	rec := httptest.NewRecorder()

	WriteNegotiated(rec, req, o, httputil.NegotiateAPI, Payload{
		HTML: "<h1>Zone removed</h1>\n",
	})

	// The converter renders an h1 as an uppercased banner, so the
	// comparison is case-insensitive.
	body := rec.Body.String()
	if !strings.Contains(strings.ToLower(body), "zone removed") {
		t.Errorf("derived text body = %q, want it to carry the heading", body)
	}
	if strings.Contains(body, "<h1>") {
		t.Errorf("derived text body still contains markup: %q", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Error("text body does not end in a newline")
	}
}

func TestWriteNegotiatedHonorsTheStatus(t *testing.T) {
	o := testOptions(t)

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()

	WriteNegotiated(rec, req, o, httputil.NegotiateAPI, Payload{
		JSON:   map[string]string{"state": "draining"},
		Status: http.StatusServiceUnavailable,
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestWriteError(t *testing.T) {
	o := testOptions(t)

	tests := []struct {
		name     string
		target   string
		headers  map[string]string
		wantType string
	}{
		{"api path answers json", "/api/v1/server/zones", map[string]string{"Accept": "application/json"}, "application/json; charset=utf-8"},
		{"frontend path answers html", "/zones", map[string]string{"User-Agent": "Mozilla/5.0"}, "text/html; charset=utf-8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			WriteError(rec, req, o, apierror.New(apierror.CodeNotFound))

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestWriteErrorNeverLeaksTheInternalCause(t *testing.T) {
	// AI.md PART 11's public endpoint safety principle: the internal
	// cause is for the log, never for the client.
	o := testOptions(t)
	e := apierror.Wrap(apierror.CodeServerError, errNoSuchTable)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/zones", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	WriteError(rec, req, o, e)

	body := rec.Body.String()
	if strings.Contains(body, errNoSuchTable.Error()) {
		t.Errorf("response leaks the internal cause:\n%s", body)
	}

	var envelope apierror.Response
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if envelope.OK {
		t.Error("error envelope has ok = true, want false")
	}
	if envelope.Error != apierror.CodeServerError {
		t.Errorf("error = %q, want %q", envelope.Error, apierror.CodeServerError)
	}
	if envelope.Debug != nil {
		t.Errorf("_debug = %v, want it absent in production", envelope.Debug)
	}
}

func TestIsAPIPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"versioned api route", "/api/v1/server/zones", true},
		{"unversioned alias", "/api/swagger", true},
		{"bare api root", "/api", true},
		{"frontend route", "/server/healthz", false},
		{"root", "/", false},
		{"a path that merely starts with the letters", "/apidocs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAPIPath(tt.path, "/api/v1"); got != tt.want {
				t.Errorf("isAPIPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHTMLDocument(t *testing.T) {
	o := testOptions(t)

	doc := HTMLDocument(o, Payload{HTML: "<p>body</p>\n", Title: "Zones"})

	for _, want := range []string{
		"<!DOCTYPE html>",
		`<meta name="viewport" content="width=device-width, initial-scale=1">`,
		"<title>Zones</title>",
		"prefers-color-scheme: dark",
		"<p>body</p>",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document is missing %q:\n%s", want, doc)
		}
	}
	if !strings.HasSuffix(doc, "</html>\n") {
		t.Error("document does not end with a newline-terminated </html>")
	}
}

func TestHTMLDocumentFallsBackToTheApplicationName(t *testing.T) {
	o := testOptions(t)

	doc := HTMLDocument(o, Payload{HTML: "<p>body</p>\n"})

	want := "<title>" + escapeHTML(o.Config.Server.ApplicationName) + "</title>"
	if !strings.Contains(doc, want) {
		t.Errorf("document is missing %q:\n%s", want, doc)
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text is unchanged", "example.com", "example.com"},
		{"ampersand", "a&b", "a&amp;b"},
		{"tags", "<script>", "&lt;script&gt;"},
		{"double quote", `a"b`, "a&quot;b"},
		{"single quote", "a'b", "a&#39;b"},
		{"ampersand is escaped once", "&lt;", "&amp;lt;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeHTML(tt.in); got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
