package handler

import (
	"net/http"
	"strconv"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/server/service"
	"github.com/webappsgo/redxt/src/server/template"
	"github.com/webappsgo/redxt/src/user"
)

// csrfTokenLength is the length of a freshly minted double-submit token.
const csrfTokenLength = 32

// Web returns the server-rendered surface for PART 34, 35 and 36.
//
// Every page is a form that posts to a route on this same mux and is
// answered with a redirect, which is the plain HTML path PART 16
// requires to work before any script is considered.
func (h *Handler) Web() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /server/auth/login", h.webLoginPage)
	mux.HandleFunc("POST /server/auth/login", h.webLogin)
	mux.HandleFunc("GET /server/auth/register", h.webRegisterPage)
	mux.HandleFunc("POST /server/auth/register", h.webRegister)
	mux.HandleFunc("GET /server/auth/invite/user/{code}", h.webInvitePage)
	mux.HandleFunc("POST /server/auth/logout", h.webLogout)
	mux.HandleFunc("GET /server/auth/2fa", h.webTwoFactorPage)
	mux.HandleFunc("POST /server/auth/2fa", h.webTwoFactor)
	mux.HandleFunc("GET /server/auth/password/forgot", h.webForgotPage)
	mux.HandleFunc("POST /server/auth/password/forgot", h.webForgot)
	mux.HandleFunc("GET /server/auth/password/reset", h.webResetPage)
	mux.HandleFunc("POST /server/auth/password/reset", h.webReset)
	mux.HandleFunc("GET /server/auth/verify/{code}", h.webVerify)

	mux.HandleFunc("GET /users/account", h.webAccountPage)
	mux.HandleFunc("POST /users/account", h.webSaveAccount)
	mux.HandleFunc("GET /users/settings", h.webSettingsPage)
	mux.HandleFunc("POST /users/settings", h.webSaveSettings)
	mux.HandleFunc("GET /users/security", h.webSecurityPage)
	mux.HandleFunc("POST /users/security/password", h.webChangePassword)
	mux.HandleFunc("POST /users/security/2fa", h.webTwoFactorStart)
	mux.HandleFunc("POST /users/security/2fa/confirm", h.webTwoFactorConfirm)
	mux.HandleFunc("POST /users/security/2fa/disable", h.webTwoFactorDisable)
	mux.HandleFunc("POST /users/sessions/{session_id}/revoke", h.webRevokeSession)
	mux.HandleFunc("POST /users/tokens", h.webIssueToken)
	mux.HandleFunc("POST /users/tokens/{token_id}/revoke", h.webRevokeToken)
	mux.HandleFunc("GET /users/{username}", h.webProfile)

	mux.HandleFunc("GET /orgs", h.webOrgsPage)
	mux.HandleFunc("POST /orgs", h.webCreateOrg)
	mux.HandleFunc("POST /users/invites/{code}", h.webAcceptInvite)
	mux.HandleFunc("GET /orgs/{slug}", h.webOrgPage)
	mux.HandleFunc("POST /orgs/{slug}/settings", h.webSaveOrg)
	mux.HandleFunc("POST /orgs/{slug}/members/{member_id}", h.webSetMemberRole)
	mux.HandleFunc("POST /orgs/{slug}/members/{member_id}/remove", h.webRemoveMember)
	mux.HandleFunc("POST /orgs/{slug}/invites", h.webCreateInvite)
	mux.HandleFunc("POST /orgs/{slug}/invites/{invite_id}/revoke", h.webRevokeInvite)
	mux.HandleFunc("GET /orgs/{slug}/audit", h.webAudit)
	mux.HandleFunc("POST /orgs/{slug}/domains", h.webAddDomain)
	mux.HandleFunc("GET /orgs/{slug}/domains/{domain_id}", h.webDomainPage)
	mux.HandleFunc("POST /orgs/{slug}/domains/{domain_id}", h.webSaveDomain)
	mux.HandleFunc("POST /orgs/{slug}/domains/{domain_id}/verify", h.webVerifyDomain)
	mux.HandleFunc("POST /orgs/{slug}/domains/{domain_id}/remove", h.webRemoveDomain)

	return mux
}

// WebPrefixes returns the patterns the main router mounts the
// server-rendered surface on.
func (h *Handler) WebPrefixes() []string {
	return []string{
		"/server/auth/",
		"/users",
		"/users/",
		"/orgs",
		"/orgs/",
	}
}

// csrfToken returns the request's double-submit token, minting and
// setting one when the request carried none.
//
// The cookie is HttpOnly: the token is embedded in the form server-side,
// so no script ever needs to read it, and a cross-site page that cannot
// read the cookie cannot echo the value back.
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

// page builds the data every template receives.
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
	if c, ok := currentUser(r); ok {
		if account, err := h.svc.User(r.Context(), c.UserID); err == nil {
			p.Viewer = &template.Viewer{
				ID:          account.ID,
				Username:    account.Username,
				DisplayName: account.DisplayName,
			}
		}
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
func (h *Handler) renderErr(w http.ResponseWriter, r *http.Request, name, title string, data any, err error) {
	if h.templates == nil {
		sendErr(w, err)
		return
	}
	e := translate(err)
	p := h.page(w, r, title, data)
	p.Error = e.Message
	if renderErr := h.templates.Render(w, e.HTTPStatusCode, name, p); renderErr != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// redirect answers a successful form submission.
//
// A POST is always answered with a redirect so that reloading the
// resulting page does not resubmit the form.
func redirect(w http.ResponseWriter, r *http.Request, target string) {
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// webUser resolves the signed-in account or sends the visitor to the
// sign-in page.
func (h *Handler) webUser(w http.ResponseWriter, r *http.Request) (caller, bool) {
	c, ok := currentUser(r)
	if !ok || c.Token {
		redirect(w, r, "/server/auth/login")
		return caller{}, false
	}
	if c.Pending {
		redirect(w, r, "/server/auth/2fa")
		return caller{}, false
	}
	return c, true
}

// webOrg resolves the organization named in the path.
func (h *Handler) webOrg(w http.ResponseWriter, r *http.Request) (caller, service.OrgAccess, bool) {
	c, ok := h.webUser(w, r)
	if !ok {
		return caller{}, service.OrgAccess{}, false
	}
	access, err := h.svc.AccessBySlug(r.Context(), r.PathValue("slug"), c.UserID)
	if err != nil {
		h.render(w, r, http.StatusNotFound, "message", "Not found", messageData{
			Body:     "That organization does not exist, or you are not a member of it.",
			LinkURL:  "/orgs",
			LinkText: "Your organizations",
		})
		return caller{}, service.OrgAccess{}, false
	}
	return c, access, true
}

// messageData is the payload of the generic result page.
type messageData struct {
	Body     string
	LinkURL  string
	LinkText string
}

// secretData is the payload of the one-time secret page.
type secretData struct {
	Secret     string
	Extra      []string
	ExtraTitle string
	ConfirmURL string
	LinkURL    string
	LinkText   string
}

// loginData is the sign-in page payload.
type loginData struct {
	RegistrationOpen bool
}

// registerData is the registration page payload.
type registerData struct {
	Closed         bool
	InviteRequired bool
	Code           string
}

// resetData is the password-reset page payload.
type resetData struct {
	Token string
}

// accountData is the profile page payload.
type accountData struct {
	User userView
}

// securityData is the security page payload.
type securityData struct {
	Email            string
	EmailVerified    bool
	TwoFactorEnabled bool
	Sessions         []model.Session
	Tokens           []tokenView
	Orgs             []orgView
}

// orgsData is the organization index payload.
type orgsData struct {
	Orgs           []orgView
	CanCreate      bool
	InviteRequired bool
}

// orgData is the organization detail payload.
type orgData struct {
	Org              orgView
	Role             string
	CanManageMembers bool
	CanSettings      bool
	Members          []memberView
	Invites          []inviteView
	Domains          []domainView
}

// domainData is the custom-domain detail payload.
type domainData struct {
	Slug   string
	Domain domainView
	Record service.VerificationRecord
}

// auditData is the audit page payload.
type auditData struct {
	Slug    string
	Entries []model.AuditEntry
}

// webLoginPage renders the sign-in form.
func (h *Handler) webLoginPage(w http.ResponseWriter, r *http.Request) {
	mode := h.svc.RegistrationMode()
	h.render(w, r, http.StatusOK, "login", "Sign in", loginData{
		RegistrationOpen: mode.SelfServiceAllowed() || mode.InviteAllowed(),
	})
}

// webLogin verifies a password and opens a session.
func (h *Handler) webLogin(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "login", "Sign in", loginData{}, err)
		return
	}

	result, err := h.svc.Login(r.Context(), service.LoginInput{
		Identifier: formValue(r, "identifier"),
		Password:   r.PostFormValue("password"),
		IP:         h.clientIP(r),
		UserAgent:  r.UserAgent(),
	})
	if err != nil {
		mode := h.svc.RegistrationMode()
		h.renderErr(w, r, "login", "Sign in", loginData{
			RegistrationOpen: mode.SelfServiceAllowed() || mode.InviteAllowed(),
		}, err)
		return
	}

	h.setSession(w, r, result.SessionToken)
	if result.TwoFactorPending {
		redirect(w, r, "/server/auth/2fa")
		return
	}
	redirect(w, r, "/users/account")
}

// registerPageData describes the registration form for the current mode.
func (h *Handler) registerPageData(code string) registerData {
	mode := h.svc.RegistrationMode()
	return registerData{
		Closed:         !mode.SelfServiceAllowed() && !mode.InviteAllowed(),
		InviteRequired: !mode.SelfServiceAllowed() && mode.InviteAllowed(),
		Code:           code,
	}
}

// webRegisterPage renders the registration form.
func (h *Handler) webRegisterPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, "register", "Create an account", h.registerPageData(""))
}

// webInvitePage renders the registration form with an invite filled in.
func (h *Handler) webInvitePage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, "register", "Create an account",
		h.registerPageData(r.PathValue("code")))
}

// webRegister creates an account.
func (h *Handler) webRegister(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "register", "Create an account", h.registerPageData(""), err)
		return
	}

	code := formValue(r, "invite")
	_, err := h.svc.Register(r.Context(), service.RegisterInput{
		Username:   formValue(r, "username"),
		Email:      formValue(r, "email"),
		Password:   r.PostFormValue("password"),
		InviteCode: code,
	})
	if err != nil {
		h.renderErr(w, r, "register", "Create an account", h.registerPageData(code), err)
		return
	}
	redirect(w, r, "/server/auth/login")
}

// webLogout ends the session and returns to the sign-in page.
func (h *Handler) webLogout(w http.ResponseWriter, r *http.Request) {
	if token := h.sessionToken(r); token != "" {
		_ = h.svc.Logout(r.Context(), token)
	}
	h.clearSession(w, r)
	redirect(w, r, "/server/auth/login")
}

// webTwoFactorPage renders the second-factor prompt.
func (h *Handler) webTwoFactorPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, "twofactor", "Two-factor code", nil)
}

// webTwoFactor completes a sign-in waiting on a code.
func (h *Handler) webTwoFactor(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "twofactor", "Two-factor code", nil, err)
		return
	}

	token := h.sessionToken(r)
	if token == "" {
		redirect(w, r, "/server/auth/login")
		return
	}
	account, _, err := h.svc.ResolveSession(r.Context(), token)
	if err != nil {
		redirect(w, r, "/server/auth/login")
		return
	}
	if err = h.svc.VerifyTwoFactor(r.Context(), account.ID, token, formValue(r, "code")); err != nil {
		h.renderErr(w, r, "twofactor", "Two-factor code", nil, err)
		return
	}
	redirect(w, r, "/users/account")
}

// webForgotPage renders the password-reset request form.
func (h *Handler) webForgotPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, "forgot", "Reset your password", nil)
}

// webForgot issues a reset token.
//
// The visitor is sent to the same confirmation whether or not an account
// matched, so the page cannot be used to test which addresses exist.
func (h *Handler) webForgot(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "forgot", "Reset your password", nil, err)
		return
	}
	if _, _, err := h.svc.StartPasswordReset(r.Context(), formValue(r, "identifier")); err != nil {
		h.renderErr(w, r, "forgot", "Reset your password", nil, err)
		return
	}
	h.render(w, r, http.StatusOK, "message", "Check your email", messageData{
		Body:     "If that account exists, a reset link is on its way.",
		LinkURL:  "/server/auth/login",
		LinkText: "Back to sign in",
	})
}

// webResetPage renders the new-password form.
func (h *Handler) webResetPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, "reset", "Choose a new password", resetData{
		Token: r.URL.Query().Get("token"),
	})
}

// webReset sets a new password from a reset token.
func (h *Handler) webReset(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "reset", "Choose a new password", resetData{}, err)
		return
	}

	token := formValue(r, "token")
	if err := h.svc.CompletePasswordReset(r.Context(), token, r.PostFormValue("password")); err != nil {
		h.renderErr(w, r, "reset", "Choose a new password", resetData{Token: token}, err)
		return
	}
	redirect(w, r, "/server/auth/login")
}

// webVerify confirms an email address from a link.
func (h *Handler) webVerify(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.CompleteEmailVerification(r.Context(), r.PathValue("code")); err != nil {
		h.renderErr(w, r, "message", "Verification failed", messageData{
			LinkURL:  "/server/auth/login",
			LinkText: "Back to sign in",
		}, err)
		return
	}
	h.render(w, r, http.StatusOK, "message", "Email verified", messageData{
		Body:     "Your email address is confirmed.",
		LinkURL:  "/users/account",
		LinkText: "Your profile",
	})
}

// webAccountPage renders the caller's profile form.
func (h *Handler) webAccountPage(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	account, err := h.svc.User(r.Context(), c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "Profile", messageData{}, err)
		return
	}
	h.render(w, r, http.StatusOK, "account", "Profile", accountData{User: newUserView(account)})
}

// webSaveAccount writes the caller's profile.
func (h *Handler) webSaveAccount(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "Profile", messageData{}, err)
		return
	}

	account, err := h.svc.UpdateProfile(r.Context(), c.UserID, service.ProfileInput{
		DisplayName:       formValue(r, "display_name"),
		Bio:               formValue(r, "bio"),
		Location:          formValue(r, "location"),
		Website:           formValue(r, "website"),
		AvatarURL:         formValue(r, "avatar_url"),
		Timezone:          formValue(r, "timezone"),
		Language:          formValue(r, "language"),
		NotificationEmail: formValue(r, "notification_email"),
		Visibility:        formValue(r, "visibility"),
		OrgVisibility:     formBool(r, "org_visibility"),
	})
	if err != nil {
		current, loadErr := h.svc.User(r.Context(), c.UserID)
		if loadErr != nil {
			h.renderErr(w, r, "message", "Profile", messageData{}, err)
			return
		}
		h.renderErr(w, r, "account", "Profile", accountData{User: newUserView(current)}, err)
		return
	}
	_ = account
	redirect(w, r, "/users/account")
}

// webSettingsPage renders the caller's preferences.
func (h *Handler) webSettingsPage(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	prefs, err := h.svc.Preferences(r.Context(), c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "Settings", messageData{}, err)
		return
	}
	h.render(w, r, http.StatusOK, "settings", "Settings", newPrefsView(prefs))
}

// webSaveSettings writes the caller's preferences.
func (h *Handler) webSaveSettings(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "Settings", messageData{}, err)
		return
	}

	stored, err := h.svc.Preferences(r.Context(), c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "Settings", messageData{}, err)
		return
	}

	submitted := prefsView{
		ShowEmail:     formBool(r, "show_email"),
		ShowActivity:  formBool(r, "show_activity"),
		ShowOrgs:      formBool(r, "show_orgs"),
		Searchable:    formBool(r, "searchable"),
		EmailSecurity: formBool(r, "email_security"),
		EmailOrg:      formBool(r, "email_org"),
		EmailProduct:  formBool(r, "email_product"),
		Theme:         formValue(r, "theme"),
		FontSize:      formValue(r, "font_size"),
		ReduceMotion:  formBool(r, "reduce_motion"),
		DateFormat:    formValue(r, "date_format"),
		TimeFormat:    formValue(r, "time_format"),
	}
	if err = h.svc.SavePreferences(r.Context(), submitted.apply(stored)); err != nil {
		h.renderErr(w, r, "settings", "Settings", submitted, err)
		return
	}
	redirect(w, r, "/users/settings")
}

// webSecurityPage renders sessions, tokens and second-factor state.
func (h *Handler) webSecurityPage(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	data, err := h.securityData(r, c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "Security", messageData{}, err)
		return
	}
	h.render(w, r, http.StatusOK, "security", "Security", data)
}

// securityData collects everything the security page shows.
func (h *Handler) securityData(r *http.Request, userID int64) (securityData, error) {
	account, err := h.svc.User(r.Context(), userID)
	if err != nil {
		return securityData{}, err
	}
	enabled, err := h.svc.TwoFactorEnabled(r.Context(), userID)
	if err != nil {
		return securityData{}, err
	}
	sessions, err := h.svc.ListSessions(r.Context(), userID)
	if err != nil {
		return securityData{}, err
	}
	tokens, err := h.svc.ListTokens(r.Context(), userID)
	if err != nil {
		return securityData{}, err
	}
	orgs, err := h.svc.OrgsForUser(r.Context(), userID)
	if err != nil {
		return securityData{}, err
	}

	return securityData{
		Email:            account.Email,
		EmailVerified:    account.EmailVerified,
		TwoFactorEnabled: enabled,
		Sessions:         sessions,
		Tokens:           tokenViews(tokens),
		Orgs:             orgViews(orgs),
	}, nil
}

// webChangePassword replaces the caller's password.
func (h *Handler) webChangePassword(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "Security", messageData{}, err)
		return
	}

	err := h.svc.ChangePassword(r.Context(), c.UserID,
		r.PostFormValue("current_password"), r.PostFormValue("new_password"))
	if err != nil {
		data, loadErr := h.securityData(r, c.UserID)
		if loadErr != nil {
			h.renderErr(w, r, "message", "Security", messageData{}, err)
			return
		}
		h.renderErr(w, r, "security", "Security", data, err)
		return
	}

	// Every session ended with the password, including this one.
	h.clearSession(w, r)
	redirect(w, r, "/server/auth/login")
}

// webTwoFactorStart begins an enrollment and shows the seed once.
func (h *Handler) webTwoFactorStart(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	enrollment, err := h.svc.StartTwoFactor(r.Context(), c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "Two-factor setup", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		}, err)
		return
	}

	h.render(w, r, http.StatusOK, "secret", "Two-factor setup", secretData{
		Secret:     enrollment.Secret,
		Extra:      enrollment.RecoveryCodes,
		ExtraTitle: "Recovery codes",
		ConfirmURL: "/users/security/2fa/confirm",
		LinkURL:    "/users/security",
		LinkText:   "Back to security",
	})
}

// webTwoFactorConfirm proves an enrollment with a code.
func (h *Handler) webTwoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "Two-factor setup", messageData{}, err)
		return
	}
	if err := h.svc.ConfirmTwoFactor(r.Context(), c.UserID, formValue(r, "code")); err != nil {
		h.renderErr(w, r, "message", "Two-factor setup", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		}, err)
		return
	}
	redirect(w, r, "/users/security")
}

// webTwoFactorDisable removes an enrollment.
func (h *Handler) webTwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "Security", messageData{}, err)
		return
	}
	if err := h.svc.DisableTwoFactor(r.Context(), c.UserID, r.PostFormValue("password")); err != nil {
		data, loadErr := h.securityData(r, c.UserID)
		if loadErr != nil {
			h.renderErr(w, r, "message", "Security", messageData{}, err)
			return
		}
		h.renderErr(w, r, "security", "Security", data, err)
		return
	}
	redirect(w, r, "/users/security")
}

// webRevokeSession ends one of the caller's sessions.
func (h *Handler) webRevokeSession(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "session_id")
	if !ok {
		h.render(w, r, http.StatusNotFound, "message", "Not found", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		})
		return
	}
	if err := h.svc.RevokeSession(r.Context(), c.UserID, id); err != nil {
		h.renderErr(w, r, "message", "Security", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		}, err)
		return
	}
	redirect(w, r, "/users/security")
}

// webIssueToken creates an API token and shows the secret once.
func (h *Handler) webIssueToken(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "API token", messageData{}, err)
		return
	}

	access, err := h.svc.AccessBySlug(r.Context(), formValue(r, "org"), c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "API token", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		}, err)
		return
	}

	issued, err := h.svc.IssueToken(r.Context(), c.UserID, service.IssueTokenInput{
		Name:       formValue(r, "name"),
		OrgID:      access.Org.ID,
		Capability: formValue(r, "capability"),
		Role:       formValue(r, "role"),
	})
	if err != nil {
		h.renderErr(w, r, "message", "API token", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		}, err)
		return
	}

	h.render(w, r, http.StatusOK, "secret", "API token", secretData{
		Secret:   issued.Secret,
		LinkURL:  "/users/security",
		LinkText: "Back to security",
	})
}

// webRevokeToken revokes one of the caller's tokens.
func (h *Handler) webRevokeToken(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "token_id")
	if !ok {
		h.render(w, r, http.StatusNotFound, "message", "Not found", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		})
		return
	}
	if err := h.svc.RevokeToken(r.Context(), c.UserID, id); err != nil {
		h.renderErr(w, r, "message", "Security", messageData{
			LinkURL:  "/users/security",
			LinkText: "Back to security",
		}, err)
		return
	}
	redirect(w, r, "/users/security")
}

// webProfile renders a public vanity profile.
func (h *Handler) webProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.svc.UserProfile(r.Context(), r.PathValue("username"), viewerID(r))
	if err != nil {
		h.renderErr(w, r, "message", "Not found", messageData{}, err)
		return
	}
	h.render(w, r, http.StatusOK, "profile", profile.Username, profile)
}

// webOrgsPage lists the caller's organizations.
func (h *Handler) webOrgsPage(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	orgs, err := h.svc.OrgsForUser(r.Context(), c.UserID)
	if err != nil {
		h.renderErr(w, r, "message", "Organizations", messageData{}, err)
		return
	}

	mode := h.svc.CreationMode()
	h.render(w, r, http.StatusOK, "orgs", "Organizations", orgsData{
		Orgs:           orgViews(orgs),
		CanCreate:      mode.SelfServiceAllowed() || mode.InviteAllowed(),
		InviteRequired: !mode.SelfServiceAllowed() && mode.InviteAllowed(),
	})
}

// webCreateOrg starts a shared organization.
func (h *Handler) webCreateOrg(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", "Organizations", messageData{}, err)
		return
	}

	org, err := h.svc.CreateOrg(r.Context(), c.UserID, service.CreateOrgInput{
		Slug:        formValue(r, "slug"),
		Name:        formValue(r, "name"),
		Description: formValue(r, "description"),
		InviteCode:  formValue(r, "invite"),
	})
	if err != nil {
		h.renderErr(w, r, "message", "Organizations", messageData{
			LinkURL:  "/orgs",
			LinkText: "Your organizations",
		}, err)
		return
	}
	redirect(w, r, "/orgs/"+org.Slug)
}

// webAcceptInvite joins the caller to an organization.
func (h *Handler) webAcceptInvite(w http.ResponseWriter, r *http.Request) {
	c, ok := h.webUser(w, r)
	if !ok {
		return
	}
	org, err := h.svc.AcceptInvite(r.Context(), c.UserID, r.PathValue("code"))
	if err != nil {
		h.renderErr(w, r, "message", "Invitation", messageData{
			LinkURL:  "/orgs",
			LinkText: "Your organizations",
		}, err)
		return
	}
	redirect(w, r, "/orgs/"+org.Slug)
}

// webOrgPage renders an organization's members, invitations and domains.
func (h *Handler) webOrgPage(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	data, err := h.orgPageData(r, c, access)
	if err != nil {
		h.renderErr(w, r, "message", access.Org.Name, messageData{}, err)
		return
	}
	h.render(w, r, http.StatusOK, "org", access.Org.Name, data)
}

// orgPageData collects everything the organization page shows.
//
// A listing the caller's role does not reach is left empty rather than
// failing the page, so a viewer sees what they may see and learns
// nothing about the rest.
func (h *Handler) orgPageData(r *http.Request, c caller, access service.OrgAccess) (orgData, error) {
	data := orgData{
		Org:              newOrgView(access.Org),
		Role:             string(access.Role),
		CanManageMembers: access.Can(user.PermMembersManage),
		CanSettings:      access.Can(user.PermOrgSettings),
	}

	members, err := h.svc.Members(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		return orgData{}, err
	}
	for _, m := range members {
		data.Members = append(data.Members, memberView{
			UserID:   m.UserID,
			Username: m.Username,
			Email:    m.Email,
			Role:     m.Role,
			JoinedAt: m.CreatedAt,
		})
	}

	if data.CanManageMembers {
		invites, inviteErr := h.svc.ListInvites(r.Context(), access.Org.ID, c.UserID)
		if inviteErr != nil {
			return orgData{}, inviteErr
		}
		for _, inv := range invites {
			data.Invites = append(data.Invites, newInviteView(inv))
		}
	}

	domains, err := h.svc.ListDomains(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		return orgData{}, err
	}
	for _, d := range domains {
		data.Domains = append(data.Domains, newDomainView(d))
	}

	return data, nil
}

// webSaveOrg writes an organization's profile and visibility.
func (h *Handler) webSaveOrg(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.renderErr(w, r, "message", access.Org.Name, messageData{}, err)
		return
	}

	_, err := h.svc.UpdateOrg(r.Context(), access.Org.ID, c.UserID, service.CreateOrgInput{
		Slug:        access.Org.Slug,
		Name:        formValue(r, "name"),
		Description: formValue(r, "description"),
		Website:     formValue(r, "website"),
		Location:    formValue(r, "location"),
	})
	if err != nil {
		h.orgError(w, r, access, err)
		return
	}
	if visibility := formValue(r, "visibility"); visibility != "" && visibility != access.Org.Visibility {
		if err = h.svc.SetOrgVisibility(r.Context(), access.Org.ID, c.UserID, visibility); err != nil {
			h.orgError(w, r, access, err)
			return
		}
	}
	redirect(w, r, "/orgs/"+access.Org.Slug)
}

// orgError renders a failure on the organization page.
func (h *Handler) orgError(w http.ResponseWriter, r *http.Request, access service.OrgAccess, err error) {
	h.renderErr(w, r, "message", access.Org.Name, messageData{
		LinkURL:  "/orgs/" + access.Org.Slug,
		LinkText: "Back to " + access.Org.Name,
	}, err)
}

// webSetMemberRole changes a member's role.
func (h *Handler) webSetMemberRole(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	target, ok := pathID(r, "member_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}
	if err := h.svc.SetMemberRole(r.Context(), access.Org.ID, c.UserID, target, formValue(r, "role")); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug)
}

// webRemoveMember removes a member.
func (h *Handler) webRemoveMember(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	target, ok := pathID(r, "member_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}
	if err := h.svc.RemoveMember(r.Context(), access.Org.ID, c.UserID, target); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug)
}

// webCreateInvite issues an invitation and shows its code once.
func (h *Handler) webCreateInvite(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.orgError(w, r, access, err)
		return
	}

	maxUses, _ := strconv.Atoi(formValue(r, "max_uses"))
	result, err := h.svc.InviteMember(r.Context(), access.Org.ID, c.UserID, service.InviteInput{
		Email:   formValue(r, "email"),
		Role:    formValue(r, "role"),
		MaxUses: maxUses,
	})
	if err != nil {
		h.orgError(w, r, access, err)
		return
	}

	h.render(w, r, http.StatusOK, "secret", "Invitation", secretData{
		Secret:   result.Code,
		LinkURL:  "/orgs/" + access.Org.Slug,
		LinkText: "Back to " + access.Org.Name,
	})
}

// webRevokeInvite withdraws an invitation.
func (h *Handler) webRevokeInvite(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "invite_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}
	if err := h.svc.RevokeInvite(r.Context(), access.Org.ID, c.UserID, id); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug)
}

// webAudit renders an organization's audit trail.
func (h *Handler) webAudit(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	entries, err := h.svc.OrgAudit(r.Context(), access.Org.ID, c.UserID, maxAuditPage, 0)
	if err != nil {
		h.orgError(w, r, access, err)
		return
	}
	h.render(w, r, http.StatusOK, "audit", "Audit log", auditData{
		Slug:    access.Org.Slug,
		Entries: entries,
	})
}

// webAddDomain claims a custom domain.
func (h *Handler) webAddDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.orgError(w, r, access, err)
		return
	}

	domain, err := h.svc.AddDomain(r.Context(), access.Org.ID, c.UserID, service.AddDomainInput{
		Domain:  formValue(r, "domain"),
		Purpose: formValue(r, "purpose"),
	})
	if err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug+"/domains/"+strconv.FormatInt(domain.ID, 10))
}

// webDomainPage renders a domain's verification instructions and state.
func (h *Handler) webDomainPage(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}

	domains, err := h.svc.ListDomains(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		h.orgError(w, r, access, err)
		return
	}
	var found model.Domain
	for _, d := range domains {
		if d.ID == id {
			found = d
			break
		}
	}
	if found.ID == 0 {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}

	data := domainData{Slug: access.Org.Slug, Domain: newDomainView(found)}
	// The challenge value is fetched only while the domain is still
	// unverified, and only for a caller whose role carries the settings
	// permission the service checks.
	if found.VerificationStatus != model.VerificationVerified {
		if record, recErr := h.svc.VerificationInstructions(r.Context(), access.Org.ID, c.UserID, id); recErr == nil {
			data.Record = record
		}
	}
	h.render(w, r, http.StatusOK, "domain", found.Name, data)
}

// webSaveDomain changes what a domain is used for.
func (h *Handler) webSaveDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	if err := parseForm(r); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}
	err := h.svc.SetDomainPurpose(r.Context(), access.Org.ID, c.UserID, id, formValue(r, "purpose"))
	if err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug+"/domains/"+strconv.FormatInt(id, 10))
}

// webVerifyDomain checks a published record and activates the domain.
func (h *Handler) webVerifyDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}
	if _, err := h.svc.VerifyDomain(r.Context(), access.Org.ID, c.UserID, id); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug+"/domains/"+strconv.FormatInt(id, 10))
}

// webRemoveDomain releases a custom domain.
func (h *Handler) webRemoveDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.webOrg(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		h.orgError(w, r, access, service.ErrNotFound)
		return
	}
	if err := h.svc.RemoveDomain(r.Context(), access.Org.ID, c.UserID, id); err != nil {
		h.orgError(w, r, access, err)
		return
	}
	redirect(w, r, "/orgs/"+access.Org.Slug)
}
