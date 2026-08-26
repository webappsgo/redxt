// Package swagger tests — cover the registry contract, the generated
// OpenAPI document, and the two HTTP handlers required by AI.md PART 14.
//
// Every assertion is deterministic. Go map iteration order is random, so
// anything derived from a map is either sorted first or checked by
// membership rather than by order.

package swagger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// testAPIBase is a deliberately unusual API prefix. PART 14 forbids
// assuming a version, so the tests prove the generated document follows
// whatever prefix the caller supplies.
const testAPIBase = "/api/v9"

// newTestRegistry returns a registry holding the fixed server operations
// plus a small object surface with a path parameter and a request body.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := RegisterServerOperations(r); err != nil {
		t.Fatalf("RegisterServerOperations: %v", err)
	}
	if err := r.RegisterType(ObjectType{
		Name:        "Widget",
		Description: "A widget.",
		Fields: []Field{
			{Name: "id", Kind: KindString, Required: true, Description: "Identifier."},
			{Name: "size", Kind: KindInt, Description: "Size in units."},
		},
	}); err != nil {
		t.Fatalf("RegisterType: %v", err)
	}
	ops := []Operation{
		{
			ID:           "widget.get",
			Method:       "GET",
			Scope:        ScopeAPI,
			Path:         "/widget/{id}",
			Summary:      "Fetch a widget",
			Tag:          "widget",
			Auth:         AuthNone,
			Params:       []Param{{Name: "id", In: ParamPath, Kind: KindString, Required: true, Description: "Identifier."}},
			ResponseKind: KindObject,
			ResponseType: "Widget",
		},
		{
			ID:           "widget.create",
			Method:       "POST",
			Scope:        ScopeAPI,
			Path:         "/widget",
			Summary:      "Create a widget",
			Tag:          "widget",
			Auth:         AuthBearer,
			RequestType:  "Widget",
			ResponseKind: KindObject,
			ResponseType: "Widget",
		},
	}
	for _, op := range ops {
		if err := r.Register(op); err != nil {
			t.Fatalf("Register %q: %v", op.ID, err)
		}
	}
	return r
}

// newTestOptions returns options wired to a fresh test registry.
func newTestOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Registry:      newTestRegistry(t),
		Info:          DocumentInfo{Title: "redxt", Version: "0.0.1"},
		APIBasePath:   testAPIBase,
		SpecPath:      testAPIBase + "/server/swagger",
		GraphQLUIPath: "/server/docs/graphql",
		Theme:         ThemeAuto,
	}
}

func TestKindOpenAPIType(t *testing.T) {
	cases := []struct {
		name       string
		kind       Kind
		wantType   string
		wantFormat string
	}{
		{name: "string", kind: KindString, wantType: "string"},
		{name: "int", kind: KindInt, wantType: "integer", wantFormat: "int64"},
		{name: "float", kind: KindFloat, wantType: "number", wantFormat: "double"},
		{name: "bool", kind: KindBool, wantType: "boolean"},
		{name: "time", kind: KindTime, wantType: "string", wantFormat: "date-time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotFormat := tc.kind.OpenAPIType()
			if gotType != tc.wantType || gotFormat != tc.wantFormat {
				t.Fatalf("OpenAPIType() = %q, %q; want %q, %q", gotType, gotFormat, tc.wantType, tc.wantFormat)
			}
		})
	}
}

func TestOperationFullPath(t *testing.T) {
	cases := []struct {
		name string
		op   Operation
		want string
	}{
		{
			name: "api scope takes the prefix",
			op:   Operation{Scope: ScopeAPI, Path: "/server/healthz"},
			want: testAPIBase + "/server/healthz",
		},
		{
			name: "root scope is verbatim",
			op:   Operation{Scope: ScopeRoot, Path: "/api/autodiscover"},
			want: "/api/autodiscover",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.op.FullPath(testAPIBase); got != tc.want {
				t.Fatalf("FullPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOperationGraphQLName(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{id: "server.healthz", want: "serverHealthz"},
		{id: "server.metrics.service", want: "serverMetricsService"},
		{id: "widget-create", want: "widgetCreate"},
		{id: "widget_list", want: "widgetList"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if got := (Operation{ID: tc.id}).GraphQLName(); got != tc.want {
				t.Fatalf("GraphQLName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRegistryRegisterRejects(t *testing.T) {
	valid := Operation{
		ID:           "widget.list",
		Method:       "GET",
		Scope:        ScopeAPI,
		Path:         "/widget",
		Summary:      "List widgets",
		Auth:         AuthNone,
		ResponseKind: KindString,
		ResponseList: true,
	}
	mutate := func(f func(*Operation)) Operation {
		op := valid
		f(&op)
		return op
	}
	cases := []struct {
		name    string
		op      Operation
		wantErr bool
	}{
		{name: "valid", op: valid},
		{name: "empty id", op: mutate(func(o *Operation) { o.ID = "" }), wantErr: true},
		{name: "bad method", op: mutate(func(o *Operation) { o.Method = "FETCH" }), wantErr: true},
		{name: "bad scope", op: mutate(func(o *Operation) { o.Scope = Scope("elsewhere") }), wantErr: true},
		{name: "relative path", op: mutate(func(o *Operation) { o.Path = "widget" }), wantErr: true},
		{name: "no summary", op: mutate(func(o *Operation) { o.Summary = "" }), wantErr: true},
		{name: "object response without type", op: mutate(func(o *Operation) { o.ResponseKind = KindObject }), wantErr: true},
		{name: "response type without object kind", op: mutate(func(o *Operation) { o.ResponseType = "Widget" }), wantErr: true},
		{name: "object parameter", op: mutate(func(o *Operation) {
			o.Params = []Param{{Name: "body", In: ParamQuery, Kind: KindObject}}
		}), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewRegistry().Register(tc.op)
			if tc.wantErr && err == nil {
				t.Fatal("Register() succeeded, want an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Register() = %v, want success", err)
			}
		})
	}
}

func TestRegistryRejectsDuplicates(t *testing.T) {
	op := Operation{
		ID:           "widget.list",
		Method:       "GET",
		Scope:        ScopeAPI,
		Path:         "/widget",
		Summary:      "List widgets",
		Auth:         AuthNone,
		ResponseKind: KindString,
	}
	r := NewRegistry()
	if err := r.Register(op); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(op); err == nil {
		t.Fatal("duplicate ID was accepted")
	}
	renamed := op
	renamed.ID = "widget.other"
	if err := r.Register(renamed); err == nil {
		t.Fatal("duplicate method and path pair was accepted")
	}
}

func TestRegistryValidateCatchesUndeclaredPathParam(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Operation{
		ID:           "widget.get",
		Method:       "GET",
		Scope:        ScopeAPI,
		Path:         "/widget/{id}",
		Summary:      "Fetch a widget",
		Auth:         AuthNone,
		ResponseKind: KindString,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Validate(); err == nil {
		t.Fatal("Validate() accepted a path parameter that was never declared")
	}
}

func TestRegistryOperationsAreSorted(t *testing.T) {
	r := newTestRegistry(t)
	ops := r.Operations()
	ids := make([]string, 0, len(ops))
	for _, op := range ops {
		ids = append(ids, op.ID)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("Operations() returned unsorted IDs: %v", ids)
	}
	types := r.Types()
	names := make([]string, 0, len(types))
	for _, tp := range types {
		names = append(names, tp.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("Types() returned unsorted names: %v", names)
	}
}

func TestBuildDocumentShape(t *testing.T) {
	doc, err := BuildDocument(newTestOptions(t))
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	if doc.OpenAPI != OpenAPIVersion {
		t.Fatalf("openapi = %q, want %q", doc.OpenAPI, OpenAPIVersion)
	}
	if doc.Info.Title != "redxt" || doc.Info.Version != "0.0.1" {
		t.Fatalf("info = %+v, want the supplied title and version", doc.Info)
	}

	wantPaths := []string{
		testAPIBase + "/server/healthz",
		testAPIBase + "/server/metrics",
		testAPIBase + "/server/metrics/{service}",
		testAPIBase + "/widget",
		testAPIBase + "/widget/{id}",
		"/api/autodiscover",
	}
	for _, path := range wantPaths {
		if _, ok := doc.Paths[path]; !ok {
			t.Errorf("paths is missing %q", path)
		}
	}
	if item := doc.Paths[testAPIBase+"/widget"]; item.Post == nil || item.Post.OperationID != "widget.create" {
		t.Errorf("POST %s/widget is missing its operation", testAPIBase)
	}
	if item := doc.Paths[testAPIBase+"/widget/{id}"]; item.Get == nil || len(item.Get.Parameters) != 1 {
		t.Errorf("GET %s/widget/{id} is missing its path parameter", testAPIBase)
	}

	for _, name := range []string{ErrorEnvelopeName, "Widget", "HealthStatus", "AutodiscoverInfo"} {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Errorf("components.schemas is missing %q", name)
		}
	}
	if _, ok := doc.Components.SecuritySchemes[AuthBearer.SecuritySchemeName()]; !ok {
		t.Error("components.securitySchemes is missing the bearer scheme")
	}

	tags := make([]string, 0, len(doc.Tags))
	for _, tag := range doc.Tags {
		tags = append(tags, tag.Name)
	}
	if !sort.StringsAreSorted(tags) {
		t.Fatalf("tags are not sorted: %v", tags)
	}
}

func TestBuildDocumentRejectsIncompleteOptions(t *testing.T) {
	cases := []struct {
		name  string
		amend func(*Options)
	}{
		{name: "no registry", amend: func(o *Options) { o.Registry = nil }},
		{name: "no base path", amend: func(o *Options) { o.APIBasePath = "" }},
		{name: "relative base path", amend: func(o *Options) { o.APIBasePath = "api" }},
		{name: "no title", amend: func(o *Options) { o.Info.Title = "" }},
		{name: "no version", amend: func(o *Options) { o.Info.Version = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newTestOptions(t)
			tc.amend(&o)
			if _, err := BuildDocument(o); err == nil {
				t.Fatal("BuildDocument() succeeded, want an error")
			}
		})
	}
}

func TestMarshalDocumentIsValidJSON(t *testing.T) {
	doc, err := BuildDocument(newTestOptions(t))
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	body, err := MarshalDocument(doc)
	if err != nil {
		t.Fatalf("MarshalDocument: %v", err)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Error("the marshalled document does not end with a newline")
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("the marshalled document is not valid JSON: %v", err)
	}
	for _, key := range []string{"openapi", "info", "paths", "components"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("the document is missing the top level key %q", key)
		}
	}

	again, err := MarshalDocument(doc)
	if err != nil {
		t.Fatalf("second MarshalDocument: %v", err)
	}
	if string(again) != string(body) {
		t.Error("marshalling the same document twice produced different bytes")
	}
}

func TestSpecHandler(t *testing.T) {
	h, err := NewSpecHandler(newTestOptions(t))
	if err != nil {
		t.Fatalf("NewSpecHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testAPIBase+"/server/swagger", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("GET content type = %q, want the JSON media type", ct)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("the served document is not valid JSON: %v", err)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, testAPIBase+"/server/swagger", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("POST Allow header = %q, want %q", allow, http.MethodGet)
	}
}

func TestUIHandlerIsSelfContained(t *testing.T) {
	h, err := NewUIHandler(newTestOptions(t))
	if err != nil {
		t.Fatalf("NewUIHandler: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/server/docs/swagger", nil))
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
	for _, want := range []string{"<style>", "--dark-bg", "prefers-color-scheme", testAPIBase + "/widget"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page is missing %q", want)
		}
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
	for _, token := range []string{"--bg", "--text", "--border", "--accent", "--get", "--post", "--btn-bg", "--tint"} {
		if !strings.Contains(css, token+":") {
			t.Errorf("the stylesheet uses %s without defining it", token)
		}
	}
	if strings.Count(css, "#") == 0 {
		t.Error("the stylesheet defines no palette values")
	}
}
