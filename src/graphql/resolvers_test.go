// Package graphql tests — cover the query parser, the resolver set, and
// the executor.

package graphql

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// testWidget is the value the test resolvers return. Its unexported
// meaning is simple: the executor must narrow it down to the selected
// fields, so the secret field must never reach the response unless it
// was selected.
type testWidget struct {
	ID     string `json:"id"`
	Size   int    `json:"size"`
	Secret string `json:"secret"`
}

// newTestResolvers returns a resolver set answering the widget fields of
// the test schema.
func newTestResolvers(t *testing.T) *Resolvers {
	t.Helper()
	res := NewResolvers()
	if err := res.Register("widgetGet", func(_ context.Context, args map[string]any) (any, error) {
		id, _ := args["id"].(string)
		return testWidget{ID: id, Size: 3, Secret: "hidden"}, nil
	}); err != nil {
		t.Fatalf("Register widgetGet: %v", err)
	}
	if err := res.Register("widgetCreate", func(_ context.Context, _ map[string]any) (any, error) {
		return nil, fmt.Errorf("widgets cannot be created in a test")
	}); err != nil {
		t.Fatalf("Register widgetCreate: %v", err)
	}
	return res
}

func TestParseQuery(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr bool
	}{
		{name: "shorthand", src: `{ widgetGet(id: "a") { id } }`},
		{name: "named query with variables", src: `query One($id: String!) { widgetGet(id: $id) { id } }`},
		{name: "mutation", src: `mutation { widgetCreate(input: {id: "a"}) { id } }`},
		{name: "empty document", src: "   ", wantErr: true},
		{name: "subscription", src: `subscription { widgetGet { id } }`, wantErr: true},
		{name: "fragment", src: `fragment F on Widget { id }`, wantErr: true},
		{name: "unterminated selection set", src: `{ widgetGet(id: "a") { id }`, wantErr: true},
		{name: "unterminated string", src: `{ widgetGet(id: "a) { id } }`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseQuery(tc.src)
			if tc.wantErr && err == nil {
				t.Fatal("ParseQuery() succeeded, want an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ParseQuery() = %v, want success", err)
			}
		})
	}
}

func TestParseQuerySelectionDetail(t *testing.T) {
	doc, err := ParseQuery(`query One($id: String!) { alias: widgetGet(id: $id) { id size } }`)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	op, err := doc.Operation("One")
	if err != nil {
		t.Fatalf("Operation: %v", err)
	}
	if op.Type != OperationQuery {
		t.Errorf("operation type = %q, want %q", op.Type, OperationQuery)
	}
	if len(op.Vars) != 1 || op.Vars[0].Name != "id" {
		t.Fatalf("variable definitions = %+v, want a single id", op.Vars)
	}
	if len(op.Selections) != 1 {
		t.Fatalf("selections = %+v, want one", op.Selections)
	}
	sel := op.Selections[0]
	if sel.Name != "widgetGet" || sel.Key() != "alias" {
		t.Errorf("selection = %q with key %q, want widgetGet with key alias", sel.Name, sel.Key())
	}
	if len(sel.Selections) != 2 {
		t.Errorf("subfields = %+v, want two", sel.Selections)
	}
}

func TestExecuteNarrowsToSelectedFields(t *testing.T) {
	s := newTestSchema(t)
	res := newTestResolvers(t)

	resp := Execute(context.Background(), s, res, Request{Query: `{ widgetGet(id: "w-1") { id } }`})
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
	if _, leaked := widget["secret"]; leaked {
		t.Error("an unselected field reached the response")
	}
	if len(widget) != 1 {
		t.Errorf("selected object = %#v, want only the id field", widget)
	}
}

func TestExecuteWithVariablesAndAlias(t *testing.T) {
	s := newTestSchema(t)
	res := newTestResolvers(t)

	resp := Execute(context.Background(), s, res, Request{
		Query:     `query One($id: String!) { first: widgetGet(id: $id) { id size } }`,
		Variables: map[string]any{"id": "w-2"},
	})
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", resp.Errors)
	}
	widget, ok := resp.Data["first"].(map[string]any)
	if !ok {
		t.Fatalf("data[first] = %#v, want an object", resp.Data["first"])
	}
	if widget["id"] != "w-2" {
		t.Errorf("id = %#v, want %q", widget["id"], "w-2")
	}
	if _, ok := widget["size"]; !ok {
		t.Error("the selected size field is missing from the response")
	}
}

func TestExecuteReportsFieldErrors(t *testing.T) {
	s := newTestSchema(t)
	res := newTestResolvers(t)

	cases := []struct {
		name  string
		req   Request
		field string
	}{
		{
			name:  "unknown field",
			req:   Request{Query: `{ nosuchField }`},
			field: "nosuchField",
		},
		{
			name:  "missing required argument",
			req:   Request{Query: `{ widgetGet { id } }`},
			field: "widgetGet",
		},
		{
			name:  "unknown argument",
			req:   Request{Query: `{ widgetGet(id: "a", colour: "red") { id } }`},
			field: "widgetGet",
		},
		{
			name:  "field with no resolver",
			req:   Request{Query: `{ serverHealthz { status } }`},
			field: "serverHealthz",
		},
		{
			name:  "resolver failure",
			req:   Request{Query: `mutation { widgetCreate(input: {id: "a"}) { id } }`},
			field: "widgetCreate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := Execute(context.Background(), s, res, tc.req)
			if len(resp.Errors) == 0 {
				t.Fatalf("errors = none, want one for %q", tc.field)
			}
			if len(resp.Errors[0].Path) != 1 || resp.Errors[0].Path[0] != tc.field {
				t.Errorf("error path = %v, want [%s]", resp.Errors[0].Path, tc.field)
			}
			if value, ok := resp.Data[tc.field]; !ok || value != nil {
				t.Errorf("data[%s] = %#v, want an explicit null", tc.field, value)
			}
		})
	}
}

func TestExecuteRejectsMalformedQuery(t *testing.T) {
	s := newTestSchema(t)
	res := newTestResolvers(t)

	resp := Execute(context.Background(), s, res, Request{Query: `{ widgetGet(id: "a"`})
	if len(resp.Errors) == 0 {
		t.Fatal("errors = none, want a parse failure")
	}
	if len(resp.Data) != 0 {
		t.Errorf("data = %#v, want none on a parse failure", resp.Data)
	}
}

func TestExecuteServesSDL(t *testing.T) {
	s := newTestSchema(t)
	res := newTestResolvers(t)

	resp := Execute(context.Background(), s, res, Request{Query: "{ " + SDLFieldName + " }"})
	if len(resp.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", resp.Errors)
	}
	sdl, ok := resp.Data[SDLFieldName].(string)
	if !ok {
		t.Fatalf("data[%s] = %#v, want a string", SDLFieldName, resp.Data[SDLFieldName])
	}
	if !strings.Contains(sdl, "type Query {") {
		t.Error("the returned SDL does not describe a query root")
	}
}

func TestResolversRegistration(t *testing.T) {
	res := NewResolvers()
	fn := func(_ context.Context, _ map[string]any) (any, error) { return nil, nil }

	if err := res.Register("", fn); err == nil {
		t.Error("an empty field name was accepted")
	}
	if err := res.Register("widgetGet", nil); err == nil {
		t.Error("a nil resolver was accepted")
	}
	if err := res.Register("widgetGet", fn); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := res.Register("widgetGet", fn); err == nil {
		t.Error("a duplicate field name was accepted")
	}
	if _, ok := res.Lookup("widgetGet"); !ok {
		t.Error("the registered resolver cannot be looked up")
	}
	if _, ok := res.Lookup("widgetCreate"); ok {
		t.Error("an unregistered field resolved")
	}
	if names := res.Names(); len(names) != 1 || names[0] != "widgetGet" {
		t.Errorf("Names() = %v, want [widgetGet]", names)
	}
}

func TestResolversMissingIsSortedAndSkipsSDL(t *testing.T) {
	s := newTestSchema(t)
	res := newTestResolvers(t)

	missing := res.Missing(s)
	if len(missing) == 0 {
		t.Fatal("Missing() = none, want the unimplemented server fields")
	}
	for i := 1; i < len(missing); i++ {
		if missing[i-1] >= missing[i] {
			t.Fatalf("Missing() is not sorted: %v", missing)
		}
	}
	for _, name := range missing {
		if name == SDLFieldName {
			t.Errorf("Missing() lists the built-in %q field", SDLFieldName)
		}
		if name == "widgetGet" || name == "widgetCreate" {
			t.Errorf("Missing() lists %q, which has a resolver", name)
		}
	}
}
