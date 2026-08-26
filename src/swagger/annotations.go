// Package swagger implements the OpenAPI/Swagger surface required by
// AI.md PART 14 (API TYPES — REST, Swagger, GraphQL).
//
// This file holds the annotation layer: the in-code registry that every
// route declares itself into. PART 14 forbids hand-written OpenAPI JSON
// and hand-written GraphQL schemas, and requires that REST, Swagger and
// GraphQL never drift apart. The registry in this file is the single
// source both generators read: src/swagger builds the OpenAPI document
// from it and src/graphql builds the GraphQL schema from the very same
// operations, so a route that exists in one necessarily exists in all
// three.
//
// Nothing here performs I/O and nothing here imports the router, the
// config, or the startup package. The router calls Register for each of
// its routes and hands the registry to the handler constructors.
package swagger

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Kind is the transport-neutral type of a value. Each Kind knows how to
// render itself as an OpenAPI type and as a GraphQL type, which is what
// keeps the two documents describing the same thing.
type Kind string

const (
	// KindString is a UTF-8 text value.
	KindString Kind = "string"
	// KindInt is a 64-bit signed integer value.
	KindInt Kind = "int"
	// KindFloat is a double precision floating point value.
	KindFloat Kind = "float"
	// KindBool is a boolean value.
	KindBool Kind = "bool"
	// KindTime is an RFC 3339 timestamp carried as a string.
	KindTime Kind = "time"
	// KindJSON is a free-form JSON document with no fixed shape.
	KindJSON Kind = "json"
	// KindObject is a reference to a named object type in the registry.
	KindObject Kind = "object"
)

// Valid reports whether k is one of the defined kinds.
func (k Kind) Valid() bool {
	switch k {
	case KindString, KindInt, KindFloat, KindBool, KindTime, KindJSON, KindObject:
		return true
	default:
		return false
	}
}

// OpenAPIType returns the JSON Schema type and format for a kind.
// KindObject has no intrinsic type because it always renders as a $ref.
func (k Kind) OpenAPIType() (string, string) {
	switch k {
	case KindInt:
		return "integer", "int64"
	case KindFloat:
		return "number", "double"
	case KindBool:
		return "boolean", ""
	case KindTime:
		return "string", "date-time"
	case KindJSON:
		return "object", ""
	case KindObject:
		return "", ""
	default:
		return "string", ""
	}
}

// GraphQLType returns the GraphQL named type for a kind. KindObject
// returns an empty string because the caller substitutes the referenced
// object type name.
func (k Kind) GraphQLType() string {
	switch k {
	case KindInt:
		return "Int"
	case KindFloat:
		return "Float"
	case KindBool:
		return "Boolean"
	case KindJSON:
		return ScalarJSON
	case KindObject:
		return ""
	default:
		return "String"
	}
}

// ScalarJSON is the name of the custom GraphQL scalar used for
// free-form JSON payloads that have no fixed object shape.
const ScalarJSON = "JSON"

// Scope says how an operation's Path is turned into a URL.
type Scope string

const (
	// ScopeAPI prefixes Path with the caller-supplied API base path, so
	// that the API version segment is never hardcoded in this package.
	ScopeAPI Scope = "api"
	// ScopeRoot uses Path verbatim, for the unversioned root endpoints
	// listed in the PART 14 root-level endpoint table.
	ScopeRoot Scope = "root"
)

// Valid reports whether s is one of the defined scopes.
func (s Scope) Valid() bool {
	return s == ScopeAPI || s == ScopeRoot
}

// Auth names the credential an operation requires.
type Auth string

const (
	// AuthNone marks a public endpoint.
	AuthNone Auth = "none"
	// AuthBearer marks an endpoint requiring an API bearer token.
	AuthBearer Auth = "bearer"
	// AuthSession marks an endpoint requiring a browser session cookie.
	AuthSession Auth = "session"
)

// Valid reports whether a is one of the defined auth modes.
func (a Auth) Valid() bool {
	return a == AuthNone || a == AuthBearer || a == AuthSession
}

// SecuritySchemeName returns the components.securitySchemes key for an
// auth mode, or an empty string for a public endpoint.
func (a Auth) SecuritySchemeName() string {
	switch a {
	case AuthBearer:
		return "bearerAuth"
	case AuthSession:
		return "sessionAuth"
	default:
		return ""
	}
}

// Field describes one property of an object type, one member of a
// request body, or one element of a response payload.
type Field struct {
	// Name is the wire name, used verbatim as the JSON property name
	// and as the GraphQL field name.
	Name string
	// Kind is the field's transport-neutral type.
	Kind Kind
	// Ref names the object type when Kind is KindObject.
	Ref string
	// List marks the field as a list of Kind rather than a single value.
	List bool
	// Required marks the field as always present (responses) or
	// mandatory (requests).
	Required bool
	// Description is the human-readable documentation for the field.
	Description string
}

// ObjectType is a named structure shared by the OpenAPI components
// section and the GraphQL type system.
type ObjectType struct {
	// Name is the type name, used as the OpenAPI schema key and as the
	// GraphQL type name, so it must be a valid identifier.
	Name string
	// Description documents the type in both outputs.
	Description string
	// Input marks a type that only ever appears in request bodies, so
	// that GraphQL renders it as an input type.
	Input bool
	// Fields are the type's properties.
	Fields []Field
}

// Param describes a path, query, or header parameter.
type Param struct {
	// Name is the parameter name.
	Name string
	// In is ParamPath, ParamQuery, or ParamHeader.
	In string
	// Kind is the parameter's scalar type; KindObject is not allowed.
	Kind Kind
	// Required marks a mandatory parameter. Path parameters are always
	// treated as required regardless of this field.
	Required bool
	// Description documents the parameter.
	Description string
}

// Parameter locations.
const (
	// ParamPath is a parameter interpolated into the URL path.
	ParamPath = "path"
	// ParamQuery is a URL query string parameter.
	ParamQuery = "query"
	// ParamHeader is an HTTP request header.
	ParamHeader = "header"
)

// Operation is one REST endpoint, one OpenAPI operation, and one
// GraphQL field, declared once.
type Operation struct {
	// ID is the stable operation identifier, for example
	// "server.healthz". It becomes the OpenAPI operationId and, camel
	// cased, the GraphQL field name.
	ID string
	// Method is the uppercase HTTP method.
	Method string
	// Scope decides whether Path is prefixed with the API base path.
	Scope Scope
	// Path is the route path, with {name} placeholders for path
	// parameters. For ScopeAPI it is relative to the API base path.
	Path string
	// Summary is the one-line description shown in listings.
	Summary string
	// Description is the long-form documentation.
	Description string
	// Tag groups related operations in the UI. Defaults to "default".
	Tag string
	// Auth is the credential the endpoint requires.
	Auth Auth
	// Params are the path, query, and header parameters.
	Params []Param
	// RequestType names the object type carried in the request body.
	// An empty value means the operation takes no body.
	RequestType string
	// ResponseKind is the kind of the success payload. KindObject uses
	// ResponseType; KindJSON is a free-form document.
	ResponseKind Kind
	// ResponseType names the object type of the success payload when
	// ResponseKind is KindObject.
	ResponseType string
	// ResponseList marks a success payload that is a list.
	ResponseList bool
	// ResponseContentType overrides the success media type. Empty means
	// application/json.
	ResponseContentType string
}

// DefaultTag is the tag applied to operations that declare none.
const DefaultTag = "default"

// TagOrDefault returns the operation's tag, falling back to DefaultTag.
func (o Operation) TagOrDefault() string {
	if o.Tag == "" {
		return DefaultTag
	}
	return o.Tag
}

// ContentTypeOrDefault returns the success media type for an operation.
func (o Operation) ContentTypeOrDefault() string {
	if o.ResponseContentType == "" {
		return "application/json"
	}
	return o.ResponseContentType
}

// IsMutation reports whether the operation changes state, which decides
// whether GraphQL exposes it as a mutation rather than a query.
func (o Operation) IsMutation() bool {
	return o.Method != "GET" && o.Method != "HEAD"
}

// GraphQLName returns the camel cased GraphQL field name derived from
// the operation ID, so that the REST and GraphQL surfaces cannot name
// the same operation differently.
func (o Operation) GraphQLName() string {
	return graphQLName(o.ID)
}

// FullPath returns the operation's URL. apiBase is the versioned API
// prefix supplied by the caller, for example the value of the config
// package's APIBasePath. The version segment is never assumed here.
func (o Operation) FullPath(apiBase string) string {
	if o.Scope == ScopeRoot {
		return o.Path
	}
	return strings.TrimSuffix(apiBase, "/") + o.Path
}

// httpMethods is the set of methods an operation may declare. OpenAPI
// path items have no field for any other method.
var httpMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"PATCH":   true,
	"DELETE":  true,
	"HEAD":    true,
	"OPTIONS": true,
}

// Registry holds every declared operation and object type. It is safe
// for concurrent use so that route packages can register during their
// own initialisation while a handler reads.
type Registry struct {
	mu    sync.RWMutex
	ops   map[string]Operation
	types map[string]ObjectType
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		ops:   make(map[string]Operation),
		types: make(map[string]ObjectType),
	}
}

// defaultRegistry is the process-wide registry used when a caller does
// not want to thread its own instance through the router.
var defaultRegistry = NewRegistry()

// Default returns the process-wide registry.
func Default() *Registry {
	return defaultRegistry
}

// Register adds an operation. It fails on a malformed operation, on a
// duplicate ID, and on a duplicate method plus path pair.
func (r *Registry) Register(op Operation) error {
	if err := validateOperation(op); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.ops[op.ID]; ok {
		return fmt.Errorf("swagger: operation %q already registered", op.ID)
	}
	name := op.GraphQLName()
	for _, existing := range r.ops {
		if existing.Method == op.Method && existing.Scope == op.Scope && existing.Path == op.Path {
			return fmt.Errorf("swagger: %s %s already registered as %q", op.Method, op.Path, existing.ID)
		}
		if existing.GraphQLName() == name {
			return fmt.Errorf("swagger: operation %q collides with %q on GraphQL field %q", op.ID, existing.ID, name)
		}
	}
	r.ops[op.ID] = op
	return nil
}

// MustRegister adds an operation and panics if it is invalid. A bad
// registration is a programming error in a route declaration, caught on
// the first run rather than served as a broken document.
func (r *Registry) MustRegister(op Operation) {
	if err := r.Register(op); err != nil {
		panic(err)
	}
}

// RegisterType adds an object type. It fails on a malformed type and on
// a duplicate name.
func (r *Registry) RegisterType(t ObjectType) error {
	if err := validateType(t); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.types[t.Name]; ok {
		return fmt.Errorf("swagger: type %q already registered", t.Name)
	}
	r.types[t.Name] = t
	return nil
}

// MustRegisterType adds an object type and panics if it is invalid.
func (r *Registry) MustRegisterType(t ObjectType) {
	if err := r.RegisterType(t); err != nil {
		panic(err)
	}
}

// Operations returns every operation sorted by ID. Sorting matters: Go
// map iteration order is random, and both generated documents must be
// byte-for-byte reproducible across runs.
func (r *Registry) Operations() []Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Operation, 0, len(r.ops))
	for _, op := range r.ops {
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Operation returns one operation by ID.
func (r *Registry) Operation(id string) (Operation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.ops[id]
	return op, ok
}

// Types returns every object type sorted by name.
func (r *Registry) Types() []ObjectType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ObjectType, 0, len(r.types))
	for _, t := range r.types {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Type returns one object type by name.
func (r *Registry) Type(name string) (ObjectType, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.types[name]
	return t, ok
}

// Len returns the number of registered operations.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.ops)
}

// Validate checks that every type reference resolves and that every
// path parameter in a route has a matching declared parameter. Both
// generators call it before emitting a document, so an incomplete
// registration can never reach a client.
func (r *Registry) Validate() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.types))
	for name := range r.types {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := r.types[name]
		for _, f := range t.Fields {
			if f.Kind != KindObject {
				continue
			}
			if _, ok := r.types[f.Ref]; !ok {
				return fmt.Errorf("swagger: type %q field %q references unknown type %q", t.Name, f.Name, f.Ref)
			}
		}
	}

	ids := make([]string, 0, len(r.ops))
	for id := range r.ops {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		op := r.ops[id]
		if op.RequestType != "" {
			if _, ok := r.types[op.RequestType]; !ok {
				return fmt.Errorf("swagger: operation %q references unknown request type %q", op.ID, op.RequestType)
			}
		}
		if op.ResponseKind == KindObject {
			if _, ok := r.types[op.ResponseType]; !ok {
				return fmt.Errorf("swagger: operation %q references unknown response type %q", op.ID, op.ResponseType)
			}
		}
		declared := make(map[string]bool, len(op.Params))
		for _, p := range op.Params {
			if p.In == ParamPath {
				declared[p.Name] = true
			}
		}
		for _, want := range pathPlaceholders(op.Path) {
			if !declared[want] {
				return fmt.Errorf("swagger: operation %q path parameter %q is not declared", op.ID, want)
			}
		}
	}
	return nil
}

// validateOperation checks a single operation in isolation.
func validateOperation(op Operation) error {
	if !identifier(op.ID) {
		return fmt.Errorf("swagger: operation ID %q must be a non-empty [A-Za-z0-9._-] string", op.ID)
	}
	if !httpMethods[op.Method] {
		return fmt.Errorf("swagger: operation %q has unsupported method %q", op.ID, op.Method)
	}
	if !op.Scope.Valid() {
		return fmt.Errorf("swagger: operation %q has invalid scope %q", op.ID, op.Scope)
	}
	if !strings.HasPrefix(op.Path, "/") {
		return fmt.Errorf("swagger: operation %q path %q must start with a slash", op.ID, op.Path)
	}
	if !op.Auth.Valid() {
		return fmt.Errorf("swagger: operation %q has invalid auth %q", op.ID, op.Auth)
	}
	if op.Summary == "" {
		return fmt.Errorf("swagger: operation %q needs a summary", op.ID)
	}
	if !op.ResponseKind.Valid() {
		return fmt.Errorf("swagger: operation %q has invalid response kind %q", op.ID, op.ResponseKind)
	}
	if op.ResponseKind == KindObject && op.ResponseType == "" {
		return fmt.Errorf("swagger: operation %q has an object response with no response type", op.ID)
	}
	if op.ResponseKind != KindObject && op.ResponseType != "" {
		return fmt.Errorf("swagger: operation %q sets a response type but its kind is %q", op.ID, op.ResponseKind)
	}
	if op.GraphQLName() == "" {
		return fmt.Errorf("swagger: operation %q produces an empty GraphQL field name", op.ID)
	}
	seen := make(map[string]bool, len(op.Params))
	for _, p := range op.Params {
		if !identifier(p.Name) {
			return fmt.Errorf("swagger: operation %q has an invalid parameter name %q", op.ID, p.Name)
		}
		if seen[p.Name] {
			return fmt.Errorf("swagger: operation %q declares parameter %q twice", op.ID, p.Name)
		}
		seen[p.Name] = true
		if p.In != ParamPath && p.In != ParamQuery && p.In != ParamHeader {
			return fmt.Errorf("swagger: operation %q parameter %q has invalid location %q", op.ID, p.Name, p.In)
		}
		if !p.Kind.Valid() || p.Kind == KindObject || p.Kind == KindJSON {
			return fmt.Errorf("swagger: operation %q parameter %q must be a scalar kind, got %q", op.ID, p.Name, p.Kind)
		}
	}
	return nil
}

// validateType checks a single object type in isolation.
func validateType(t ObjectType) error {
	if !typeName(t.Name) {
		return fmt.Errorf("swagger: type name %q must start with a letter and contain only letters and digits", t.Name)
	}
	if len(t.Fields) == 0 {
		return fmt.Errorf("swagger: type %q has no fields", t.Name)
	}
	seen := make(map[string]bool, len(t.Fields))
	for _, f := range t.Fields {
		if !identifier(f.Name) {
			return fmt.Errorf("swagger: type %q has an invalid field name %q", t.Name, f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("swagger: type %q declares field %q twice", t.Name, f.Name)
		}
		seen[f.Name] = true
		if !f.Kind.Valid() {
			return fmt.Errorf("swagger: type %q field %q has invalid kind %q", t.Name, f.Name, f.Kind)
		}
		if f.Kind == KindObject && f.Ref == "" {
			return fmt.Errorf("swagger: type %q field %q is an object with no referenced type", t.Name, f.Name)
		}
		if f.Kind != KindObject && f.Ref != "" {
			return fmt.Errorf("swagger: type %q field %q sets a ref but its kind is %q", t.Name, f.Name, f.Kind)
		}
	}
	return nil
}

// identifier reports whether s is a non-empty ASCII identifier made of
// letters, digits, dot, underscore, or hyphen.
func identifier(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// typeName reports whether s is usable as both an OpenAPI schema key
// and a GraphQL type name.
func typeName(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// graphQLName camel cases a dotted, dashed, or underscored operation ID
// into a GraphQL field name.
func graphQLName(id string) string {
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '/'
	})
	var b strings.Builder
	first := true
	for _, p := range parts {
		if p == "" {
			continue
		}
		if first {
			b.WriteString(strings.ToLower(p))
			first = false
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(strings.ToLower(p[1:]))
	}
	return b.String()
}

// pathPlaceholders returns the {name} placeholders found in a path.
func pathPlaceholders(path string) []string {
	var out []string
	rest := path
	for {
		open := strings.Index(rest, "{")
		if open < 0 {
			return out
		}
		rest = rest[open+1:]
		closed := strings.Index(rest, "}")
		if closed < 0 {
			return out
		}
		name := rest[:closed]
		if name != "" {
			out = append(out, name)
		}
		rest = rest[closed+1:]
	}
}

// RegisterServerOperations declares the fixed server endpoints from the
// PART 14 root-level endpoint table. Their contract is set by the spec
// rather than by business logic, so they live here and every project
// route package registers the rest of the surface itself.
//
// The Swagger and GraphiQL transports are deliberately absent: they
// carry the documentation, they are not part of the documented API
// surface, and listing them would make GraphQL describe itself.
func RegisterServerOperations(r *Registry) error {
	types := []ObjectType{
		{
			Name:        "HealthStatus",
			Description: "Public health document served without authentication.",
			Fields: []Field{
				{Name: "status", Kind: KindString, Required: true, Description: "healthy, degraded, unhealthy, restart_required, maintenance, or shutting_down."},
				{Name: "version", Kind: KindString, Required: true, Description: "SemVer application version."},
				{Name: "go_version", Kind: KindString, Required: true, Description: "Go runtime version of this build."},
				{Name: "uptime", Kind: KindString, Required: true, Description: "Human readable uptime, for example 2d 5h 30m."},
				{Name: "mode", Kind: KindString, Required: true, Description: "Resolved application mode."},
				{Name: "timestamp", Kind: KindTime, Required: true, Description: "Current server time in UTC."},
			},
		},
		{
			Name:        "AutodiscoverInfo",
			Description: "Server settings a client or agent needs before it picks an API version.",
			Fields: []Field{
				{Name: "name", Kind: KindString, Required: true, Description: "Branding title of this instance."},
				{Name: "tagline", Kind: KindString, Description: "Short branding slogan."},
				{Name: "api_version", Kind: KindString, Required: true, Description: "Current API version segment."},
				{Name: "api_base_path", Kind: KindString, Required: true, Description: "Versioned API prefix for this instance."},
				{Name: "health_path", Kind: KindString, Required: true, Description: "Path of the unauthenticated health endpoint."},
				{Name: "swagger_path", Kind: KindString, Required: true, Description: "Path of the OpenAPI JSON document."},
				{Name: "graphql_path", Kind: KindString, Required: true, Description: "Path that accepts GraphQL queries."},
			},
		},
	}
	for _, t := range types {
		if err := r.RegisterType(t); err != nil {
			return err
		}
	}

	ops := []Operation{
		{
			ID:           "server.healthz",
			Method:       "GET",
			Scope:        ScopeAPI,
			Path:         "/server/healthz",
			Summary:      "Health check",
			Description:  "Reports overall status, build identity, and per-component checks. Requires no authentication and exposes no internal detail.",
			Tag:          "server",
			Auth:         AuthNone,
			ResponseKind: KindObject,
			ResponseType: "HealthStatus",
		},
		{
			ID:           "server.autodiscover",
			Method:       "GET",
			Scope:        ScopeRoot,
			Path:         "/api/autodiscover",
			Summary:      "Client autodiscovery",
			Description:  "Returns the server identity and the paths a client or agent needs before it has chosen an API version. Deliberately unversioned.",
			Tag:          "server",
			Auth:         AuthNone,
			ResponseKind: KindObject,
			ResponseType: "AutodiscoverInfo",
		},
		{
			ID:                  "server.metrics",
			Method:              "GET",
			Scope:               ScopeAPI,
			Path:                "/server/metrics",
			Summary:             "Prometheus metrics",
			Description:         "Prometheus exposition of every metric category. Requires a bearer token.",
			Tag:                 "server",
			Auth:                AuthBearer,
			ResponseKind:        KindString,
			ResponseContentType: "text/plain",
		},
		{
			ID:          "server.metrics.service",
			Method:      "GET",
			Scope:       ScopeAPI,
			Path:        "/server/metrics/{service}",
			Summary:     "Prometheus metrics for one service",
			Description: "Prometheus exposition limited to a single service. Requires the bearer token configured for that service.",
			Tag:         "server",
			Auth:        AuthBearer,
			Params: []Param{
				{Name: "service", In: ParamPath, Kind: KindString, Required: true, Description: "Service whose metrics are returned."},
			},
			ResponseKind:        KindString,
			ResponseContentType: "text/plain",
		},
	}
	for _, op := range ops {
		if err := r.Register(op); err != nil {
			return err
		}
	}
	return nil
}
