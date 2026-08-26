package handler

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/admin"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/service"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/server/template"
)

// newTestServerWithAdmin builds the same mounted Regular User surface as
// newTestServer, plus a real admin.Service wired into the shared login
// form the way startup/users.go wires it, so PART 17's "the shared
// /server/auth/login form also signs an admin in" fallback runs against
// real databases rather than a stub.
func newTestServerWithAdmin(t *testing.T) (*testServer, *admin.Service, string) {
	t.Helper()

	usersDB, err := database.OpenUsers(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	t.Cleanup(func() { _ = usersDB.Close() })
	if err = database.EnsureUsersSchema(context.Background(), usersDB); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}

	serverDB, err := database.OpenServer(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenServer: %v", err)
	}
	t.Cleanup(func() { _ = serverDB.Close() })
	if err = database.EnsureServerSchema(context.Background(), serverDB); err != nil {
		t.Fatalf("EnsureServerSchema: %v", err)
	}

	setupToken, err := security.GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken: %v", err)
	}
	if _, _, err = database.EnsureSecret(context.Background(), serverDB, security.SecretSetupToken, func() (string, error) {
		return security.HashToken(setupToken), nil
	}); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}

	adminSvc := admin.NewService(usersDB, serverDB)
	if _, err = adminSvc.CompleteSetup(context.Background(), setupToken, "root", "root@example.test", "correct horse battery staple"); err != nil {
		t.Fatalf("CompleteSetup: %v", err)
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

	svc, err := service.New(service.Options{Store: store.New(usersDB), Config: cfg, Cipher: cipher})
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	pages, err := template.New()
	if err != nil {
		t.Fatalf("template.New: %v", err)
	}
	h, err := New(Options{Service: svc, Config: cfg, Templates: pages, AdminService: adminSvc})
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

	ts := &testServer{handler: h, svc: svc, mux: auth(mux), base: cfg.APIBasePath()}
	return ts, adminSvc, cfg.AdminBasePath()
}

// TestSharedLoginSignsInAnAdmin covers the PART 17 isolation rule from
// the other side: the shared login form, with no admin-specific field or
// hint, must still recognize an admin's credentials and land them on the
// admin panel rather than /users/account.
func TestSharedLoginSignsInAnAdmin(t *testing.T) {
	ts, adminSvc, adminBase := newTestServerWithAdmin(t)

	rec := ts.do(t, http.MethodPost, "/server/auth/login", url.Values{
		"identifier": {"root"},
		"password":   {"correct horse battery staple"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin login: status = %d, want %d (%s)", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if got, want := rec.Header().Get("Location"), adminBase+"/"; got != want {
		t.Fatalf("admin login: Location = %q, want %q", got, want)
	}

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == admin.SessionCookieName(ts.handler.config) {
			sessionCookie = c
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("admin login: no admin session cookie issued")
	}
	if _, err := adminSvc.CurrentAdmin(context.Background(), sessionCookie.Value); err != nil {
		t.Fatalf("CurrentAdmin: %v", err)
	}

	// The Regular User session cookie is a separate name; the shared
	// form must not also sign the visitor in as a Regular User just
	// because an admin account with the same login route exists.
	for _, c := range rec.Result().Cookies() {
		if c.Name == ts.handler.SessionCookieName() && c.Value != "" {
			t.Fatalf("admin login: unexpected regular-user session cookie %q", c.Name)
		}
	}
}

// TestSharedLoginRejectsWrongAdminPassword confirms the fallback never
// masks a genuine failure as success, and that a caller who is neither a
// known Regular User nor a known admin sees the same rejected form
// either way, per PART 17's "no admin-specific hint" rule.
func TestSharedLoginRejectsWrongAdminPassword(t *testing.T) {
	ts, _, _ := newTestServerWithAdmin(t)

	rec := ts.do(t, http.MethodPost, "/server/auth/login", url.Values{
		"identifier": {"root"},
		"password":   {"wrong password"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong admin password: status = %d, want %d (%s)", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if (c.Name == ts.handler.SessionCookieName() || c.Name == admin.SessionCookieName(ts.handler.config)) && c.Value != "" {
			t.Fatalf("wrong admin password: unexpected session cookie %q", c.Name)
		}
	}
}
