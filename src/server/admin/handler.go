// Package admin serves the AI.md PART 17 admin panel: the first-run
// setup wizard, the landing dashboard, and session logout.
//
// Sign-in itself is not served here. PART 17 requires the admin panel to
// use the same shared /server/auth/login form Regular Users see, with no
// admin-specific hint anywhere on it, so the credential check lives in
// the server/handler package's webLogin alongside the Regular User
// check; this package only issues and reads the resulting cookie.
//
// This package deliberately covers only the panel's entry points. The
// large config/* management surface (settings, SSL, email, scheduler,
// logs, backup, updates, users, orgs, cluster, agents) is tracked in
// TODO.AI.md as follow-up work rather than stubbed out here: PART 1
// forbids partial implementations, so a route that has nothing real
// behind it yet does not exist.
package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/template"
)

// csrfTokenLength is the length of a freshly minted double-submit token.
const csrfTokenLength = 32

// Options configures a Handler.
type Options struct {
	// Service is the PART 17 admin account lifecycle.
	Service *Service
	// Config is the live configuration, read per request so a runtime
	// change to the admin path or cookie settings takes effect at once.
	Config *config.Config
	// Templates renders the server-side pages. Nil disables the panel.
	Templates *template.Set
	// IsRegularUser reports whether the request carries a valid,
	// signed-in Regular User session. It is nil-safe: when unset, no
	// request is ever treated as a Regular User. The admin dashboard
	// uses it to send a signed-in non-admin straight to their own
	// space rather than the shared sign-in form, per the PART 17
	// isolation table.
	IsRegularUser func(*http.Request) bool
}

// Handler serves the admin panel's server-rendered surface.
type Handler struct {
	svc           *Service
	config        *config.Config
	templates     *template.Set
	isRegularUser func(*http.Request) bool
}

// New returns a Handler.
func New(opts Options) (*Handler, error) {
	if opts.Service == nil {
		return nil, errors.New("admin: service is required")
	}
	if opts.Config == nil {
		return nil, errors.New("admin: config is required")
	}
	return &Handler{
		svc:           opts.Service,
		config:        opts.Config,
		templates:     opts.Templates,
		isRegularUser: opts.IsRegularUser,
	}, nil
}

// Web returns the admin panel, mounted under the configured admin path.
//
// The base path is read once at construction so the mux's registered
// patterns are fixed for the lifetime of this handler; a change to
// server.admin_path takes effect on the next process restart, which
// matches how every other init-only setting in PART 5 behaves.
func (h *Handler) Web() http.Handler {
	base := h.config.AdminBasePath()
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+base+"/config/setup", h.setupPage)
	mux.HandleFunc("POST "+base+"/config/setup", h.setup)
	mux.HandleFunc("POST "+base+"/logout", h.logout)
	mux.HandleFunc("GET "+base+"/", h.dashboard)

	return mux
}

// WebPrefixes returns the pattern the main router mounts this surface
// on.
func (h *Handler) WebPrefixes() []string {
	return []string{h.config.AdminBasePath() + "/"}
}

// csrfToken returns the request's double-submit token, minting and
// setting one when the request carried none. It reuses the same cookie
// the rest of the site's forms use: the token defends against a
// cross-site POST regardless of which account space it targets.
func (h *Handler) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(middleware.DefaultCSRFCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	token, err := security.RandomString(csrfTokenLength)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.DefaultCSRFCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure(h.config.Server.Session.Secure, r),
		SameSite: http.SameSiteStrictMode,
	})
	return token
}

// page builds the data every admin template receives.
func (h *Handler) page(w http.ResponseWriter, r *http.Request, title string, data any) template.Page {
	p := template.Page{
		Title:    title,
		AppName:  h.config.Server.ApplicationName,
		Language: h.config.Server.I18n.DefaultLanguage,
		CSRF:     h.csrfToken(w, r),
		Data:     data,
	}
	if p.Language == "" {
		p.Language = "en"
	}
	return p
}

// render writes a page, or a plain error when the template surface is
// switched off.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, name, title string, data any) {
	if h.templates == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if err := h.templates.Render(w, status, name, h.page(w, r, title, data)); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// renderErr writes a page with a failure message attached.
func (h *Handler) renderErr(w http.ResponseWriter, r *http.Request, status int, name, title string, data any, message string) {
	if h.templates == nil {
		http.Error(w, message, status)
		return
	}
	p := h.page(w, r, title, data)
	p.Error = message
	if err := h.templates.Render(w, status, name, p); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// redirect answers a successful form submission with a 303, so reloading
// the resulting page does not resubmit the form.
func redirect(w http.ResponseWriter, r *http.Request, target string) {
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// SessionCookieName is the cookie an admin session is carried in. It is
// exported so the shared /server/auth/login handler in server/handler
// can set the same cookie on a successful admin sign-in.
func SessionCookieName(cfg *config.Config) string {
	name := cfg.Server.Session.Admin.CookieName
	if name == "" {
		name = "admin_session"
	}
	return name
}

// SetSessionCookie writes the admin session cookie.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, cfg *config.Config, token string) {
	sess := cfg.Server.Session
	maxAge := int(sess.Admin.MaxAge.Duration().Seconds())
	if maxAge <= 0 {
		maxAge = int(SessionLifetime.Seconds())
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName(cfg),
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: sess.HTTPOnly,
		Secure:   cookieSecure(sess.Secure, r),
		SameSite: sameSite(sess.SameSite),
	})
}

// ClearSessionCookie removes the admin session cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	sess := cfg.Server.Session
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName(cfg),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: sess.HTTPOnly,
		Secure:   cookieSecure(sess.Secure, r),
		SameSite: sameSite(sess.SameSite),
	})
}

// cookieSecure resolves the configured secure setting. The default,
// auto, marks the cookie Secure exactly when the request arrived over
// TLS, so a plain-HTTP development run still keeps a session.
func cookieSecure(setting string, r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(setting)) {
	case "true":
		return true
	case "false":
		return false
	default:
		return r.TLS != nil ||
			strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
}

// sameSite maps the configured name onto the cookie attribute, defaulting
// to Strict.
func sameSite(name string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "lax":
		return http.SameSiteLaxMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteStrictMode
	}
}

// sessionToken returns the raw session value the request carried.
func (h *Handler) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName(h.config))
	if err != nil {
		return ""
	}
	return cookie.Value
}

// currentAdmin resolves the signed-in Server Admin behind a request.
func (h *Handler) currentAdmin(r *http.Request) (Admin, bool) {
	token := h.sessionToken(r)
	if token == "" {
		return Admin{}, false
	}
	found, err := h.svc.CurrentAdmin(r.Context(), token)
	if err != nil {
		return Admin{}, false
	}
	return found, true
}

// setupData is the first-run setup page payload.
type setupData struct {
	Token string
}

// dashboardData is the landing page payload.
type dashboardData struct {
	Admin Admin
}

// setupPage renders the first-run wizard, or sends the visitor to the
// shared sign-in page once an admin already exists.
func (h *Handler) setupPage(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := h.svc.NeedsSetup(r.Context())
	if err != nil {
		h.renderErr(w, r, http.StatusInternalServerError, "adminsetup", "Set up the administrator account", setupData{}, "could not check setup status")
		return
	}
	if !needsSetup {
		redirect(w, r, "/server/auth/login")
		return
	}
	h.render(w, r, http.StatusOK, "adminsetup", "Set up the administrator account", setupData{Token: r.URL.Query().Get("token")})
}

// setup creates the Primary Admin from the one-time setup token shown on
// the console at first run.
func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, http.StatusBadRequest, "adminsetup", "Set up the administrator account", setupData{}, "the form could not be read")
		return
	}

	token := formValue(r, "token")
	username := formValue(r, "username")
	email := formValue(r, "email")
	password := r.PostFormValue("password")

	if _, err := h.svc.CompleteSetup(r.Context(), token, username, email, password); err != nil {
		status := http.StatusInternalServerError
		message := "the account could not be created"
		switch {
		case errors.Is(err, ErrSetupComplete):
			redirect(w, r, "/server/auth/login")
			return
		case errors.Is(err, ErrInvalidSetupToken):
			status = http.StatusUnauthorized
			message = "invalid or expired setup token"
		}
		h.renderErr(w, r, status, "adminsetup", "Set up the administrator account", setupData{Token: token}, message)
		return
	}

	redirect(w, r, "/server/auth/login")
}

// logout ends the admin session.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if token := h.sessionToken(r); token != "" {
		_ = h.svc.Logout(r.Context(), token)
	}
	ClearSessionCookie(w, r, h.config)
	redirect(w, r, "/server/auth/login")
}

// dashboard is the admin panel's landing page. It carries out the PART 17
// isolation rule directly: an unauthenticated visitor is sent to the
// shared sign-in surface with no admin-specific hint, a signed-in
// Regular User is sent to their own space and never shown the admin
// login again, and a signed-in Server Admin sees the panel.
func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if found, ok := h.currentAdmin(r); ok {
		h.render(w, r, http.StatusOK, "admindashboard", "Administration", dashboardData{Admin: found})
		return
	}
	if h.isRegularUser != nil && h.isRegularUser(r) {
		redirect(w, r, "/users")
		return
	}
	redirect(w, r, "/server/auth/login")
}

// parseForm reads a bounded form body.
func parseForm(r *http.Request) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxFormBytes)
	return r.ParseForm()
}

// maxFormBytes bounds a request body. A setup form is a few hundred
// bytes; anything larger is either a mistake or an attempt to make the
// server allocate on a caller's behalf.
const maxFormBytes = 1 << 20

// formValue returns a trimmed form field.
func formValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PostFormValue(name))
}
