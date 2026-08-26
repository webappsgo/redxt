// Package graphql tests — cover the POST endpoint, the GraphiQL page,
// and the theme helpers.

package graphql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestOptions returns options wired to the test schema and resolvers.
func newTestOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Schema:        newTestSchema(t),
		Resolvers:     newTestResolvers(t),
		EndpointPath:  testAPIBase + "/server/graphql",
		SwaggerUIPath: "/server/docs/swagger",
		Title:         "redxt",
		Theme:         ThemeAuto,
	}
}

func TestNewHandlerRejectsIncompleteOptions(t *testing.T) {
	cases := []struct {
		name  string
		amend func(*Options)
	}{
		{name: "no schema", amend: func(o *Options) { o.Schema = nil }},
		{name: "no resolvers", amend: func(o *Options) { o.Resolvers = nil }},
		{name: "no endpoint path", amend: func(o *Options) { o.EndpointPath = "" }},
		{name: "relative endpoint path", amend: func(o *Options) { o.EndpointPath = "graphql" }},
		{name: "no title", amend: func(o *Options) { o.Title = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newTestOptions(t)
			tc.amend(&o)
			if _, err := NewHandler(o); err == nil {
				t.Error("NewHandler() succeeded, want an error")
			}
			if _, err := NewUIHandler(o); err == nil {
				t.Error("NewUIHandler() succeeded, want an error")
			}
		})
	}
}

func TestHandlerExecutesQuery(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewHandler(o)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	body := `{"query":"{ widgetGet(id: \"w-1\") { id } }"}`
	req := httptest.NewRequest(http.MethodPost, o.EndpointPath, strings.NewReader(body))
	req.Header.Set("Content-Type", ContentTypeJSON)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ContentTypeJSON+"; charset=utf-8" {
		t.Errorf("content type = %q, want the JSON media type", ct)
	}
	if !strings.HasSuffix(rec.Body.String(), "\n") {
		t.Error("the response does not end with a newline")
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("the response is not valid JSON: %v", err)
	}
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", resp.Errors)
	}
	widget, ok := resp.Data["widgetGet"].(map[string]any)
	if !ok {
		t.Fatalf("data[widgetGet] = %#v, want an object", resp.Data["widgetGet"])
	}
	if widget["id"] != "w-1" {
		t.Errorf("id = %#v, want %q", widget["id"], "w-1")
	}
}

func TestHandlerAcceptsBareQueryDocument(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewHandler(o)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, o.EndpointPath, strings.NewReader(`{ widgetGet(id: "w-3") { id } }`))
	req.Header.Set("Content-Type", ContentTypeGraphQL)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "w-3") {
		t.Errorf("body = %s, want the resolved widget", rec.Body.String())
	}
}

func TestHandlerRejectsBadRequests(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewHandler(o)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	cases := []struct {
		name        string
		method      string
		contentType string
		body        string
		wantStatus  int
	}{
		{name: "GET", method: http.MethodGet, wantStatus: http.StatusMethodNotAllowed},
		{name: "malformed JSON", method: http.MethodPost, contentType: ContentTypeJSON, body: "{", wantStatus: http.StatusBadRequest},
		{name: "empty query", method: http.MethodPost, contentType: ContentTypeJSON, body: `{"query":"   "}`, wantStatus: http.StatusBadRequest},
		{name: "unsupported media type", method: http.MethodPost, contentType: "text/csv", body: "id", wantStatus: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, o.EndpointPath, strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("the error body is not valid JSON: %v", err)
			}
			if envelope.OK {
				t.Error("the error envelope reports ok")
			}
			if envelope.Error == "" {
				t.Error("the error envelope carries no error code")
			}
		})
	}
}

func TestHandlerReportsQueryErrorsWithStatusOK(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewHandler(o)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, o.EndpointPath, strings.NewReader(`{"query":"{ nosuchField }"}`))
	req.Header.Set("Content-Type", ContentTypeJSON)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for a GraphQL level failure", rec.Code, http.StatusOK)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("the response is not valid JSON: %v", err)
	}
	if len(resp.Errors) == 0 {
		t.Error("errors = none, want the unknown field failure")
	}
}

func TestUIHandlerIsSelfContained(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewUIHandler(o)
	if err != nil {
		t.Fatalf("NewUIHandler: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/server/docs/graphql", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("content type = %q, want the HTML media type", ct)
	}

	page := rec.Body.String()
	for _, forbidden := range []string{"http://", "https://", "//cdn", "<script", "src="} {
		if strings.Contains(page, forbidden) {
			t.Errorf("the page contains %q, which a strict CSP forbids", forbidden)
		}
	}
	for _, want := range []string{"<style>", "--dark-bg", "prefers-color-scheme", o.EndpointPath, "widgetGet"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
}

func TestUIHandlerExecutesSubmittedForm(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewUIHandler(o)
	if err != nil {
		t.Fatalf("NewUIHandler: %v", err)
	}

	form := url.Values{}
	form.Set("query", `{ widgetGet(id: "w-4") { id } }`)
	req := httptest.NewRequest(http.MethodPost, "/server/docs/graphql", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	page := rec.Body.String()
	if !strings.Contains(page, "result-window") {
		t.Error("the page does not render a result panel")
	}
	if !strings.Contains(page, "w-4") {
		t.Error("the page does not contain the resolved widget")
	}
}

func TestUIHandlerRejectsOtherMethods(t *testing.T) {
	o := newTestOptions(t)
	h, err := NewUIHandler(o)
	if err != nil {
		t.Fatalf("NewUIHandler: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/server/docs/graphql", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow header = %q, want %q", allow, "GET, POST")
	}
}

func TestThemeClass(t *testing.T) {
	cases := []struct {
		pref string
		want string
	}{
		{pref: ThemeDark, want: "theme-dark"},
		{pref: ThemeLight, want: "theme-light"},
		{pref: ThemeAuto, want: "theme-auto"},
		{pref: "", want: "theme-auto"},
		{pref: "solarized", want: "theme-auto"},
	}
	for _, tc := range cases {
		t.Run("pref="+tc.pref, func(t *testing.T) {
			if got := ThemeClass(tc.pref); got != tc.want {
				t.Fatalf("ThemeClass(%q) = %q, want %q", tc.pref, got, tc.want)
			}
		})
	}
}

func TestCSSDefinesEveryUsedToken(t *testing.T) {
	css := CSS()
	for _, token := range []string{"--bg", "--text", "--border", "--accent", "--field", "--ok", "--warn", "--btn-bg", "--tint"} {
		if !strings.Contains(css, token+":") {
			t.Errorf("the stylesheet uses %s without defining it", token)
		}
	}
	if !strings.Contains(css, "prefers-color-scheme") {
		t.Error("the stylesheet has no automatic light mode")
	}
}
