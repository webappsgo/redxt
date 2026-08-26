// Package graphql implements the GraphQL surface required by AI.md
// PART 14 (API TYPES — REST, Swagger, GraphQL).
//
// PART 14 forbids a hand-written schema and requires that GraphQL never
// drifts from REST or from Swagger. This file therefore derives the
// whole schema from the annotation registry in src/swagger, the same
// registry the OpenAPI document is generated from: every registered
// operation becomes a query field when it is safe and a mutation field
// when it changes state, and every registered object type becomes a
// GraphQL type. A route cannot appear in one surface and be missing
// from the other, because neither surface has a source of its own.
package graphql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/webappsgo/redxt/src/swagger"
)

// Operation type names, used when looking a field up.
const (
	// OperationQuery selects the read-only root type.
	OperationQuery = "query"
	// OperationMutation selects the state-changing root type.
	OperationMutation = "mutation"
)

// SDLFieldName is the built-in query field that returns this server's
// schema as SDL text. It gives clients and the GraphiQL page a way to
// read the schema without a full introspection implementation, and it
// cannot collide with a generated field name because a generated name
// never begins with an underscore.
const SDLFieldName = "_sdl"

// InputTypeSuffix is appended to an object type's name when it is
// emitted as a GraphQL input type for a request body.
const InputTypeSuffix = "Input"

// ArgDef is one argument of a schema field.
type ArgDef struct {
	// Name is the argument name.
	Name string
	// Type is the rendered GraphQL type, including any non-null marker.
	Type string
	// Description documents the argument.
	Description string
	// Required reports whether the argument may be omitted.
	Required bool
}

// FieldDef is one root field of the schema, derived from exactly one
// registered REST operation.
type FieldDef struct {
	// Name is the GraphQL field name.
	Name string
	// Description documents the field.
	Description string
	// Args are the field's arguments, sorted by name.
	Args []ArgDef
	// Type is the rendered GraphQL result type.
	Type string
	// OperationID is the REST operation this field mirrors.
	OperationID string
	// Method is the HTTP method of the mirrored operation.
	Method string
	// Path is the resolved URL of the mirrored operation.
	Path string
	// Mutation reports whether the field lives on the mutation root.
	Mutation bool
}

// TypeField is one member of a schema type.
type TypeField struct {
	// Name is the field name.
	Name string
	// Type is the rendered GraphQL type.
	Type string
	// Description documents the field.
	Description string
}

// TypeDef is one object or input type of the schema.
type TypeDef struct {
	// Name is the GraphQL type name.
	Name string
	// Description documents the type.
	Description string
	// Input marks an input type rather than an output type.
	Input bool
	// Fields are the type's members, sorted by name.
	Fields []TypeField
}

// Schema is the generated GraphQL schema.
type Schema struct {
	// Queries are the read-only root fields, sorted by name.
	Queries []FieldDef
	// Mutations are the state-changing root fields, sorted by name.
	Mutations []FieldDef
	// Types are the object and input types, sorted by name.
	Types []TypeDef
	// Scalars are the custom scalar names in use, sorted.
	Scalars []string
}

// BuildSchema derives the schema from a swagger annotation registry.
//
// apiBasePath is the versioned API prefix supplied by the caller, for
// example the value returned by the config package's APIBasePath. This
// package never assumes an API version.
func BuildSchema(reg *swagger.Registry, apiBasePath string) (*Schema, error) {
	if reg == nil {
		return nil, fmt.Errorf("graphql: a registry is required")
	}
	if !strings.HasPrefix(apiBasePath, "/") {
		return nil, fmt.Errorf("graphql: API base path %q must start with a slash", apiBasePath)
	}
	if err := reg.Validate(); err != nil {
		return nil, err
	}

	s := &Schema{}
	usesJSON := false

	inputNeeded := make(map[string]bool)
	for _, op := range reg.Operations() {
		if op.RequestType != "" {
			inputNeeded[op.RequestType] = true
		}
	}

	for _, t := range reg.Types() {
		def := TypeDef{Name: t.Name, Description: t.Description}
		for _, f := range t.Fields {
			if f.Kind == swagger.KindJSON {
				usesJSON = true
			}
			def.Fields = append(def.Fields, TypeField{
				Name:        f.Name,
				Type:        renderType(f.Kind, f.Ref, f.List, f.Required),
				Description: f.Description,
			})
		}
		sort.Slice(def.Fields, func(i, j int) bool { return def.Fields[i].Name < def.Fields[j].Name })
		s.Types = append(s.Types, def)

		if !inputNeeded[t.Name] {
			continue
		}
		in := TypeDef{
			Name:        t.Name + InputTypeSuffix,
			Description: t.Description,
			Input:       true,
		}
		for _, f := range def.Fields {
			in.Fields = append(in.Fields, TypeField{
				Name:        f.Name,
				Type:        inputTypeName(f.Type, inputNeeded),
				Description: f.Description,
			})
		}
		s.Types = append(s.Types, in)
	}

	for _, op := range reg.Operations() {
		field := FieldDef{
			Name:        op.GraphQLName(),
			Description: fieldDescription(op),
			Type:        responseType(op),
			OperationID: op.ID,
			Method:      op.Method,
			Path:        op.FullPath(apiBasePath),
			Mutation:    op.IsMutation(),
		}
		if op.ResponseKind == swagger.KindJSON {
			usesJSON = true
		}
		for _, p := range op.Params {
			required := p.Required || p.In == swagger.ParamPath
			field.Args = append(field.Args, ArgDef{
				Name:        p.Name,
				Type:        renderType(p.Kind, "", false, required),
				Description: p.Description,
				Required:    required,
			})
		}
		if op.RequestType != "" {
			field.Args = append(field.Args, ArgDef{
				Name:        "input",
				Type:        op.RequestType + InputTypeSuffix + "!",
				Description: "Request payload.",
				Required:    true,
			})
		}
		sort.Slice(field.Args, func(i, j int) bool { return field.Args[i].Name < field.Args[j].Name })

		if field.Mutation {
			s.Mutations = append(s.Mutations, field)
		} else {
			s.Queries = append(s.Queries, field)
		}
	}

	s.Queries = append(s.Queries, FieldDef{
		Name:        SDLFieldName,
		Description: "This server's schema rendered as SDL text.",
		Type:        "String!",
	})

	sort.Slice(s.Queries, func(i, j int) bool { return s.Queries[i].Name < s.Queries[j].Name })
	sort.Slice(s.Mutations, func(i, j int) bool { return s.Mutations[i].Name < s.Mutations[j].Name })
	sort.Slice(s.Types, func(i, j int) bool { return s.Types[i].Name < s.Types[j].Name })

	if usesJSON {
		s.Scalars = append(s.Scalars, swagger.ScalarJSON)
	}
	return s, nil
}

// fieldDescription builds the field documentation, always naming the
// REST route the field mirrors so the two surfaces stay traceable to
// each other.
func fieldDescription(op swagger.Operation) string {
	desc := op.Summary
	if op.Description != "" {
		desc = op.Description
	}
	return desc + " Mirrors " + op.Method + " " + op.Path + "."
}

// responseType renders the GraphQL result type of an operation.
func responseType(op swagger.Operation) string {
	if op.ResponseKind == swagger.KindObject {
		return renderType(swagger.KindObject, op.ResponseType, op.ResponseList, true)
	}
	return renderType(op.ResponseKind, "", op.ResponseList, true)
}

// renderType renders one GraphQL type reference. A list is always a
// list of non-null elements; the required flag applies to the outermost
// type.
func renderType(kind swagger.Kind, ref string, list, required bool) string {
	base := kind.GraphQLType()
	if kind == swagger.KindObject {
		base = ref
	}
	out := base
	if list {
		out = "[" + base + "!]"
	}
	if required {
		out += "!"
	}
	return out
}

// inputTypeName rewrites an output type reference into its input
// counterpart when the referenced type also exists as an input type.
func inputTypeName(rendered string, inputNeeded map[string]bool) string {
	trimmed := strings.TrimSuffix(rendered, "!")
	listed := false
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		listed = true
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
		trimmed = strings.TrimSuffix(trimmed, "!")
	}
	if !inputNeeded[trimmed] {
		return rendered
	}
	out := trimmed + InputTypeSuffix
	if listed {
		out = "[" + out + "!]"
	}
	if strings.HasSuffix(rendered, "!") {
		out += "!"
	}
	return out
}

// Lookup returns a root field by operation type and name.
func (s *Schema) Lookup(operation, name string) (FieldDef, bool) {
	fields := s.Queries
	if operation == OperationMutation {
		fields = s.Mutations
	}
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return FieldDef{}, false
}

// FieldNames returns every root field name, query and mutation, sorted.
func (s *Schema) FieldNames() []string {
	out := make([]string, 0, len(s.Queries)+len(s.Mutations))
	for _, f := range s.Queries {
		out = append(out, f.Name)
	}
	for _, f := range s.Mutations {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

// Type returns one schema type by name.
func (s *Schema) Type(name string) (TypeDef, bool) {
	for _, t := range s.Types {
		if t.Name == name {
			return t, true
		}
	}
	return TypeDef{}, false
}

// SDL renders the schema as GraphQL schema definition language.
//
// The output is deterministic: every collection was sorted when the
// schema was built, so the same registry always produces byte-identical
// SDL.
func (s *Schema) SDL() string {
	var b strings.Builder
	b.WriteString("schema {\n  query: Query\n")
	if len(s.Mutations) > 0 {
		b.WriteString("  mutation: Mutation\n")
	}
	b.WriteString("}\n")

	for _, name := range s.Scalars {
		b.WriteString("\nscalar " + name + "\n")
	}

	b.WriteString("\ntype Query {\n")
	for _, f := range s.Queries {
		writeSDLField(&b, f)
	}
	b.WriteString("}\n")

	if len(s.Mutations) > 0 {
		b.WriteString("\ntype Mutation {\n")
		for _, f := range s.Mutations {
			writeSDLField(&b, f)
		}
		b.WriteString("}\n")
	}

	for _, t := range s.Types {
		keyword := "type"
		if t.Input {
			keyword = "input"
		}
		b.WriteString("\n")
		if t.Description != "" {
			b.WriteString("\"" + sdlText(t.Description) + "\"\n")
		}
		b.WriteString(keyword + " " + t.Name + " {\n")
		for _, f := range t.Fields {
			if f.Description != "" {
				b.WriteString("  \"" + sdlText(f.Description) + "\"\n")
			}
			b.WriteString("  " + f.Name + ": " + f.Type + "\n")
		}
		b.WriteString("}\n")
	}
	return b.String()
}

// writeSDLField renders one root field, its arguments, and its doc
// string.
func writeSDLField(b *strings.Builder, f FieldDef) {
	if f.Description != "" {
		b.WriteString("  \"" + sdlText(f.Description) + "\"\n")
	}
	b.WriteString("  " + f.Name)
	if len(f.Args) > 0 {
		parts := make([]string, 0, len(f.Args))
		for _, a := range f.Args {
			parts = append(parts, a.Name+": "+a.Type)
		}
		b.WriteString("(" + strings.Join(parts, ", ") + ")")
	}
	b.WriteString(": " + f.Type + "\n")
}

// sdlText makes a description safe to embed in an SDL string literal by
// collapsing newlines and escaping quotes and backslashes.
func sdlText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
