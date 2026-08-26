package template

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pageNames are the documents a handler may ask for by name. The list is
// asserted rather than derived, so a page deleted or renamed out of the
// set fails here instead of at the first request for it.
var pageNames = []string{
	"login", "register", "twofactor", "forgot", "reset", "message",
	"account", "settings", "security", "secret", "orgs", "org",
	"domain", "audit", "profile", "orgprofile",
	"adminsetup", "admindashboard",
}

// The payload types below mirror the shapes the handler package passes
// as Page.Data. They are restated here rather than imported because the
// handler imports this package, and reaching back the other way would
// close a cycle.
type loginPayload struct {
	RegistrationOpen bool
}

type accountUser struct {
	Username      string
	DisplayName   string
	Bio           string
	Location      string
	Website       string
	AvatarURL     string
	Timezone      string
	Language      string
	Visibility    string
	OrgVisibility bool
}

type accountPayload struct {
	User accountUser
}

func TestNewParsesEveryPage(t *testing.T) {
	set, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, name := range pageNames {
		if !set.Has(name) {
			t.Errorf("page %q is not defined", name)
		}
	}
}

func TestHas(t *testing.T) {
	set, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name string
		set  *Set
		page string
		want bool
	}{
		{name: "defined page", set: set, page: "login", want: true},
		{name: "undefined page", set: set, page: "nonexistent"},
		{name: "empty name", set: set, page: ""},
		// A nil Set answers false rather than panicking, so a handler that
		// checks before rendering degrades into a 404 instead of taking
		// the process down with it.
		{name: "nil set", set: nil, page: "login"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.set.Has(tc.page); got != tc.want {
				t.Fatalf("Has(%q) = %v, want %v", tc.page, got, tc.want)
			}
		})
	}
}

func TestRenderWritesTheRequestedStatusAndType(t *testing.T) {
	set, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	page := Page{
		Title:    "Sign in",
		AppName:  "redxt",
		Language: "en",
		Base:     "/server",
		CSRF:     "tok",
		Data:     loginPayload{RegistrationOpen: true},
	}

	if err = set.Render(rec, http.StatusOK, "login", page); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}

	body := rec.Body.String()
	if body == "" {
		t.Fatal("rendered an empty body")
	}
	// The form must carry the token, or every submission from this page
	// would be rejected by the double-submit check.
	if !strings.Contains(body, "tok") {
		t.Error("body omits the CSRF token")
	}
	if !strings.Contains(body, "Sign in") {
		t.Error("body omits the page title")
	}
	if strings.Contains(body, "<script") {
		t.Error("body loads a script, but every page must work without one")
	}
}

func TestRenderCarriesAnErrorStatus(t *testing.T) {
	set, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	page := Page{
		Title:    "Sign in",
		Language: "en",
		Error:    "Those details did not match.",
		Data:     loginPayload{},
	}

	if err = set.Render(rec, http.StatusUnauthorized, "login", page); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "Those details did not match.") {
		t.Error("body omits the error message")
	}
}

func TestRenderRefusesAnUndefinedPage(t *testing.T) {
	set, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	if err = set.Render(rec, http.StatusOK, "nonexistent", Page{}); err == nil {
		t.Fatal("Render accepted an undefined page")
	}

	// Nothing may reach the wire when the render fails. Had a status been
	// written first, the caller could no longer answer with a 500 and the
	// visitor would receive an empty 200 instead.
	if rec.Body.Len() != 0 {
		t.Errorf("wrote %d bytes for a failed render, want 0", rec.Body.Len())
	}
	if rec.Header().Get("Content-Type") != "" {
		t.Error("set a Content-Type for a failed render")
	}
}

func TestRenderEscapesUntrustedText(t *testing.T) {
	set, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const payload = `<script>alert(1)</script>`

	rec := httptest.NewRecorder()
	page := Page{
		Title:    "Profile",
		Language: "en",
		// A display name and a username are whatever the account owner
		// typed, so both are untrusted input on their way to a browser.
		Viewer: &Viewer{ID: 1, Username: payload, DisplayName: payload},
		Data: accountPayload{User: accountUser{
			Username:    payload,
			DisplayName: payload,
			Bio:         payload,
			Location:    payload,
			Website:     payload,
		}},
	}

	if err = set.Render(rec, http.StatusOK, "account", page); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if strings.Contains(rec.Body.String(), payload) {
		t.Fatal("account-owned text was rendered as live markup")
	}
}

func TestBufferWriterAccumulates(t *testing.T) {
	b := &bufferWriter{}

	for _, part := range []string{"one", "", "two"} {
		n, err := b.Write([]byte(part))
		if err != nil {
			t.Fatalf("Write(%q): %v", part, err)
		}
		if n != len(part) {
			t.Fatalf("Write(%q) = %d, want %d", part, n, len(part))
		}
	}

	if got := string(b.bytes); got != "onetwo" {
		t.Fatalf("buffer = %q, want %q", got, "onetwo")
	}
}
