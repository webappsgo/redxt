// Package handler serves the HTTP surface for AI.md PART 34 (Multi-User),
// PART 35 (Organizations), and PART 36 (Custom Domains).
//
// It translates requests into service calls and results into responses,
// and it makes no policy decisions of its own: every question about
// whether an action is allowed is answered one layer down, in the
// service, so the REST surface and the server-rendered pages can never
// disagree about who may do what.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/server/admin"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/service"
	"github.com/webappsgo/redxt/src/server/template"
)

// maxBodyBytes bounds a request body. A profile or an organization form
// is a few kilobytes; anything larger is either a mistake or an attempt
// to make the server allocate on a caller's behalf.
const maxBodyBytes = 1 << 20

// Options configures a Handler.
type Options struct {
	// Service is the PART 34-36 policy layer.
	Service *service.Service
	// Config is the live configuration, read per request so a runtime
	// change to registration mode or admin path takes effect at once.
	Config *config.Config
	// Templates renders the server-side pages. Nil disables the web
	// surface and leaves the REST surface working.
	Templates *template.Set
	// ClientIP resolves the caller's address under the PART 12
	// trusted-proxy rules. Nil falls back to the transport peer, which is
	// the correct answer for a server that is not behind a proxy.
	ClientIP func(*http.Request) string
	// AdminService backs the PART 17 admin sign-in check the shared
	// /server/auth/login form also performs. Nil disables admin sign-in
	// on this surface, leaving the rest of the web surface working.
	AdminService *admin.Service
}

// Handler serves both the REST and the server-rendered surfaces.
type Handler struct {
	svc       *service.Service
	config    *config.Config
	templates *template.Set
	clientIP  func(*http.Request) string
	admin     *admin.Service
}

// New returns a Handler.
func New(opts Options) (*Handler, error) {
	if opts.Service == nil {
		return nil, errors.New("handler: service is required")
	}
	if opts.Config == nil {
		return nil, errors.New("handler: config is required")
	}
	resolve := opts.ClientIP
	if resolve == nil {
		resolve = remoteHost
	}

	return &Handler{
		svc:       opts.Service,
		config:    opts.Config,
		templates: opts.Templates,
		clientIP:  resolve,
		admin:     opts.AdminService,
	}, nil
}

// remoteHost returns the transport peer's address without its port.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// apiBase returns the versioned API prefix, for example /api/v1.
func (h *Handler) apiBase() string {
	return h.config.APIBasePath()
}

// sessionCookie returns the end-user session cookie name.
func (h *Handler) sessionCookie() string {
	name := h.config.Server.Session.User.CookieName
	if name == "" {
		name = "user_session"
	}
	return name
}

// SessionCookieName is the cookie a Regular User session is carried in.
// The authentication stage needs the same answer this handler uses, so
// both read it from here rather than from the configuration twice.
func (h *Handler) SessionCookieName() string {
	return h.sessionCookie()
}

// Service returns the policy layer behind the handler, which the
// credential verifier needs in order to resolve sessions and tokens.
func (h *Handler) Service() *service.Service {
	return h.svc
}

// setSession writes the session cookie.
//
// The cookie is HttpOnly so script cannot read it, SameSite so a
// cross-site form cannot ride it, and Secure whenever the request itself
// arrived over TLS.
func (h *Handler) setSession(w http.ResponseWriter, r *http.Request, token string) {
	sess := h.config.Server.Session
	maxAge := int(sess.User.MaxAge.Duration().Seconds())
	if maxAge <= 0 {
		maxAge = int(h.config.Server.Users.Auth.SessionDuration.Duration().Seconds())
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.sessionCookie(),
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: sess.HTTPOnly,
		Secure:   cookieSecure(sess.Secure, r),
		SameSite: sameSite(sess.SameSite),
	})
}

// clearSession removes the session cookie.
func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	sess := h.config.Server.Session
	http.SetCookie(w, &http.Cookie{
		Name:     h.sessionCookie(),
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
	cookie, err := r.Cookie(h.sessionCookie())
	if err != nil {
		return ""
	}
	return cookie.Value
}

// caller is the identity behind a request, resolved from what the Auth
// middleware attached.
type caller struct {
	UserID int64
	// Token is set when the request authenticated with an API token
	// rather than a session, which some actions refuse.
	Token bool
	// Pending is set when the session has not answered its second factor
	// yet. Such a caller reaches the challenge route and nothing else.
	Pending bool
}

// currentUser resolves the Regular User behind a request.
//
// It reads the identity the Auth stage attached rather than re-verifying
// the credential, so a request is authenticated exactly once, and it
// accepts only a Regular User: a Server Admin credential is a different
// account space and must not reach an end-user route.
func currentUser(r *http.Request) (caller, bool) {
	info, ok := middleware.AuthFromContext(r.Context())
	if !ok {
		return caller{}, false
	}
	if info.Kind != middleware.SubjectUser && info.Kind != middleware.SubjectService {
		return caller{}, false
	}
	id, err := strconv.ParseInt(info.Subject, 10, 64)
	if err != nil || id <= 0 {
		return caller{}, false
	}
	return caller{
		UserID:  id,
		Token:   info.SessionID == "",
		Pending: info.HasScope(ScopeTwoFactorPending),
	}, true
}

// viewerID returns the caller's user id, or zero for an anonymous
// request. It is what the public profile routes use, because they answer
// for both.
func viewerID(r *http.Request) int64 {
	if c, ok := currentUser(r); ok {
		return c.UserID
	}
	return 0
}

// contentType returns the request's media type without its parameters.
func contentType(r *http.Request) string {
	value := r.Header.Get("Content-Type")
	if i := strings.IndexByte(value, ';'); i >= 0 {
		value = value[:i]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

// isForm reports whether the request carried an HTML form body.
//
// Both encodings are accepted on the same route so that the no-JS form
// path and the API client path share one handler, which is what keeps
// the two surfaces from drifting apart in behavior.
func isForm(r *http.Request) bool {
	switch contentType(r) {
	case "application/x-www-form-urlencoded", "multipart/form-data":
		return true
	default:
		return false
	}
}

// parseForm reads a bounded form body.
func parseForm(r *http.Request) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	return r.ParseForm()
}

// decodeJSON reads a bounded JSON body into v.
//
// Unknown fields are rejected rather than ignored, so a caller that
// misspells a field learns about it instead of silently having the old
// value kept.
func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()

	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// bind fills v from the request body, whichever encoding it used.
//
// A form request is reported back to the caller so it can read the
// fields it expects by name; a JSON request is decoded into v directly.
func bind(r *http.Request, v any) (bool, error) {
	if isForm(r) {
		return true, parseForm(r)
	}
	return false, decodeJSON(r, v)
}

// formValue returns a trimmed form field.
func formValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PostFormValue(name))
}

// formBool reports whether a checkbox was submitted. An absent checkbox
// is false, which is how HTML forms report an unchecked box.
func formBool(r *http.Request, name string) bool {
	switch strings.ToLower(formValue(r, name)) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// pathID reads a numeric path segment.
func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// sendOK writes a successful JSON response in the PART 14 envelope.
func sendOK(w http.ResponseWriter, data any) {
	_ = apierror.SendOK(w, data)
}

// sendErr writes a service error in the PART 14 envelope.
func sendErr(w http.ResponseWriter, err error) {
	_ = apierror.SendError(w, translate(err))
}

// translate maps a service error onto the PART 14 error envelope.
//
// An unrecognized error becomes a plain server error with no detail: the
// internal message may name a table, a driver, or a path, and PART 9
// forbids any of that reaching a client.
func translate(err error) *apierror.Error {
	var verify *service.VerificationError
	if errors.As(err, &verify) {
		return apierror.New(apierror.CodeValidationFailed).
			WithDetails(map[string]any{
				"code":   verify.Code,
				"domain": verify.Domain,
			})
	}

	switch {
	case errors.Is(err, service.ErrValidation):
		return apierror.Newf(apierror.CodeValidationFailed, "%s", err.Error())
	case errors.Is(err, service.ErrInvalidCredentials):
		return apierror.New(apierror.CodeUnauthorized)
	case errors.Is(err, service.ErrAccountLocked):
		return apierror.New(apierror.CodeAccountLocked)
	case errors.Is(err, service.ErrTwoFactorRequired):
		return apierror.New(apierror.CodeTwoFactorRequired)
	case errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrQuotaExceeded):
		return apierror.New(apierror.CodeForbidden)
	case errors.Is(err, service.ErrNotFound):
		return apierror.New(apierror.CodeNotFound)
	case errors.Is(err, service.ErrConflict):
		return apierror.New(apierror.CodeConflict)
	case errors.Is(err, service.ErrDisabled):
		return apierror.New(apierror.CodeNotFound)
	default:
		return apierror.Wrap(apierror.CodeServerError, err)
	}
}
