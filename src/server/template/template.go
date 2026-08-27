// Package template renders the server-side pages for AI.md PART 34
// (Multi-User), PART 35 (Organizations), and PART 36 (Custom Domains).
//
// Every page is a plain HTML document with forms that submit to the same
// routes the REST surface serves, so the whole feature set works with
// JavaScript disabled. Nothing here loads a script, and nothing here
// depends on one: PART 16 makes the no-JS path the primary path and
// leaves script as an optional enhancement layered on top later.
package template

import (
	"html/template"
	"io"
	"net/http"

	"github.com/webappsgo/redxt/src/common/i18n"
)

// Viewer is the signed-in account a page renders navigation for. A nil
// Viewer renders the signed-out navigation instead.
type Viewer struct {
	ID          int64
	Username    string
	DisplayName string
}

// Page is the data every template receives.
type Page struct {
	// Title names the document.
	Title string
	// AppName is the configured application name, shown in the header.
	AppName string
	// Language is the document language attribute, an i18n.Supported()
	// code. Direction() derives dir="ltr"/"rtl" from it.
	Language string
	// Base is the web path prefix the page's links are built from.
	Base string
	// CSRF is the double-submit token embedded in every form.
	CSRF string
	// Viewer is the signed-in account, or nil when anonymous.
	Viewer *Viewer
	// Notice is a neutral message shown above the content.
	Notice string
	// Error is a failure message shown above the content.
	Error string
	// Data is the page's own payload.
	Data any
}

// Direction returns "rtl" for a right-to-left language (Arabic) and
// "ltr" otherwise, for the document's dir="" attribute per PART 31
// a11y requirements.
func (p Page) Direction() string {
	return i18n.Direction(p.Language)
}

// Set holds the parsed templates.
type Set struct {
	tpl *template.Template
}

// New parses the built-in pages.
//
// Parsing happens once at startup rather than per request, so a template
// that will not parse is a startup failure the operator sees immediately
// instead of a 500 the first visitor discovers.
func New() (*Set, error) {
	// Parse must see every function name the templates reference, so a
	// placeholder FuncMap is bound here. Render below clones the
	// parsed tree per request and rebinds t/tf/tp to the actual
	// request language — Parse never runs again, only the cheap
	// Clone+Funcs rebinding does.
	tpl, err := template.New("redxt").Funcs(i18n.FuncMap(i18n.DefaultLanguage)).Parse(pages)
	if err != nil {
		return nil, err
	}
	return &Set{tpl: tpl}, nil
}

// Has reports whether a page name is defined.
func (s *Set) Has(name string) bool {
	return s != nil && s.tpl.Lookup(name) != nil
}

// Render writes a page.
//
// The body is buffered before anything is written, so a template that
// fails halfway cannot leave a half-rendered page on the wire with a 200
// already committed to it.
func (s *Set) Render(w http.ResponseWriter, status int, name string, data Page) error {
	buf := &bufferWriter{}
	// Clone shares the already-parsed tree (no re-parsing) and rebinds
	// t/tf/tp to this request's language, so concurrent requests in
	// different languages never race on a shared FuncMap.
	rendered, err := s.tpl.Clone()
	if err != nil {
		return err
	}
	rendered = rendered.Funcs(i18n.FuncMap(data.Language))
	if err := rendered.ExecuteTemplate(buf, name, data); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(buf.bytes)
	return err
}

// bufferWriter collects a rendered page in memory.
type bufferWriter struct {
	bytes []byte
}

// Write appends to the buffer and never fails.
func (b *bufferWriter) Write(p []byte) (int, error) {
	b.bytes = append(b.bytes, p...)
	return len(p), nil
}

// Ensure bufferWriter satisfies the writer the template engine wants.
var _ io.Writer = (*bufferWriter)(nil)
