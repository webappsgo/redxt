package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/server/template"
)

// newTestHandler builds a Handler over the real service from
// newTestService (see service_test.go), so the routes under test see
// the same database constraints the running server does. isRegularUser
// is nil unless a test supplies one.
func newTestHandler(t *testing.T, isRegularUser func(*http.Request) bool) (*Handler, *Service, string) {
	t.Helper()

	svc, serverDB := newTestService(t)
	token := seedSetupToken(t, serverDB)

	pages, err := template.New()
	if err != nil {
		t.Fatalf("template.New: %v", err)
	}

	h, err := New(Options{
		Service:       svc,
		Config:        config.DefaultConfig(),
		Templates:     pages,
		IsRegularUser: isRegularUser,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h, svc, token
}

// createAdmin completes first-run setup and returns the session token
// for the resulting Primary Admin.
func createAdmin(t *testing.T, svc *Service, token string) string {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.CompleteSetup(ctx, token, "root", "root@example.test", "correct horse battery staple"); err != nil {
		t.Fatalf("CompleteSetup: %v", err)
	}
	sessionToken, _, err := svc.Login(ctx, "root", "correct horse battery staple", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return sessionToken
}

func TestNewRequiresServiceAndConfig(t *testing.T) {
	svc := &Service{}
	tests := []struct {
		name string
		opts Options
	}{
		{name: "missing service", opts: Options{Config: config.DefaultConfig()}},
		{name: "missing config", opts: Options{Service: svc}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Fatal("New: expected an error")
			}
		})
	}
}

func TestSetupPageBeforeAndAfterSetup(t *testing.T) {
	h, svc, token := newTestHandler(t, nil)
	mux := h.Web()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, h.config.AdminBasePath()+"/config/setup?token="+token, nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET setup before completion: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), token) {
		t.Fatal("GET setup: expected the token to be echoed into the form")
	}

	createAdmin(t, svc, token)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, h.config.AdminBasePath()+"/config/setup", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET setup after completion: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/server/auth/login" {
		t.Fatalf("GET setup after completion: Location = %q, want %q", got, "/server/auth/login")
	}
}

func TestSetupCreatesAdminAndRedirects(t *testing.T) {
	h, _, token := newTestHandler(t, nil)
	mux := h.Web()

	form := url.Values{
		"token":    {token},
		"username": {"root"},
		"email":    {"root@example.test"},
		"password": {"correct horse battery staple"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, h.config.AdminBasePath()+"/config/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST setup: status = %d, want %d, body: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/server/auth/login" {
		t.Fatalf("POST setup: Location = %q, want %q", got, "/server/auth/login")
	}
}

func TestSetupWrongToken(t *testing.T) {
	h, _, _ := newTestHandler(t, nil)
	mux := h.Web()

	form := url.Values{
		"token":    {"not-the-real-token"},
		"username": {"root"},
		"email":    {"root@example.test"},
		"password": {"correct horse battery staple"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, h.config.AdminBasePath()+"/config/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST setup wrong token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestSetupAlreadyComplete(t *testing.T) {
	h, svc, token := newTestHandler(t, nil)
	createAdmin(t, svc, token)
	mux := h.Web()

	form := url.Values{
		"token":    {token},
		"username": {"second"},
		"email":    {"second@example.test"},
		"password": {"correct horse battery staple"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, h.config.AdminBasePath()+"/config/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST setup already complete: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/server/auth/login" {
		t.Fatalf("POST setup already complete: Location = %q, want %q", got, "/server/auth/login")
	}
}

func TestDashboardIsolation(t *testing.T) {
	tests := []struct {
		name          string
		isRegularUser func(*http.Request) bool
		signInAdmin   bool
		wantStatus    int
		wantLocation  string
	}{
		{
			name:         "unauthenticated goes to shared login",
			wantStatus:   http.StatusSeeOther,
			wantLocation: "/server/auth/login",
		},
		{
			name:          "regular user goes to their own dashboard",
			isRegularUser: func(*http.Request) bool { return true },
			wantStatus:    http.StatusSeeOther,
			wantLocation:  "/users",
		},
		{
			name:        "admin session sees the panel",
			signInAdmin: true,
			wantStatus:  http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, svc, token := newTestHandler(t, tc.isRegularUser)
			mux := h.Web()

			req := httptest.NewRequest(http.MethodGet, h.config.AdminBasePath()+"/", nil)
			if tc.signInAdmin {
				sessionToken := createAdmin(t, svc, token)
				req.AddCookie(&http.Cookie{Name: SessionCookieName(h.config), Value: sessionToken})
			}

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("dashboard: status = %d, want %d, body: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantLocation != "" {
				if got := rec.Header().Get("Location"); got != tc.wantLocation {
					t.Fatalf("dashboard: Location = %q, want %q", got, tc.wantLocation)
				}
			}
		})
	}
}

func TestLogoutClearsSessionAndRedirects(t *testing.T) {
	h, svc, token := newTestHandler(t, nil)
	sessionToken := createAdmin(t, svc, token)
	mux := h.Web()

	req := httptest.NewRequest(http.MethodPost, h.config.AdminBasePath()+"/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName(h.config), Value: sessionToken})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/server/auth/login" {
		t.Fatalf("logout: Location = %q, want %q", got, "/server/auth/login")
	}

	if _, err := svc.CurrentAdmin(context.Background(), sessionToken); err == nil {
		t.Fatal("logout: session token still valid after logout")
	}

	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName(h.config) {
			cleared = c
		}
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatal("logout: expected the session cookie to be cleared")
	}
}

func TestWebPrefixes(t *testing.T) {
	h, _, _ := newTestHandler(t, nil)
	want := h.config.AdminBasePath() + "/"
	got := h.WebPrefixes()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("WebPrefixes = %v, want [%q]", got, want)
	}
}
