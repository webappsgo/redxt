package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/service"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/server/template"
)

// testServer is a handler mounted the way the router mounts it, with the
// authentication stage in front of it. Exercising the mounted tree is the
// point: a handler that works in isolation but is unreachable over HTTP
// is not wired in, and only a routed request proves otherwise.
type testServer struct {
	handler *Handler
	svc     *service.Service
	mux     http.Handler
	base    string
}

// newTestServer builds a running Regular User surface over a real
// database, so nothing below the handler is stubbed out.
func newTestServer(t *testing.T) *testServer {
	t.Helper()

	db, err := database.OpenUsers(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = database.EnsureUsersSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}

	key, err := security.GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	cipher, err := security.NewCipher(key, 1)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.Users.Enabled = true
	cfg.Server.Users.Registration.Mode = "open"
	cfg.Server.Users.Registration.RequireEmailVerification = false
	cfg.Server.Orgs.Enabled = true
	cfg.Server.Orgs.Creation.Mode = "open"

	svc, err := service.New(service.Options{Store: store.New(db), Config: cfg, Cipher: cipher})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	pages, err := template.New()
	if err != nil {
		t.Fatalf("template.New: %v", err)
	}
	h, err := New(Options{Service: svc, Config: cfg, Templates: pages})
	if err != nil {
		t.Fatalf("handler.New: %v", err)
	}

	mux := http.NewServeMux()
	for _, prefix := range h.WebPrefixes() {
		mux.Handle(prefix, h.Web())
	}
	for _, prefix := range h.APIPrefixes() {
		mux.Handle(prefix, h.API())
	}

	auth := middleware.Auth(middleware.AuthOptions{
		Verifier:           NewVerifier(svc, h.SessionCookieName()),
		SessionCookieNames: []string{h.SessionCookieName()},
		IsPublicPath:       h.IsPublicPath,
	})

	return &testServer{handler: h, svc: svc, mux: auth(mux), base: cfg.APIBasePath()}
}

// do performs one request against the mounted tree.
func (ts *testServer) do(t *testing.T, method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if form == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	ts.mux.ServeHTTP(rec, req)
	return rec
}

// register creates an account through the mounted registration route.
func (ts *testServer) register(t *testing.T, username, email, password string) {
	t.Helper()

	rec := ts.do(t, http.MethodPost, "/server/auth/register", url.Values{
		"username": {username},
		"email":    {email},
		"password": {password},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("register %s: status = %d, want %d (%s)", username, rec.Code, http.StatusSeeOther, rec.Body.String())
	}
}

// login signs in and returns the session cookie.
func (ts *testServer) login(t *testing.T, identifier, password string) *http.Cookie {
	t.Helper()

	rec := ts.do(t, http.MethodPost, "/server/auth/login", url.Values{
		"identifier": {identifier},
		"password":   {password},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login %s: status = %d, want %d (%s)", identifier, rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == ts.handler.SessionCookieName() && c.Value != "" {
			return c
		}
	}
	t.Fatalf("login %s: no session cookie issued", identifier)
	return nil
}

// account registers and signs in one user in a single step.
func (ts *testServer) account(t *testing.T, username string) *http.Cookie {
	t.Helper()

	const password = "Correct-Horse9-Battery"
	ts.register(t, username, username+"@example.test", password)
	return ts.login(t, username, password)
}

// TestWebPagesAreReachableWithoutCredentials covers the no-JS entry
// points PART 16 requires to work before anything else does.
func TestWebPagesAreReachableWithoutCredentials(t *testing.T) {
	ts := newTestServer(t)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "sign in", path: "/server/auth/login", want: `name="identifier"`},
		{name: "register", path: "/server/auth/register", want: `name="username"`},
		{name: "forgot password", path: "/server/auth/password/forgot", want: `method="post"`},
		{name: "reset password", path: "/server/auth/password/reset", want: `name="password"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := ts.do(t, http.MethodGet, tt.path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.want) {
				t.Errorf("body does not contain %q", tt.want)
			}
			if strings.Contains(body, "<script") {
				t.Errorf("page contains a script, which the no-JS rule forbids")
			}
		})
	}
}

// TestCSRFCookieIsMintedOnAGetForm proves the double-submit token the
// forms embed actually exists as a cookie, without which every POST from
// a browser would be refused.
func TestCSRFCookieIsMintedOnAGetForm(t *testing.T) {
	ts := newTestServer(t)

	rec := ts.do(t, http.MethodGet, "/server/auth/login", nil)
	var token string
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.DefaultCSRFCookie {
			token = c.Value
			if !c.HttpOnly {
				t.Error("csrf cookie is readable by script")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Error("csrf cookie is not SameSite=Strict")
			}
		}
	}
	if token == "" {
		t.Fatal("no csrf cookie issued")
	}
	if !strings.Contains(rec.Body.String(), token) {
		t.Error("the form does not embed the token from the cookie")
	}
}

// TestSignedOutUserIsSentToTheSignInPage covers the web surface's
// treatment of a missing credential: a redirect, not an API error.
func TestSignedOutUserIsSentToTheSignInPage(t *testing.T) {
	ts := newTestServer(t)

	for _, path := range []string{"/users/account", "/users/settings", "/users/security", "/orgs"} {
		t.Run(path, func(t *testing.T) {
			rec := ts.do(t, http.MethodGet, path, nil)
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != "/server/auth/login" {
				t.Errorf("Location = %q, want %q", got, "/server/auth/login")
			}
		})
	}
}

// TestRegisterAndSignIn is the happy path across the mounted routes.
func TestRegisterAndSignIn(t *testing.T) {
	ts := newTestServer(t)
	cookie := ts.account(t, "ada")

	rec := ts.do(t, http.MethodGet, "/users/account", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("account page status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "ada") {
		t.Error("account page does not name the signed-in user")
	}
}

// TestRegistrationRejectsInvalidInput covers server-side validation,
// which PART 34 requires regardless of what the form itself allows.
func TestRegistrationRejectsInvalidInput(t *testing.T) {
	ts := newTestServer(t)
	ts.register(t, "taken", "taken@example.test", "Correct-Horse9-Battery")

	tests := []struct {
		name  string
		form  url.Values
		wants int
	}{
		{
			name:  "empty username",
			form:  url.Values{"username": {""}, "email": {"a@example.test"}, "password": {"Correct-Horse9-Battery"}},
			wants: http.StatusBadRequest,
		},
		{
			name:  "malformed email",
			form:  url.Values{"username": {"bob"}, "email": {"not-an-address"}, "password": {"Correct-Horse9-Battery"}},
			wants: http.StatusBadRequest,
		},
		{
			name:  "short password",
			form:  url.Values{"username": {"bob"}, "email": {"bob@example.test"}, "password": {"x"}},
			wants: http.StatusBadRequest,
		},
		{
			name:  "username already taken",
			form:  url.Values{"username": {"taken"}, "email": {"other@example.test"}, "password": {"Correct-Horse9-Battery"}},
			wants: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := ts.do(t, http.MethodPost, "/server/auth/register", tt.form)
			if rec.Code != tt.wants {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wants, rec.Body.String())
			}
		})
	}
}

// TestWrongPasswordAndUnknownAccountAnswerIdentically is the PART 11
// rule that a failed sign-in must not reveal which half was wrong.
func TestWrongPasswordAndUnknownAccountAnswerIdentically(t *testing.T) {
	ts := newTestServer(t)
	ts.register(t, "ada", "ada@example.test", "Correct-Horse9-Battery")

	wrongPassword := ts.do(t, http.MethodPost, "/server/auth/login", url.Values{
		"identifier": {"ada"},
		"password":   {"not the password"},
	})
	noSuchUser := ts.do(t, http.MethodPost, "/server/auth/login", url.Values{
		"identifier": {"nobody"},
		"password":   {"not the password"},
	})

	if wrongPassword.Code != noSuchUser.Code {
		t.Fatalf("status %d vs %d: a failed sign-in reveals which half was wrong",
			wrongPassword.Code, noSuchUser.Code)
	}

	// The two pages differ in the identifier the visitor typed and in
	// the fresh double-submit token, neither of which tells the visitor
	// anything they did not already supply. What must match is the
	// reason given for the refusal.
	wrongReason := errorMessage(t, wrongPassword.Body.String())
	missingReason := errorMessage(t, noSuchUser.Body.String())
	if wrongReason != missingReason {
		t.Errorf("a wrong password is refused with %q and an unknown account with %q, which distinguishes them",
			wrongReason, missingReason)
	}

	for _, c := range wrongPassword.Result().Cookies() {
		if c.Name == ts.handler.SessionCookieName() && c.Value != "" {
			t.Error("a failed sign-in issued a session")
		}
	}
}

// errorMessage pulls the rendered failure reason out of a page.
func errorMessage(t *testing.T, body string) string {
	t.Helper()

	const marker = `class="error"`
	markerAt := strings.Index(body, marker)
	if markerAt < 0 {
		t.Fatalf("page carries no error message: %s", body)
	}
	tagEnd := strings.Index(body[markerAt:], ">")
	if tagEnd < 0 {
		t.Fatalf("unterminated error tag: %s", body)
	}
	rest := body[markerAt+tagEnd+1:]
	end := strings.Index(rest, "</p>")
	if end < 0 {
		t.Fatalf("unterminated error message: %s", body)
	}
	return strings.TrimSpace(rest[:end])
}

// TestAPIRefusesAnAnonymousRequest covers the REST surface's treatment of
// a missing credential.
func TestAPIRefusesAnAnonymousRequest(t *testing.T) {
	ts := newTestServer(t)

	for _, path := range []string{"/users", "/users/security", "/users/tokens", "/orgs"} {
		t.Run(path, func(t *testing.T) {
			rec := ts.do(t, http.MethodGet, ts.base+path, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

// TestAPIAnswersASignedInCaller proves the versioned REST tree is mounted
// and shares the session the web surface issued.
func TestAPIAnswersASignedInCaller(t *testing.T) {
	ts := newTestServer(t)
	cookie := ts.account(t, "ada")

	rec := ts.do(t, http.MethodGet, ts.base+"/users", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var envelope struct {
		Data struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if envelope.Data.Username != "ada" {
		t.Errorf("username = %q, want %q", envelope.Data.Username, "ada")
	}
}

// TestAnotherOrganizationIsNotAddressable is the PART 35 isolation rule.
// A caller outside an organization must not be able to tell an
// organization they cannot reach from one that does not exist, which is
// why the answer is not-found rather than forbidden.
func TestAnotherOrganizationIsNotAddressable(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.account(t, "ada")
	outsider := ts.account(t, "grace")

	created := ts.do(t, http.MethodPost, ts.base+"/orgs", url.Values{
		"slug": {"acme"},
		"name": {"Acme"},
	}, owner)
	if created.Code != http.StatusOK {
		t.Fatalf("create org: status = %d, want %d (%s)", created.Code, http.StatusOK, created.Body.String())
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "the organization itself", path: ts.base + "/orgs/acme"},
		{name: "its members", path: ts.base + "/orgs/acme/members"},
		{name: "its audit trail", path: ts.base + "/orgs/acme/audit"},
		{name: "its domains", path: ts.base + "/orgs/acme/domains"},
		{name: "an organization that does not exist", path: ts.base + "/orgs/nowhere"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := ts.do(t, http.MethodGet, tt.path, nil, outsider)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}

	reachable := ts.do(t, http.MethodGet, ts.base+"/orgs/acme", nil, owner)
	if reachable.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want %d (%s)", reachable.Code, http.StatusOK, reachable.Body.String())
	}
}

// TestOutsiderCannotChangeAnOrganization covers write paths as well as
// reads: an isolation rule that only holds for GET is not isolation.
func TestOutsiderCannotChangeAnOrganization(t *testing.T) {
	ts := newTestServer(t)
	owner := ts.account(t, "ada")
	outsider := ts.account(t, "grace")

	created := ts.do(t, http.MethodPost, ts.base+"/orgs", url.Values{
		"slug": {"acme"},
		"name": {"Acme"},
	}, owner)
	if created.Code != http.StatusOK {
		t.Fatalf("create org: status = %d (%s)", created.Code, created.Body.String())
	}

	tests := []struct {
		name   string
		method string
		path   string
		form   url.Values
	}{
		{name: "rename it", method: http.MethodPatch, path: ts.base + "/orgs/acme", form: url.Values{"name": {"Stolen"}}},
		{name: "delete it", method: http.MethodDelete, path: ts.base + "/orgs/acme"},
		{name: "invite to it", method: http.MethodPost, path: ts.base + "/orgs/acme/invites", form: url.Values{"role": {"admin"}}},
		{name: "claim a domain for it", method: http.MethodPost, path: ts.base + "/orgs/acme/domains", form: url.Values{"domain": {"example.test"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := ts.do(t, tt.method, tt.path, tt.form, outsider)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}

	still := ts.do(t, http.MethodGet, ts.base+"/orgs/acme", nil, owner)
	if !strings.Contains(still.Body.String(), "Acme") {
		t.Error("the organization was changed by a caller outside it")
	}
}

// TestPrivateProfileIsNotFound covers the PART 34 privacy rule: a
// profile the viewer may not see answers as absent, not as refused,
// because a refusal confirms the account exists.
func TestPrivateProfileIsNotFound(t *testing.T) {
	ts := newTestServer(t)
	cookie := ts.account(t, "ada")

	private := ts.do(t, http.MethodPost, "/users/account", url.Values{
		"display_name": {"Ada"},
		"visibility":   {"private"},
	}, cookie)
	if private.Code != http.StatusSeeOther {
		t.Fatalf("set visibility: status = %d (%s)", private.Code, private.Body.String())
	}

	rec := ts.do(t, http.MethodGet, ts.base+"/users/ada", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	missing := ts.do(t, http.MethodGet, ts.base+"/users/nobody", nil)
	if missing.Code != rec.Code {
		t.Errorf("a private profile answers %d and an absent one %d, which distinguishes them",
			rec.Code, missing.Code)
	}
}

// TestUnknownRouteUnderTheUserTreeIsNotFound proves the mounted mux
// answers its own misses rather than falling through to another PART.
func TestUnknownRouteUnderTheUserTreeIsNotFound(t *testing.T) {
	ts := newTestServer(t)
	cookie := ts.account(t, "ada")

	rec := ts.do(t, http.MethodGet, ts.base+"/users/security/nothing-here", nil, cookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestIsPublicPathDrawsTheLineAtTheAPI guards the two halves of the
// contract this handler hands the authentication stage: no path outside
// this handler may be refused on its behalf, the server-rendered pages
// answer for themselves so a browser gets a redirect rather than a bare
// 401, and the REST tree is refused up front.
func TestIsPublicPathDrawsTheLineAtTheAPI(t *testing.T) {
	ts := newTestServer(t)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "the index", path: "/", want: true},
		{name: "health", path: "/server/healthz", want: true},
		{name: "the admin panel", path: "/server/administration", want: true},
		{name: "sign in", path: "/server/auth/login", want: true},
		{name: "a vanity profile", path: "/users/ada", want: true},
		{name: "the account page", path: "/users/account", want: true},
		{name: "the security page", path: "/users/security", want: true},
		{name: "an organization page", path: "/orgs/acme/members", want: true},
		{name: "the api profile route", path: ts.base + "/users/ada", want: true},
		{name: "the api account route", path: ts.base + "/users", want: false},
		{name: "the api security route", path: ts.base + "/users/security", want: false},
		{name: "an api organization route", path: ts.base + "/orgs/acme/members", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ts.handler.IsPublicPath(tt.path); got != tt.want {
				t.Errorf("IsPublicPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
