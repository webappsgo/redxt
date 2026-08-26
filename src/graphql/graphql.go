// Package graphql — the HTTP transports required by AI.md PART 14: the
// POST endpoint that executes GraphQL queries and the GraphiQL page
// that lets a person explore the schema.
//
// The page is rendered on the server, ships inline CSS only, carries no
// script tag, and references no external host, so it satisfies a strict
// Content-Security-Policy and works with JavaScript disabled. Its query
// editor is an ordinary HTML form that posts back to the page, which is
// executed server side and rendered into the result panel.
//
// Both constructors take plain parameters so the router can wire them
// without this package importing the config or startup packages.

package graphql

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
)

// DefaultMaxBodyBytes caps an accepted GraphQL request body. A query
// document is small, so a modest cap keeps a hostile client from
// spending server memory.
const DefaultMaxBodyBytes int64 = 1 << 20

// ContentTypeJSON is the media type the endpoint accepts and returns.
const ContentTypeJSON = "application/json"

// ContentTypeGraphQL is the alternative media type carrying a bare
// query document as the request body.
const ContentTypeGraphQL = "application/graphql"

// Options carries everything the two handlers need.
type Options struct {
	// Schema is the generated schema, from BuildSchema.
	Schema *Schema
	// Resolvers is the resolver set the router populated.
	Resolvers *Resolvers
	// EndpointPath is the URL that accepts GraphQL POST requests. The
	// caller builds it from its own API base path, so no API version is
	// assumed here.
	EndpointPath string
	// SwaggerUIPath is the URL of the Swagger UI page, linked from the
	// GraphiQL page. An empty value omits the link.
	SwaggerUIPath string
	// Title is the application name shown in the page heading.
	Title string
	// Theme is ThemeAuto, ThemeDark, or ThemeLight. Empty means auto.
	Theme string
	// MaxBodyBytes caps the request body. Zero means
	// DefaultMaxBodyBytes.
	MaxBodyBytes int64
}

// validate checks the options both handlers depend on.
func (o Options) validate() error {
	if o.Schema == nil {
		return fmt.Errorf("graphql: options need a schema")
	}
	if o.Resolvers == nil {
		return fmt.Errorf("graphql: options need a resolver set")
	}
	if o.EndpointPath == "" {
		return fmt.Errorf("graphql: options need an endpoint path")
	}
	if !strings.HasPrefix(o.EndpointPath, "/") {
		return fmt.Errorf("graphql: endpoint path %q must start with a slash", o.EndpointPath)
	}
	if o.Title == "" {
		return fmt.Errorf("graphql: options need a title")
	}
	return nil
}

// bodyLimit returns the effective request body cap.
func (o Options) bodyLimit() int64 {
	if o.MaxBodyBytes > 0 {
		return o.MaxBodyBytes
	}
	return DefaultMaxBodyBytes
}

// NewHandler returns the handler that executes GraphQL queries.
//
// It accepts POST only. A body that cannot be read as a request is a
// transport failure and answers with the PART 9 error envelope; a query
// that parses but fails is a GraphQL failure and answers 200 with the
// errors array, which is what GraphQL clients expect.
func NewHandler(o Options) (http.Handler, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	limit := o.bodyLimit()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeTransportError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", http.MethodPost)
			return
		}
		req, err := decodeRequest(w, r, limit)
		if err != nil {
			writeTransportError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), "")
			return
		}
		resp := Execute(r.Context(), o.Schema, o.Resolvers, req)
		writeJSON(w, http.StatusOK, resp)
	}), nil
}

// decodeRequest reads a GraphQL request from the HTTP body, accepting
// both the JSON envelope and a bare query document.
func decodeRequest(w http.ResponseWriter, r *http.Request, limit int64) (Request, error) {
	var req Request
	body := http.MaxBytesReader(w, r.Body, limit)
	mediaType := r.Header.Get("Content-Type")
	if idx := strings.Index(mediaType, ";"); idx >= 0 {
		mediaType = mediaType[:idx]
	}
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))

	if mediaType == ContentTypeGraphQL {
		raw, err := io.ReadAll(body)
		if err != nil {
			return req, fmt.Errorf("the request body could not be read")
		}
		req.Query = string(raw)
		if strings.TrimSpace(req.Query) == "" {
			return req, fmt.Errorf("the request body carries no query")
		}
		return req, nil
	}

	if mediaType != "" && mediaType != ContentTypeJSON {
		return req, fmt.Errorf("unsupported content type %q", mediaType)
	}
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		return req, fmt.Errorf("the request body is not a valid GraphQL request")
	}
	if strings.TrimSpace(req.Query) == "" {
		return req, fmt.Errorf("the request body carries no query")
	}
	return req, nil
}

// writeJSON writes an indented JSON body with the trailing newline the
// project's JSON responses always carry.
func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		w.Header().Set("Content-Type", ContentTypeJSON+"; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{\n  \"ok\": false,\n  \"error\": \"SERVER_ERROR\",\n  \"message\": \"Internal server error\"\n}\n"))
		return
	}
	w.Header().Set("Content-Type", ContentTypeJSON+"; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

// writeTransportError writes the unified PART 9 error envelope for a
// failure that happened before the query could run.
func writeTransportError(w http.ResponseWriter, status int, code, message, allow string) {
	if allow != "" {
		w.Header().Set("Allow", allow)
	}
	writeJSON(w, status, map[string]any{
		"ok":      false,
		"error":   code,
		"message": message,
	})
}

// NewUIHandler returns the handler serving the GraphiQL page.
//
// GET renders the editor. POST executes the submitted form and renders
// the result into the same page, which is what makes the explorer work
// without any JavaScript.
func NewUIHandler(o Options) (http.Handler, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	limit := o.bodyLimit()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			page := []byte(renderUI(o, defaultQuery(o), "", ""))
			writeHTML(w, page, r.Method == http.MethodHead)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			if err := r.ParseForm(); err != nil {
				page := []byte(renderUI(o, defaultQuery(o), "", "The submitted form could not be read."))
				writeHTML(w, page, false)
				return
			}
			query := r.PostFormValue("query")
			req := Request{
				Query:         query,
				OperationName: r.PostFormValue("operationName"),
			}
			notice := ""
			if raw := strings.TrimSpace(r.PostFormValue("variables")); raw != "" {
				if err := json.Unmarshal([]byte(raw), &req.Variables); err != nil {
					notice = "The variables field is not valid JSON, so it was ignored."
				}
			}
			resp := Execute(r.Context(), o.Schema, o.Resolvers, req)
			rendered, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				page := []byte(renderUI(o, query, "", "The result could not be encoded."))
				writeHTML(w, page, false)
				return
			}
			page := []byte(renderUI(o, query, string(rendered)+"\n", notice))
			writeHTML(w, page, false)
		default:
			writeTransportError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", "GET, POST")
		}
	}), nil
}

// writeHTML writes a rendered page.
func writeHTML(w http.ResponseWriter, page []byte, headOnly bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if headOnly {
		return
	}
	_, _ = w.Write(page)
}

// defaultQuery returns the example query the editor opens with, built
// from the schema so it always runs against this server.
func defaultQuery(o Options) string {
	for _, f := range o.Schema.Queries {
		if f.Name == SDLFieldName || len(f.Args) > 0 {
			continue
		}
		if typeName := baseTypeName(f.Type); typeName != "" {
			if t, ok := o.Schema.Type(typeName); ok && len(t.Fields) > 0 {
				var b strings.Builder
				b.WriteString("query {\n  " + f.Name + " {\n")
				for _, member := range t.Fields {
					if strings.HasPrefix(member.Type, "[") {
						continue
					}
					if _, nested := o.Schema.Type(baseTypeName(member.Type)); nested {
						continue
					}
					b.WriteString("    " + member.Name + "\n")
				}
				b.WriteString("  }\n}\n")
				return b.String()
			}
		}
	}
	return "query {\n  " + SDLFieldName + "\n}\n"
}

// baseTypeName strips list brackets and non-null markers from a
// rendered GraphQL type.
func baseTypeName(rendered string) string {
	out := strings.TrimSuffix(rendered, "!")
	out = strings.TrimSuffix(strings.TrimPrefix(out, "["), "]")
	return strings.TrimSuffix(out, "!")
}

// renderUI builds the complete GraphiQL page.
func renderUI(o Options, query, result, notice string) string {
	var b strings.Builder
	title := o.Title + " GraphQL"

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
	b.WriteString(`<body class="graphiql-container">` + "\n")

	b.WriteString(`  <header class="topbar">` + "\n")
	b.WriteString(`    <h1 class="title">` + html.EscapeString(title) + "</h1>\n")
	b.WriteString(`    <p class="version">Endpoint <code>POST ` + html.EscapeString(o.EndpointPath) + "</code></p>\n")
	if o.SwaggerUIPath != "" {
		b.WriteString(`    <nav class="links"><a href="` + html.EscapeString(o.SwaggerUIPath) + `">Swagger UI</a></nav>` + "\n")
	}
	b.WriteString("  </header>\n")

	b.WriteString(`  <main class="content">` + "\n")
	if notice != "" {
		b.WriteString(`    <p class="notice">` + html.EscapeString(notice) + "</p>\n")
	}
	b.WriteString(`    <p class="intro">This explorer runs on the server. It needs no JavaScript and loads no external asset. Queries submitted here are executed against the same schema the endpoint serves.</p>` + "\n")

	b.WriteString(`    <form class="editor" method="post" action="">` + "\n")
	b.WriteString(`      <label for="query">Query</label>` + "\n")
	b.WriteString(`      <textarea id="query" name="query" rows="12" spellcheck="false" required>` + html.EscapeString(query) + "</textarea>\n")
	b.WriteString(`      <label for="variables">Variables (JSON object, optional)</label>` + "\n")
	b.WriteString(`      <textarea id="variables" name="variables" rows="4" spellcheck="false"></textarea>` + "\n")
	b.WriteString(`      <label for="operationName">Operation name (optional)</label>` + "\n")
	b.WriteString(`      <input id="operationName" name="operationName" type="text">` + "\n")
	b.WriteString(`      <button class="btn execute-button" type="submit">Execute</button>` + "\n")
	b.WriteString("    </form>\n")

	if result != "" {
		b.WriteString(`    <h2 class="section-title">Result</h2>` + "\n")
		b.WriteString(`    <pre class="result-window">` + html.EscapeString(result) + "</pre>\n")
	}

	b.WriteString(`    <h2 class="section-title">Root fields</h2>` + "\n")
	writeFieldList(&b, "Query", o.Schema.Queries)
	writeFieldList(&b, "Mutation", o.Schema.Mutations)

	b.WriteString(`    <h2 class="section-title">Schema</h2>` + "\n")
	b.WriteString("    <details class=\"schema\">\n")
	b.WriteString("      <summary>Show SDL</summary>\n")
	b.WriteString(`      <pre class="result-window">` + html.EscapeString(o.Schema.SDL()) + "</pre>\n")
	b.WriteString("    </details>\n")

	b.WriteString("  </main>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// writeFieldList renders one root type's fields as a definition list.
func writeFieldList(b *strings.Builder, root string, fields []FieldDef) {
	if len(fields) == 0 {
		return
	}
	b.WriteString(`    <h3 class="root-name">` + html.EscapeString(root) + "</h3>\n")
	b.WriteString(`    <dl class="fields">` + "\n")
	for _, f := range fields {
		signature := f.Name
		if len(f.Args) > 0 {
			parts := make([]string, 0, len(f.Args))
			for _, a := range f.Args {
				parts = append(parts, a.Name+": "+a.Type)
			}
			signature += "(" + strings.Join(parts, ", ") + ")"
		}
		signature += ": " + f.Type
		b.WriteString("      <dt><code>" + html.EscapeString(signature) + "</code></dt>\n")
		b.WriteString("      <dd>" + html.EscapeString(f.Description) + "</dd>\n")
	}
	b.WriteString("    </dl>\n")
}
