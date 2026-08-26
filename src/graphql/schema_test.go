// Package graphql tests — cover the schema generated from the shared
// annotation registry, including its parity with the REST surface.
//
// Every assertion is deterministic: anything derived from a map is
// sorted before it is compared, because Go map iteration order is
// random.

package graphql

import (
	"sort"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/swagger"
)

// testAPIBase is a deliberately unusual API prefix. PART 14 forbids
// assuming a version, so the tests prove the schema follows whatever
// prefix the caller supplies.
const testAPIBase = "/api/v9"

// newTestRegistry returns a registry holding the fixed server operations
// plus a small object surface with a path parameter and a request body.
func newTestRegistry(t *testing.T) *swagger.Registry {
	t.Helper()
	r := swagger.NewRegistry()
	if err := swagger.RegisterServerOperations(r); err != nil {
		t.Fatalf("RegisterServerOperations: %v", err)
	}
	if err := r.RegisterType(swagger.ObjectType{
		Name:        "Widget",
		Description: "A widget.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindString, Required: true, Description: "Identifier."},
			{Name: "size", Kind: swagger.KindInt, Description: "Size in units."},
		},
	}); err != nil {
		t.Fatalf("RegisterType: %v", err)
	}
	ops := []swagger.Operation{
		{
			ID:           "widget.get",
			Method:       "GET",
			Scope:        swagger.ScopeAPI,
			Path:         "/widget/{id}",
			Summary:      "Fetch a widget",
			Tag:          "widget",
			Auth:         swagger.AuthNone,
			Params:       []swagger.Param{{Name: "id", In: swagger.ParamPath, Kind: swagger.KindString, Required: true, Description: "Identifier."}},
			ResponseKind: swagger.KindObject,
			ResponseType: "Widget",
		},
		{
			ID:           "widget.create",
			Method:       "POST",
			Scope:        swagger.ScopeAPI,
			Path:         "/widget",
			Summary:      "Create a widget",
			Tag:          "widget",
			Auth:         swagger.AuthBearer,
			RequestType:  "Widget",
			ResponseKind: swagger.KindObject,
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

// newTestSchema builds the schema used across these tests.
func newTestSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := BuildSchema(newTestRegistry(t), testAPIBase)
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	return s
}

func TestBuildSchemaMirrorsEveryOperation(t *testing.T) {
	reg := newTestRegistry(t)
	s, err := BuildSchema(reg, testAPIBase)
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}

	want := []string{SDLFieldName}
	for _, op := range reg.Operations() {
		want = append(want, op.GraphQLName())
	}
	sort.Strings(want)

	got := s.FieldNames()
	if len(got) != len(want) {
		t.Fatalf("field names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field names = %v, want %v", got, want)
		}
	}
}

func TestBuildSchemaSplitsQueriesAndMutations(t *testing.T) {
	reg := newTestRegistry(t)
	s, err := BuildSchema(reg, testAPIBase)
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	for _, op := range reg.Operations() {
		root := OperationQuery
		if op.IsMutation() {
			root = OperationMutation
		}
		field, ok := s.Lookup(root, op.GraphQLName())
		if !ok {
			t.Fatalf("operation %q is missing from the %s root", op.ID, root)
		}
		if field.OperationID != op.ID {
			t.Errorf("field %q mirrors %q, want %q", field.Name, field.OperationID, op.ID)
		}
		if field.Method != op.Method {
			t.Errorf("field %q method = %q, want %q", field.Name, field.Method, op.Method)
		}
		if want := op.FullPath(testAPIBase); field.Path != want {
			t.Errorf("field %q path = %q, want %q", field.Name, field.Path, want)
		}
		if _, wrongRoot := s.Lookup(otherRoot(root), op.GraphQLName()); wrongRoot {
			t.Errorf("field %q also appears on the %s root", field.Name, otherRoot(root))
		}
	}
}

// otherRoot returns the root type a field must not appear on.
func otherRoot(root string) string {
	if root == OperationQuery {
		return OperationMutation
	}
	return OperationQuery
}

func TestBuildSchemaArgumentsAndTypes(t *testing.T) {
	s := newTestSchema(t)

	get, ok := s.Lookup(OperationQuery, "widgetGet")
	if !ok {
		t.Fatal("widgetGet is missing from the query root")
	}
	if len(get.Args) != 1 || get.Args[0].Name != "id" || !get.Args[0].Required {
		t.Fatalf("widgetGet args = %+v, want a single required id", get.Args)
	}
	if get.Type != "Widget!" {
		t.Errorf("widgetGet type = %q, want %q", get.Type, "Widget!")
	}

	create, ok := s.Lookup(OperationMutation, "widgetCreate")
	if !ok {
		t.Fatal("widgetCreate is missing from the mutation root")
	}
	found := false
	for _, arg := range create.Args {
		if arg.Name == "input" {
			found = true
			if arg.Type != "Widget"+InputTypeSuffix+"!" {
				t.Errorf("input arg type = %q, want %q", arg.Type, "Widget"+InputTypeSuffix+"!")
			}
		}
	}
	if !found {
		t.Fatalf("widgetCreate args = %+v, want an input argument", create.Args)
	}

	if _, ok := s.Type("Widget"); !ok {
		t.Error("the schema is missing the Widget type")
	}
	input, ok := s.Type("Widget" + InputTypeSuffix)
	if !ok {
		t.Fatal("the schema is missing the Widget input type")
	}
	if !input.Input {
		t.Error("the generated input type is not marked as an input")
	}
}

func TestBuildSchemaRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		reg  *swagger.Registry
		base string
	}{
		{name: "no registry", reg: nil, base: testAPIBase},
		{name: "empty base path", reg: swagger.NewRegistry(), base: ""},
		{name: "relative base path", reg: swagger.NewRegistry(), base: "api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildSchema(tc.reg, tc.base); err == nil {
				t.Fatal("BuildSchema() succeeded, want an error")
			}
		})
	}
}

func TestSchemaSDL(t *testing.T) {
	s := newTestSchema(t)
	sdl := s.SDL()
	for _, want := range []string{
		"schema {",
		"type Query {",
		"type Mutation {",
		"type Widget {",
		"input Widget" + InputTypeSuffix + " {",
		SDLFieldName + ": String!",
		"widgetGet(id: String!): Widget!",
	} {
		if !strings.Contains(sdl, want) {
			t.Errorf("the SDL is missing %q", want)
		}
	}
	if sdl != s.SDL() {
		t.Error("rendering the same schema twice produced different SDL")
	}
}

func TestSchemaCollectionsAreSorted(t *testing.T) {
	s := newTestSchema(t)

	queries := make([]string, 0, len(s.Queries))
	for _, f := range s.Queries {
		queries = append(queries, f.Name)
	}
	if !sort.StringsAreSorted(queries) {
		t.Errorf("queries are not sorted: %v", queries)
	}

	mutations := make([]string, 0, len(s.Mutations))
	for _, f := range s.Mutations {
		mutations = append(mutations, f.Name)
	}
	if !sort.StringsAreSorted(mutations) {
		t.Errorf("mutations are not sorted: %v", mutations)
	}

	types := make([]string, 0, len(s.Types))
	for _, tp := range s.Types {
		types = append(types, tp.Name)
	}
	if !sort.StringsAreSorted(types) {
		t.Errorf("types are not sorted: %v", types)
	}
}
