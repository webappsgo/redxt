// Package swagger — OpenAPI document generation and the Swagger UI
// handler required by AI.md PART 14 (API TYPES — REST, Swagger,
// GraphQL).
//
// The document is generated from the annotation registry in
// annotations.go, marshalled as JSON only (PART 14 forbids YAML), and
// held in memory, so serving it never touches the filesystem and never
// reaches an external host. The UI page is rendered server side from
// the same registry and ships no script tag and no remote asset, which
// keeps it inside a strict Content-Security-Policy and keeps it working
// with JavaScript disabled.

package swagger

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
)

// OpenAPIVersion is the specification version of the generated
// document.
const OpenAPIVersion = "3.1.0"

// Document is the root OpenAPI object.
type Document struct {
	OpenAPI    string              `json:"openapi"`
	Info       DocumentInfo        `json:"info"`
	Servers    []DocumentServer    `json:"servers,omitempty"`
	Tags       []DocumentTag       `json:"tags,omitempty"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components"`
}

// DocumentInfo is the OpenAPI info object.
type DocumentInfo struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version"`
	Contact     *Contact `json:"contact,omitempty"`
	License     *License `json:"license,omitempty"`
}

// Contact is the OpenAPI contact object.
type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// License is the OpenAPI license object.
type License struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// DocumentServer is one entry of the OpenAPI servers array.
type DocumentServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// DocumentTag groups operations in the generated document.
type DocumentTag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// PathItem holds the operations available on one path.
type PathItem struct {
	Get     *OperationObject `json:"get,omitempty"`
	Put     *OperationObject `json:"put,omitempty"`
	Post    *OperationObject `json:"post,omitempty"`
	Patch   *OperationObject `json:"patch,omitempty"`
	Delete  *OperationObject `json:"delete,omitempty"`
	Head    *OperationObject `json:"head,omitempty"`
	Options *OperationObject `json:"options,omitempty"`
}

// OperationObject is one OpenAPI operation.
type OperationObject struct {
	OperationID string                `json:"operationId"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]APIResult  `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

// Parameter is one OpenAPI parameter object.
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Schema      Schema `json:"schema"`
}

// Schema is the subset of JSON Schema the generator emits.
type Schema struct {
	Ref         string            `json:"$ref,omitempty"`
	Type        string            `json:"type,omitempty"`
	Format      string            `json:"format,omitempty"`
	Description string            `json:"description,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
}

// RequestBody is the OpenAPI request body object.
type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required"`
	Content     map[string]MediaType `json:"content"`
}

// APIResult is one OpenAPI response object.
type APIResult struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType is one OpenAPI media type object.
type MediaType struct {
	Schema Schema `json:"schema"`
}

// Components is the OpenAPI components object.
type Components struct {
	Schemas         map[string]Schema         `json:"schemas"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme describes one supported credential.
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	In           string `json:"in,omitempty"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
}

// ErrorEnvelopeName is the components key of the PART 9 error envelope.
const ErrorEnvelopeName = "ErrorEnvelope"

// Options carries everything the document builder and the two handlers
// need. Every value is a plain parameter so the router can wire the
// handlers without this package importing config or startup.
type Options struct {
	// Registry is the annotation registry to generate from.
	Registry *Registry
	// Info is the OpenAPI info block, including the application version.
	Info DocumentInfo
	// APIBasePath is the versioned API prefix, for example the value
	// returned by the config package's APIBasePath. The version segment
	// is supplied by the caller and never assumed here.
	APIBasePath string
	// SpecPath is the URL the UI links to for the raw OpenAPI JSON.
	SpecPath string
	// GraphQLUIPath is the URL of the GraphiQL page, linked from the UI.
	GraphQLUIPath string
	// Servers are the server URLs advertised in the document.
	Servers []DocumentServer
	// Theme is ThemeAuto, ThemeDark, or ThemeLight. An empty value
	// means ThemeAuto.
	Theme string
}

// validate checks the options both handlers depend on.
func (o Options) validate() error {
	if o.Registry == nil {
		return fmt.Errorf("swagger: options need a registry")
	}
	if o.APIBasePath == "" {
		return fmt.Errorf("swagger: options need an API base path")
	}
	if !strings.HasPrefix(o.APIBasePath, "/") {
		return fmt.Errorf("swagger: API base path %q must start with a slash", o.APIBasePath)
	}
	if o.Info.Title == "" {
		return fmt.Errorf("swagger: options need an info title")
	}
	if o.Info.Version == "" {
		return fmt.Errorf("swagger: options need an info version")
	}
	return nil
}

// BuildDocument generates the OpenAPI document from the registry.
func BuildDocument(o Options) (*Document, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if err := o.Registry.Validate(); err != nil {
		return nil, err
	}

	doc := &Document{
		OpenAPI: OpenAPIVersion,
		Info:    o.Info,
		Servers: o.Servers,
		Paths:   make(map[string]PathItem),
		Components: Components{
			Schemas:         make(map[string]Schema),
			SecuritySchemes: securitySchemes(),
		},
	}

	doc.Components.Schemas[ErrorEnvelopeName] = errorEnvelopeSchema()
	for _, t := range o.Registry.Types() {
		doc.Components.Schemas[t.Name] = objectSchema(t)
	}

	tags := make(map[string]bool)
	for _, op := range o.Registry.Operations() {
		tags[op.TagOrDefault()] = true
		path := op.FullPath(o.APIBasePath)
		item := doc.Paths[path]
		obj := operationObject(op)
		switch op.Method {
		case "GET":
			item.Get = obj
		case "PUT":
			item.Put = obj
		case "POST":
			item.Post = obj
		case "PATCH":
			item.Patch = obj
		case "DELETE":
			item.Delete = obj
		case "HEAD":
			item.Head = obj
		case "OPTIONS":
			item.Options = obj
		default:
			return nil, fmt.Errorf("swagger: operation %q has unsupported method %q", op.ID, op.Method)
		}
		doc.Paths[path] = item
	}

	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		doc.Tags = append(doc.Tags, DocumentTag{Name: name})
	}
	return doc, nil
}

// securitySchemes returns the credential definitions matching the Auth
// constants.
func securitySchemes() map[string]SecurityScheme {
	return map[string]SecurityScheme{
		AuthBearer.SecuritySchemeName(): {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "opaque",
			Description:  "API token presented as an Authorization bearer header.",
		},
		AuthSession.SecuritySchemeName(): {
			Type:        "apiKey",
			In:          "cookie",
			Name:        "session",
			Description: "Browser session cookie issued after an interactive login.",
		},
	}
}

// errorEnvelopeSchema is the PART 9 error envelope, shared by every
// failure response in the document.
func errorEnvelopeSchema() Schema {
	return Schema{
		Type:        "object",
		Description: "Unified error envelope. The internal cause is never serialised into a response.",
		Required:    []string{"ok", "error", "message"},
		Properties: map[string]Schema{
			"ok":      {Type: "boolean", Description: "Always false on an error response."},
			"error":   {Type: "string", Description: "Stable machine-readable error code."},
			"message": {Type: "string", Description: "Human-readable message safe to display."},
			"details": {Type: "object", Description: "Optional structured context, such as the failing field."},
		},
	}
}

// objectSchema renders a registered object type as a JSON Schema.
func objectSchema(t ObjectType) Schema {
	s := Schema{
		Type:        "object",
		Description: t.Description,
		Properties:  make(map[string]Schema, len(t.Fields)),
	}
	for _, f := range t.Fields {
		s.Properties[f.Name] = fieldSchema(f)
		if f.Required {
			s.Required = append(s.Required, f.Name)
		}
	}
	sort.Strings(s.Required)
	return s
}

// fieldSchema renders one field as a JSON Schema, wrapping it in an
// array schema for list fields.
func fieldSchema(f Field) Schema {
	inner := Schema{Description: f.Description}
	if f.Kind == KindObject {
		inner.Ref = "#/components/schemas/" + f.Ref
		inner.Description = ""
	} else {
		t, format := f.Kind.OpenAPIType()
		inner.Type = t
		inner.Format = format
	}
	if !f.List {
		return inner
	}
	return Schema{Type: "array", Description: f.Description, Items: &inner}
}

// successSchema returns the response schema for an operation, wrapping
// JSON payloads in the PART 9 success envelope.
func successSchema(op Operation) Schema {
	if op.ContentTypeOrDefault() != "application/json" {
		t, format := op.ResponseKind.OpenAPIType()
		return Schema{Type: t, Format: format}
	}
	var data Schema
	switch op.ResponseKind {
	case KindObject:
		data = Schema{Ref: "#/components/schemas/" + op.ResponseType}
	case KindJSON:
		data = Schema{Type: "object", Description: "Free-form JSON document."}
	default:
		t, format := op.ResponseKind.OpenAPIType()
		data = Schema{Type: t, Format: format}
	}
	if op.ResponseList {
		data = Schema{Type: "array", Items: &data}
	}
	return Schema{
		Type:     "object",
		Required: []string{"ok"},
		Properties: map[string]Schema{
			"ok":   {Type: "boolean", Description: "Always true on a success response."},
			"data": data,
		},
	}
}

// operationObject renders one registered operation.
func operationObject(op Operation) *OperationObject {
	obj := &OperationObject{
		OperationID: op.ID,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        []string{op.TagOrDefault()},
		Responses:   make(map[string]APIResult),
	}

	for _, p := range op.Params {
		t, format := p.Kind.OpenAPIType()
		obj.Parameters = append(obj.Parameters, Parameter{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required || p.In == ParamPath,
			Description: p.Description,
			Schema:      Schema{Type: t, Format: format},
		})
	}

	if op.RequestType != "" {
		obj.RequestBody = &RequestBody{
			Description: "Request payload.",
			Required:    true,
			Content: map[string]MediaType{
				"application/json": {Schema: Schema{Ref: "#/components/schemas/" + op.RequestType}},
			},
		}
	}

	obj.Responses["200"] = APIResult{
		Description: "Successful response.",
		Content: map[string]MediaType{
			op.ContentTypeOrDefault(): {Schema: successSchema(op)},
		},
	}
	errContent := map[string]MediaType{
		"application/json": {Schema: Schema{Ref: "#/components/schemas/" + ErrorEnvelopeName}},
	}
	if op.Auth != AuthNone {
		obj.Responses["401"] = APIResult{Description: "Authentication required or rejected.", Content: errContent}
		obj.Responses["403"] = APIResult{Description: "Authenticated caller lacks permission.", Content: errContent}
		obj.Security = []map[string][]string{{op.Auth.SecuritySchemeName(): {}}}
	}
	if len(op.Params) > 0 || op.RequestType != "" {
		obj.Responses["400"] = APIResult{Description: "Malformed request or failed validation.", Content: errContent}
	}
	obj.Responses["429"] = APIResult{Description: "Rate limit exceeded.", Content: errContent}
	obj.Responses["500"] = APIResult{Description: "Unexpected server-side failure.", Content: errContent}
	return obj
}

// MarshalDocument renders a document as the exact bytes served on the
// wire: indented JSON with a trailing newline.
func MarshalDocument(doc *Document) ([]byte, error) {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("swagger: marshal document: %w", err)
	}
	return append(b, '\n'), nil
}

// NewSpecHandler returns the handler serving the OpenAPI JSON document.
//
// The document is generated and marshalled once, at construction, so a
// request can never fail on generation and the bytes are identical for
// every caller. PART 14 allows JSON only: there is no YAML variant and
// no file suffix on the route.
func NewSpecHandler(o Options) (http.Handler, error) {
	doc, err := BuildDocument(o)
	if err != nil {
		return nil, err
	}
	body, err := MarshalDocument(doc)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	}), nil
}

// NewUIHandler returns the handler serving the Swagger UI page.
//
// The page is rendered once, at construction, from the same registry
// the JSON document comes from. It is a single self-contained HTML
// document with inline CSS, no script tag, and no reference to any
// external host, so it satisfies a strict CSP and works with
// JavaScript disabled.
func NewUIHandler(o Options) (http.Handler, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	if err := o.Registry.Validate(); err != nil {
		return nil, err
	}
	page := []byte(renderUI(o))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(page)
	}), nil
}

// methodNotAllowed writes the PART 9 error envelope for a rejected
// method.
func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusMethodNotAllowed)
	body := map[string]any{
		"ok":      false,
		"error":   "METHOD_NOT_ALLOWED",
		"message": "Method not allowed",
	}
	b, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return
	}
	_, _ = w.Write(append(b, '\n'))
}

// renderUI builds the complete Swagger UI document.
func renderUI(o Options) string {
	var b strings.Builder
	title := o.Info.Title + " API"

	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString(`<html lang="en" class="` + ThemeClass(o.Theme) + `">` + "\n")
	b.WriteString("<head>\n")
	b.WriteString(`  <meta charset="utf-8">` + "\n")
	b.WriteString(`  <meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	b.WriteString(`  <meta name="color-scheme" content="dark light">` + "\n")
	b.WriteString("  <title>" + html.EscapeString(title) + "</title>\n")
	b.WriteString("  <style>\n")
	b.WriteString(CSS())
	b.WriteString("  </style>\n")
	b.WriteString("</head>\n")
	b.WriteString(`<body class="swagger-ui">` + "\n")

	b.WriteString(`  <header class="topbar">` + "\n")
	b.WriteString(`    <h1 class="title">` + html.EscapeString(title) + "</h1>\n")
	b.WriteString(`    <p class="version">Version ` + html.EscapeString(o.Info.Version) + " &middot; OpenAPI " + OpenAPIVersion + "</p>\n")
	b.WriteString(`    <nav class="links">` + "\n")
	if o.SpecPath != "" {
		b.WriteString(`      <a href="` + html.EscapeString(o.SpecPath) + `">OpenAPI JSON</a>` + "\n")
	}
	if o.GraphQLUIPath != "" {
		b.WriteString(`      <a href="` + html.EscapeString(o.GraphQLUIPath) + `">GraphiQL</a>` + "\n")
	}
	b.WriteString("    </nav>\n")
	b.WriteString("  </header>\n")

	b.WriteString(`  <main class="content">` + "\n")
	if o.Info.Description != "" {
		b.WriteString(`    <p class="intro">` + html.EscapeString(o.Info.Description) + "</p>\n")
	}
	b.WriteString(`    <p class="intro">This explorer is rendered on the server. It needs no JavaScript and loads no external asset.</p>` + "\n")

	for _, tag := range groupedOperations(o.Registry) {
		b.WriteString(`    <section class="tag">` + "\n")
		b.WriteString(`      <h2 class="tag-name">` + html.EscapeString(tag.name) + "</h2>\n")
		for _, op := range tag.ops {
			writeOperation(&b, o, op)
		}
		b.WriteString("    </section>\n")
	}

	if o.Registry.Len() == 0 {
		b.WriteString(`    <p class="intro">No operations are registered.</p>` + "\n")
	}
	b.WriteString("  </main>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// tagGroup is one heading of the UI, holding the operations that share
// a tag.
type tagGroup struct {
	name string
	ops  []Operation
}

// groupedOperations groups operations by tag, with every level sorted
// so the rendered page is identical on every run.
func groupedOperations(r *Registry) []tagGroup {
	byTag := make(map[string][]Operation)
	for _, op := range r.Operations() {
		tag := op.TagOrDefault()
		byTag[tag] = append(byTag[tag], op)
	}
	names := make([]string, 0, len(byTag))
	for name := range byTag {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]tagGroup, 0, len(names))
	for _, name := range names {
		ops := byTag[name]
		sort.Slice(ops, func(i, j int) bool {
			if ops[i].Path != ops[j].Path {
				return ops[i].Path < ops[j].Path
			}
			return ops[i].Method < ops[j].Method
		})
		out = append(out, tagGroup{name: name, ops: ops})
	}
	return out
}

// writeOperation renders one collapsible operation block.
func writeOperation(b *strings.Builder, o Options, op Operation) {
	path := op.FullPath(o.APIBasePath)
	method := strings.ToLower(op.Method)

	b.WriteString(`      <details class="opblock opblock-` + method + `">` + "\n")
	b.WriteString(`        <summary class="opblock-summary">` + "\n")
	b.WriteString(`          <span class="method">` + html.EscapeString(op.Method) + "</span>\n")
	b.WriteString(`          <span class="path">` + html.EscapeString(path) + "</span>\n")
	b.WriteString(`          <span class="summary-text">` + html.EscapeString(op.Summary) + "</span>\n")
	b.WriteString("        </summary>\n")
	b.WriteString(`        <div class="opblock-body">` + "\n")

	if op.Description != "" {
		b.WriteString(`          <p class="description">` + html.EscapeString(op.Description) + "</p>\n")
	}
	b.WriteString(`          <dl class="meta">` + "\n")
	b.WriteString("            <dt>Operation ID</dt><dd><code>" + html.EscapeString(op.ID) + "</code></dd>\n")
	b.WriteString("            <dt>GraphQL field</dt><dd><code>" + html.EscapeString(op.GraphQLName()) + "</code></dd>\n")
	b.WriteString("            <dt>Authentication</dt><dd>" + html.EscapeString(authLabel(op.Auth)) + "</dd>\n")
	b.WriteString("            <dt>Responds with</dt><dd><code>" + html.EscapeString(op.ContentTypeOrDefault()) + "</code></dd>\n")
	b.WriteString("          </dl>\n")

	if len(op.Params) > 0 {
		b.WriteString(`          <h3 class="section-title">Parameters</h3>` + "\n")
		b.WriteString(`          <div class="table-scroll">` + "\n")
		b.WriteString(`          <table class="params">` + "\n")
		b.WriteString("            <thead><tr><th>Name</th><th>In</th><th>Type</th><th>Required</th><th>Description</th></tr></thead>\n")
		b.WriteString("            <tbody>\n")
		params := append([]Param(nil), op.Params...)
		sort.Slice(params, func(i, j int) bool { return params[i].Name < params[j].Name })
		for _, p := range params {
			required := "no"
			if p.Required || p.In == ParamPath {
				required = "yes"
			}
			b.WriteString("              <tr>")
			b.WriteString("<td><code>" + html.EscapeString(p.Name) + "</code></td>")
			b.WriteString("<td>" + html.EscapeString(p.In) + "</td>")
			b.WriteString("<td>" + html.EscapeString(string(p.Kind)) + "</td>")
			b.WriteString("<td>" + required + "</td>")
			b.WriteString("<td>" + html.EscapeString(p.Description) + "</td>")
			b.WriteString("</tr>\n")
		}
		b.WriteString("            </tbody>\n          </table>\n          </div>\n")
	}

	if op.RequestType != "" {
		if t, ok := o.Registry.Type(op.RequestType); ok {
			b.WriteString(`          <h3 class="section-title">Request body</h3>` + "\n")
			writeTypeTable(b, t)
		}
	}

	b.WriteString(`          <h3 class="section-title">Response</h3>` + "\n")
	if op.ResponseKind == KindObject {
		if t, ok := o.Registry.Type(op.ResponseType); ok {
			b.WriteString(`          <p class="description">Returned inside the <code>data</code> member of the success envelope`)
			if op.ResponseList {
				b.WriteString(", as a list")
			}
			b.WriteString(".</p>\n")
			writeTypeTable(b, t)
		}
	} else {
		b.WriteString(`          <p class="description">Returns a <code>` + html.EscapeString(string(op.ResponseKind)) + `</code> payload.</p>` + "\n")
	}

	writeTryIt(b, op, path)

	b.WriteString(`          <h3 class="section-title">Command line</h3>` + "\n")
	b.WriteString(`          <pre class="code">` + html.EscapeString(curlExample(op, path)) + "</pre>\n")

	b.WriteString("        </div>\n")
	b.WriteString("      </details>\n")
}

// writeTypeTable renders the fields of an object type.
func writeTypeTable(b *strings.Builder, t ObjectType) {
	b.WriteString(`          <p class="description"><code>` + html.EscapeString(t.Name) + "</code> &mdash; " + html.EscapeString(t.Description) + "</p>\n")
	b.WriteString(`          <div class="table-scroll">` + "\n")
	b.WriteString(`          <table class="params">` + "\n")
	b.WriteString("            <thead><tr><th>Field</th><th>Type</th><th>Required</th><th>Description</th></tr></thead>\n")
	b.WriteString("            <tbody>\n")
	fields := append([]Field(nil), t.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, f := range fields {
		typ := string(f.Kind)
		if f.Kind == KindObject {
			typ = f.Ref
		}
		if f.List {
			typ = "list of " + typ
		}
		required := "no"
		if f.Required {
			required = "yes"
		}
		b.WriteString("              <tr>")
		b.WriteString("<td><code>" + html.EscapeString(f.Name) + "</code></td>")
		b.WriteString("<td>" + html.EscapeString(typ) + "</td>")
		b.WriteString("<td>" + required + "</td>")
		b.WriteString("<td>" + html.EscapeString(f.Description) + "</td>")
		b.WriteString("</tr>\n")
	}
	b.WriteString("            </tbody>\n          </table>\n          </div>\n")
}

// writeTryIt renders a plain HTML form that executes the operation.
//
// A form can only issue a GET with a query string, so the try-it panel
// appears for parameterless GET routes and for GET routes whose only
// parameters are query parameters. Everything else gets the command
// line example instead, which keeps the page free of JavaScript.
func writeTryIt(b *strings.Builder, op Operation, path string) {
	if op.Method != "GET" {
		return
	}
	for _, p := range op.Params {
		if p.In != ParamQuery {
			return
		}
	}
	b.WriteString(`          <h3 class="section-title">Try it</h3>` + "\n")
	b.WriteString(`          <form class="tryit" method="get" action="` + html.EscapeString(path) + `">` + "\n")
	params := append([]Param(nil), op.Params...)
	sort.Slice(params, func(i, j int) bool { return params[i].Name < params[j].Name })
	for _, p := range params {
		id := op.ID + "-" + p.Name
		b.WriteString(`            <label for="` + html.EscapeString(id) + `">` + html.EscapeString(p.Name) + "</label>\n")
		b.WriteString(`            <input id="` + html.EscapeString(id) + `" name="` + html.EscapeString(p.Name) + `" type="text"`)
		if p.Required {
			b.WriteString(" required")
		}
		if p.Description != "" {
			b.WriteString(` placeholder="` + html.EscapeString(p.Description) + `"`)
		}
		b.WriteString(">\n")
	}
	b.WriteString(`            <button class="btn" type="submit">Execute</button>` + "\n")
	b.WriteString("          </form>\n")
}

// authLabel returns the human-readable name of an auth mode.
func authLabel(a Auth) string {
	switch a {
	case AuthBearer:
		return "Bearer token"
	case AuthSession:
		return "Session cookie"
	default:
		return "None (public endpoint)"
	}
}

// curlExample returns a runnable curl invocation for an operation.
func curlExample(op Operation, path string) string {
	var b strings.Builder
	b.WriteString("curl -sS -X " + op.Method)
	if op.Auth == AuthBearer {
		b.WriteString(" -H 'Authorization: Bearer $TOKEN'")
	}
	if op.RequestType != "" {
		b.WriteString(" -H 'Content-Type: application/json' -d '{}'")
	}
	b.WriteString(" '" + path + "'")
	return b.String()
}
