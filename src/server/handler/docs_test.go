package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/graphql"
	"github.com/webappsgo/redxt/src/swagger"
)

// documented builds a registry holding the fixed server endpoints plus
// this package's own surface, which is what a running server publishes.
func documented(t *testing.T) *swagger.Registry {
	t.Helper()

	reg := swagger.NewRegistry()
	if err := swagger.RegisterServerOperations(reg); err != nil {
		t.Fatalf("register server operations: %v", err)
	}
	if err := RegisterAPIOperations(reg); err != nil {
		t.Fatalf("register api operations: %v", err)
	}
	return reg
}

func TestTheDocumentedSurfaceIsWellFormed(t *testing.T) {
	reg := documented(t)

	if err := reg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got := reg.Len(); got < len(apiRoutes) {
		t.Fatalf("registry holds %d operations, want at least the %d routes served", got, len(apiRoutes))
	}
}

func TestEveryServedRouteIsDocumented(t *testing.T) {
	reg := documented(t)

	for _, rt := range apiRoutes {
		if _, ok := reg.Operation(rt.op.ID); !ok {
			t.Errorf("route %s %s is served but absent from the registry", rt.op.Method, rt.op.Path)
		}
	}
}

func TestEveryDocumentedRouteIsServed(t *testing.T) {
	ts := newTestServer(t)
	base := ts.handler.apiBase()

	// The mux is asked which pattern a request matches rather than being
	// asked to serve it. A served request cannot answer the question: a
	// mounted handler legitimately reports not found for a record that
	// does not exist, which is indistinguishable from the catch-all.
	mux, ok := ts.handler.API().(*http.ServeMux)
	if !ok {
		t.Fatal("API does not return a mux, so its patterns cannot be inspected")
	}

	for _, rt := range apiRoutes {
		path := strings.NewReplacer("{", "", "}", "").Replace(rt.op.FullPath(base))
		req := httptest.NewRequest(rt.op.Method, path, nil)

		if _, pattern := mux.Handler(req); pattern == "/" {
			t.Errorf("documented route %s %s falls through to the catch-all", rt.op.Method, rt.op.Path)
		}
	}
}

func TestOperationIdentifiersAreUnique(t *testing.T) {
	seen := make(map[string]bool, len(apiRoutes))

	for _, rt := range apiRoutes {
		if seen[rt.op.ID] {
			t.Errorf("operation id %q is declared twice", rt.op.ID)
		}
		seen[rt.op.ID] = true
	}
}

func TestRoutesFollowThePathRules(t *testing.T) {
	for _, rt := range apiRoutes {
		path := rt.op.Path

		if path != strings.ToLower(path) {
			t.Errorf("route %s is not lowercase", path)
		}
		if strings.HasSuffix(path, "/") {
			t.Errorf("route %s ends with a trailing slash", path)
		}
		if strings.Contains(path, "_") && !strings.Contains(path, "{") {
			t.Errorf("route %s uses an underscore where a hyphen belongs", path)
		}
		if rt.op.Scope != swagger.ScopeAPI {
			t.Errorf("route %s is not in the versioned api scope", path)
		}
	}
}

func TestCredentialChangingRoutesRefuseTokens(t *testing.T) {
	// A route that changes how the account authenticates must demand a
	// browser session. Were a token enough, a leaked token could rotate
	// the password or the second factor and take the account outright.
	sessionOnly := map[string]bool{
		"users.password.change":   true,
		"users.email.change":      true,
		"users.twofactor.start":   true,
		"users.twofactor.confirm": true,
		"users.twofactor.disable": true,
		"users.tokens.issue":      true,
		"users.tokens.revoke":     true,
	}

	for _, rt := range apiRoutes {
		if sessionOnly[rt.op.ID] && rt.op.Auth != swagger.AuthSession {
			t.Errorf("%s accepts %v, want a session", rt.op.ID, rt.op.Auth)
		}
	}
}

func TestNoCredentialMaterialIsDocumentedAsReadable(t *testing.T) {
	// A hash, a seed, or a secret has no business in a stored record's
	// documented shape. The enrollment and issue responses are the sole
	// exceptions: they serve their material once and never again.
	oncePerLifetime := map[string]bool{
		"TwoFactorEnrollment": true,
		"IssuedToken":         true,
		"IssuedInvite":        true,
	}
	forbidden := []string{"password", "hash", "secret", "seed"}

	for _, tp := range apiTypes {
		if tp.Input || oncePerLifetime[tp.Name] {
			continue
		}
		for _, f := range tp.Fields {
			for _, word := range forbidden {
				if strings.Contains(f.Name, word) {
					t.Errorf("type %s exposes field %q", tp.Name, f.Name)
				}
			}
		}
	}
}

func TestTheGraphSchemaDerivesFromTheSameSurface(t *testing.T) {
	reg := documented(t)

	schema, err := graphql.BuildSchema(reg, "/api/v1")
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}

	// A read becomes a query field and a write becomes a mutation field,
	// so each operation is looked up under the root its method implies.
	for _, rt := range apiRoutes {
		root := graphql.OperationQuery
		if rt.op.IsMutation() {
			root = graphql.OperationMutation
		}
		if _, ok := schema.Lookup(root, rt.op.GraphQLName()); !ok {
			t.Errorf("operation %s is missing from the %s root of the graph schema", rt.op.ID, root)
		}
	}

	if sdl := schema.SDL(); sdl == "" {
		t.Fatal("schema renders no SDL")
	}
}
