package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/server/service"
)

// API returns the versioned REST surface for PART 34, 35 and 36.
//
// The mux is built from the same table that produces the OpenAPI and
// GraphQL documents, so PART 14's requirement that all three surfaces
// describe one API holds by construction: a route cannot be served
// without being documented, and none can be documented without being
// served.
//
// Routes are registered under their full absolute paths so the mux sees
// exactly the path the client sent. Mounting a subtree without rewriting
// the path keeps one routing table for the whole server instead of a
// private one per feature.
func (h *Handler) API() http.Handler {
	base := h.apiBase()
	mux := http.NewServeMux()

	for _, rt := range apiRoutes {
		mux.HandleFunc(rt.op.Method+" "+rt.op.FullPath(base), rt.handler(h))
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_ = apierror.SendErrorCode(w, apierror.CodeNotFound)
	})

	return mux
}

// APIPrefixes returns the patterns the main router mounts this handler
// on. A bare prefix and its slashed form are both listed because Go's
// mux treats them as different patterns.
func (h *Handler) APIPrefixes() []string {
	base := h.apiBase()
	return []string{
		base + "/server/auth/",
		base + "/users",
		base + "/users/",
		base + "/orgs",
		base + "/orgs/",
	}
}

// userSubResources are the fixed segments under the user scope. They are
// listed so a single trailing segment that is not one of them can be
// recognized as a vanity username.
var userSubResources = map[string]bool{
	"settings": true,
	"security": true,
	"sessions": true,
	"tokens":   true,
}

// IsPublicPath reports whether a path may be reached without a
// credential.
//
// Only the authentication flows, the public vanity profiles, and the
// server-rendered pages that lead to them qualify. Everything else
// answers unauthorized before a handler runs, so a missing credential
// can never be mistaken for an anonymous one.
//
// The answer covers this handler's own surfaces only. A path belonging
// to another PART is reported public here because this handler has no
// standing to refuse it; that PART guards its own routes.
func (h *Handler) IsPublicPath(path string) bool {
	base := h.apiBase()
	clean := strings.TrimSuffix(path, "/")

	if !h.owns(clean) {
		return true
	}
	// The REST tree is refused up front, because an API client that is
	// missing a credential wants to be told so, not redirected to a
	// sign-in page it cannot fill in. Only the authentication flows and
	// the public vanity profiles are open.
	if strings.HasPrefix(clean, base+"/") {
		if strings.HasPrefix(clean, base+"/server/auth/") {
			return true
		}
		if rest, ok := strings.CutPrefix(clean, base+"/users/"); ok {
			return rest != "" && !strings.Contains(rest, "/") && !userSubResources[rest]
		}
		if rest, ok := strings.CutPrefix(clean, base+"/orgs/"); ok {
			return rest != "" && !strings.Contains(rest, "/")
		}
		return false
	}

	// Every server-rendered page is reachable without a credential, and
	// each one decides for itself what an anonymous visitor gets. A
	// browser asking for a page it is not signed in for must land on the
	// sign-in form, not on a bare 401 with no way forward, and only the
	// page handler can send that redirect. The middleware still resolves
	// a session when the request carries one, so a signed-in visitor is
	// recognised on exactly the same paths.
	return true
}

// owns reports whether a path is served by this handler at all.
func (h *Handler) owns(path string) bool {
	for _, prefix := range append(h.APIPrefixes(), h.WebPrefixes()...) {
		if strings.HasSuffix(prefix, "/") {
			if strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		if path == prefix {
			return true
		}
	}
	return false
}

// requireUser resolves the caller or writes the unauthorized envelope.
func (h *Handler) requireUser(w http.ResponseWriter, r *http.Request) (caller, bool) {
	c, ok := currentUser(r)
	if !ok {
		_ = apierror.SendErrorCode(w, apierror.CodeUnauthorized)
		return caller{}, false
	}
	if c.Pending {
		_ = apierror.SendErrorCode(w, apierror.CodeTwoFactorRequired)
		return caller{}, false
	}
	return c, true
}

// requireSession resolves a caller that authenticated with a browser
// session rather than an API token.
//
// Credential management, password changes and second-factor enrollment
// are refused to a token: a token able to mint or disable the
// credentials above it would defeat the scoping that limits it.
func (h *Handler) requireSession(w http.ResponseWriter, r *http.Request) (caller, bool) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return caller{}, false
	}
	if c.Token {
		_ = apierror.SendErrorCode(w, apierror.CodeForbidden)
		return caller{}, false
	}
	return c, true
}

// orgScope resolves the {slug} segment into the caller's membership.
func (h *Handler) orgScope(w http.ResponseWriter, r *http.Request) (caller, service.OrgAccess, bool) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return caller{}, service.OrgAccess{}, false
	}
	access, err := h.svc.AccessBySlug(r.Context(), r.PathValue("slug"), c.UserID)
	if err != nil {
		sendErr(w, err)
		return caller{}, service.OrgAccess{}, false
	}
	return c, access, true
}

// badRequest writes the malformed-body envelope.
func badRequest(w http.ResponseWriter) {
	_ = apierror.SendErrorCode(w, apierror.CodeBadRequest)
}

// notFound writes the not-found envelope, which is also the answer for a
// path segment that should be a number and is not.
func notFound(w http.ResponseWriter) {
	_ = apierror.SendErrorCode(w, apierror.CodeNotFound)
}

// userView is the JSON shape of an account.
type userView struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	DisplayName   string    `json:"display_name"`
	Bio           string    `json:"bio,omitempty"`
	Location      string    `json:"location,omitempty"`
	Website       string    `json:"website,omitempty"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	Timezone      string    `json:"timezone,omitempty"`
	Language      string    `json:"language,omitempty"`
	Visibility    string    `json:"visibility"`
	OrgVisibility bool      `json:"org_visibility"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// newUserView renders an account for its owner. The password hash and
// the lockout counters are absent by construction, not by omission.
func newUserView(u model.User) userView {
	return userView{
		ID:            u.ID,
		Username:      u.Username,
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
		DisplayName:   u.DisplayName,
		Bio:           u.Bio,
		Location:      u.Location,
		Website:       u.Website,
		AvatarURL:     u.AvatarURL,
		Timezone:      u.Timezone,
		Language:      u.Language,
		Visibility:    u.Visibility,
		OrgVisibility: u.OrgVisibility,
		Status:        u.Status,
		CreatedAt:     u.CreatedAt,
	}
}

// orgView is the JSON shape of an organization.
type orgView struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Website     string    `json:"website,omitempty"`
	Location    string    `json:"location,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Visibility  string    `json:"visibility"`
	Personal    bool      `json:"personal"`
	OwnerID     int64     `json:"owner_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// newOrgView renders an organization.
func newOrgView(o model.Org) orgView {
	return orgView{
		ID:          o.ID,
		Slug:        o.Slug,
		Name:        o.Name,
		Description: o.Description,
		Website:     o.Website,
		Location:    o.Location,
		AvatarURL:   o.AvatarURL,
		Visibility:  o.Visibility,
		Personal:    o.Personal,
		OwnerID:     o.OwnerID,
		Status:      o.Status,
		CreatedAt:   o.CreatedAt,
	}
}

// orgViews renders a list of organizations.
func orgViews(list []model.Org) []orgView {
	out := make([]orgView, 0, len(list))
	for _, o := range list {
		out = append(out, newOrgView(o))
	}
	return out
}

// memberView is the JSON shape of an organization membership.
type memberView struct {
	UserID   int64     `json:"user_id"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// inviteView is the JSON shape of a pending invitation.
//
// The code is absent: only its hash is stored, and the plaintext exists
// exactly once, in the response that created it.
type inviteView struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email,omitempty"`
	Role      string    `json:"role"`
	MaxUses   int       `json:"max_uses"`
	UseCount  int       `json:"use_count"`
	InvitedBy int64     `json:"invited_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// newInviteView renders an invitation without its code.
func newInviteView(inv model.Invite) inviteView {
	return inviteView{
		ID:        inv.ID,
		Email:     inv.Email,
		Role:      inv.Role,
		MaxUses:   inv.MaxUses,
		UseCount:  inv.UseCount,
		InvitedBy: inv.InvitedBy,
		CreatedAt: inv.CreatedAt,
		ExpiresAt: inv.ExpiresAt,
	}
}

// tokenView is the JSON shape of an API token.
type tokenView struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Prefix     string    `json:"prefix"`
	Scope      string    `json:"scope"`
	Role       string    `json:"role,omitempty"`
	OrgID      int64     `json:"org_id,omitempty"`
	ZoneID     int64     `json:"zone_id,omitempty"`
	Capability string    `json:"capability,omitempty"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// newTokenView renders a token record without its hash.
func newTokenView(t model.Token) tokenView {
	return tokenView{
		ID:         t.ID,
		Name:       t.Name,
		Prefix:     t.Prefix,
		Scope:      t.Scope,
		Role:       t.Role,
		OrgID:      t.OrgID,
		ZoneID:     t.ZoneID,
		Capability: t.Capability,
		LastUsedAt: t.LastUsedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
		CreatedAt:  t.CreatedAt,
	}
}

// tokenViews renders a list of token records.
func tokenViews(list []model.Token) []tokenView {
	out := make([]tokenView, 0, len(list))
	for _, t := range list {
		out = append(out, newTokenView(t))
	}
	return out
}

// domainView is the JSON shape of a custom domain.
type domainView struct {
	ID                 int64     `json:"id"`
	OrgID              int64     `json:"org_id"`
	Domain             string    `json:"domain"`
	Purpose            string    `json:"purpose"`
	IsApex             bool      `json:"is_apex"`
	IsWildcard         bool      `json:"is_wildcard"`
	VerificationStatus string    `json:"verification_status"`
	VerifiedAt         time.Time `json:"verified_at,omitempty"`
	SSLEnabled         bool      `json:"ssl_enabled"`
	SSLStatus          string    `json:"ssl_status"`
	SSLExpiresAt       time.Time `json:"ssl_expires_at,omitempty"`
	Status             string    `json:"status"`
	SuspendReason      string    `json:"suspend_reason,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// newDomainView renders a custom domain.
//
// The verification token is deliberately absent. It is served only by
// the verification-instructions route, which requires the organization
// settings permission, so a viewer cannot read the challenge value out
// of an ordinary listing.
func newDomainView(d model.Domain) domainView {
	return domainView{
		ID:                 d.ID,
		OrgID:              d.OrgID,
		Domain:             d.Name,
		Purpose:            d.Purpose,
		IsApex:             d.IsApex,
		IsWildcard:         d.IsWildcard,
		VerificationStatus: d.VerificationStatus,
		VerifiedAt:         d.VerifiedAt,
		SSLEnabled:         d.SSLEnabled,
		SSLStatus:          d.SSLStatus,
		SSLExpiresAt:       d.SSLExpiresAt,
		Status:             d.Status,
		SuspendReason:      d.SuspendReason,
		CreatedAt:          d.CreatedAt,
	}
}

// registerBody is the account-creation payload.
type registerBody struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Invite   string `json:"invite"`
}

// readRegister fills a registration payload from either encoding.
func readRegister(r *http.Request) (registerBody, error) {
	var body registerBody
	form, err := bind(r, &body)
	if err != nil {
		return body, err
	}
	if form {
		body.Username = formValue(r, "username")
		body.Email = formValue(r, "email")
		body.Password = r.PostFormValue("password")
		body.Invite = formValue(r, "invite")
	}
	return body, nil
}

// apiRegister creates a Regular User account.
func (h *Handler) apiRegister(w http.ResponseWriter, r *http.Request) {
	body, err := readRegister(r)
	if err != nil {
		badRequest(w)
		return
	}
	h.register(w, r, body)
}

// apiRegisterWithInvite creates an account from an invite link, which
// carries the code in the path rather than the body.
func (h *Handler) apiRegisterWithInvite(w http.ResponseWriter, r *http.Request) {
	body, err := readRegister(r)
	if err != nil {
		badRequest(w)
		return
	}
	body.Invite = r.PathValue("code")
	h.register(w, r, body)
}

// register performs the account creation both registration routes share.
func (h *Handler) register(w http.ResponseWriter, r *http.Request, body registerBody) {
	account, err := h.svc.Register(r.Context(), service.RegisterInput{
		Username:   body.Username,
		Email:      body.Email,
		Password:   body.Password,
		InviteCode: body.Invite,
	})
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newUserView(account))
}

// apiLogin verifies a password and opens a session.
func (h *Handler) apiLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Identifier = formValue(r, "identifier")
		body.Password = r.PostFormValue("password")
	}

	result, err := h.svc.Login(r.Context(), service.LoginInput{
		Identifier: body.Identifier,
		Password:   body.Password,
		IP:         h.clientIP(r),
		UserAgent:  r.UserAgent(),
	})
	if err != nil {
		sendErr(w, err)
		return
	}

	h.setSession(w, r, result.SessionToken)
	sendOK(w, map[string]any{
		"user":               newUserView(result.User),
		"two_factor_pending": result.TwoFactorPending,
	})
}

// apiTwoFactorChallenge completes a sign-in that is waiting on a code.
func (h *Handler) apiTwoFactorChallenge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Code = formValue(r, "code")
	}

	token := h.sessionToken(r)
	if token == "" {
		_ = apierror.SendErrorCode(w, apierror.CodeUnauthorized)
		return
	}

	account, _, err := h.svc.ResolveSession(r.Context(), token)
	if err != nil {
		sendErr(w, err)
		return
	}
	if err = h.svc.VerifyTwoFactor(r.Context(), account.ID, token, body.Code); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newUserView(account))
}

// apiLogout ends the current session.
func (h *Handler) apiLogout(w http.ResponseWriter, r *http.Request) {
	if token := h.sessionToken(r); token != "" {
		if err := h.svc.Logout(r.Context(), token); err != nil {
			sendErr(w, err)
			return
		}
	}
	h.clearSession(w, r)
	sendOK(w, map[string]any{"signed_out": true})
}

// apiPasswordForgot issues a reset token.
//
// The response is identical whether or not an account matched, so the
// endpoint cannot be used to test which addresses are registered.
func (h *Handler) apiPasswordForgot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Identifier string `json:"identifier"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Identifier = formValue(r, "identifier")
	}

	if _, _, err = h.svc.StartPasswordReset(r.Context(), body.Identifier); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"sent": true})
}

// apiPasswordReset sets a new password from a reset token.
func (h *Handler) apiPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Token = formValue(r, "token")
		body.Password = r.PostFormValue("password")
	}

	if err = h.svc.CompletePasswordReset(r.Context(), body.Token, body.Password); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"updated": true})
}

// apiEmailVerify confirms an address from a verification token.
func (h *Handler) apiEmailVerify(w http.ResponseWriter, r *http.Request) {
	account, err := h.svc.CompleteEmailVerification(r.Context(), r.PathValue("code"))
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newUserView(account))
}

// apiMe returns the signed-in account.
func (h *Handler) apiMe(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	account, err := h.svc.User(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newUserView(account))
}

// apiUpdateProfile writes the caller's own profile.
func (h *Handler) apiUpdateProfile(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var body struct {
		DisplayName       string `json:"display_name"`
		Bio               string `json:"bio"`
		Location          string `json:"location"`
		Website           string `json:"website"`
		AvatarURL         string `json:"avatar_url"`
		Timezone          string `json:"timezone"`
		Language          string `json:"language"`
		NotificationEmail string `json:"notification_email"`
		Visibility        string `json:"visibility"`
		OrgVisibility     bool   `json:"org_visibility"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.DisplayName = formValue(r, "display_name")
		body.Bio = formValue(r, "bio")
		body.Location = formValue(r, "location")
		body.Website = formValue(r, "website")
		body.AvatarURL = formValue(r, "avatar_url")
		body.Timezone = formValue(r, "timezone")
		body.Language = formValue(r, "language")
		body.NotificationEmail = formValue(r, "notification_email")
		body.Visibility = formValue(r, "visibility")
		body.OrgVisibility = formBool(r, "org_visibility")
	}

	account, err := h.svc.UpdateProfile(r.Context(), c.UserID, service.ProfileInput{
		DisplayName:       body.DisplayName,
		Bio:               body.Bio,
		Location:          body.Location,
		Website:           body.Website,
		AvatarURL:         body.AvatarURL,
		Timezone:          body.Timezone,
		Language:          body.Language,
		NotificationEmail: body.NotificationEmail,
		Visibility:        body.Visibility,
		OrgVisibility:     body.OrgVisibility,
	})
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newUserView(account))
}

// prefsView is the JSON shape of a user's preferences.
type prefsView struct {
	ShowEmail     bool   `json:"show_email"`
	ShowActivity  bool   `json:"show_activity"`
	ShowOrgs      bool   `json:"show_orgs"`
	Searchable    bool   `json:"searchable"`
	EmailSecurity bool   `json:"email_security"`
	EmailOrg      bool   `json:"email_org"`
	EmailProduct  bool   `json:"email_product"`
	Theme         string `json:"theme"`
	FontSize      string `json:"font_size"`
	ReduceMotion  bool   `json:"reduce_motion"`
	DateFormat    string `json:"date_format"`
	TimeFormat    string `json:"time_format"`
}

// newPrefsView renders stored preferences.
func newPrefsView(p model.Preferences) prefsView {
	return prefsView{
		ShowEmail:     p.ShowEmail,
		ShowActivity:  p.ShowActivity,
		ShowOrgs:      p.ShowOrgs,
		Searchable:    p.Searchable,
		EmailSecurity: p.EmailSecurity,
		EmailOrg:      p.EmailOrg,
		EmailProduct:  p.EmailProduct,
		Theme:         p.Theme,
		FontSize:      p.FontSize,
		ReduceMotion:  p.ReduceMotion,
		DateFormat:    p.DateFormat,
		TimeFormat:    p.TimeFormat,
	}
}

// apply copies a submitted view onto a stored row.
//
// The owner is left untouched so a caller cannot rewrite somebody else's
// preferences by naming them in the body.
func (v prefsView) apply(p model.Preferences) model.Preferences {
	p.ShowEmail = v.ShowEmail
	p.ShowActivity = v.ShowActivity
	p.ShowOrgs = v.ShowOrgs
	p.Searchable = v.Searchable
	p.EmailSecurity = v.EmailSecurity
	p.EmailOrg = v.EmailOrg
	p.EmailProduct = v.EmailProduct
	p.Theme = v.Theme
	p.FontSize = v.FontSize
	p.ReduceMotion = v.ReduceMotion
	p.DateFormat = v.DateFormat
	p.TimeFormat = v.TimeFormat
	return p
}

// apiPreferences returns the caller's preferences.
func (h *Handler) apiPreferences(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	prefs, err := h.svc.Preferences(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newPrefsView(prefs))
}

// apiSavePreferences writes the caller's preferences.
func (h *Handler) apiSavePreferences(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	stored, err := h.svc.Preferences(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	// The current values are the starting point, so a partial JSON body
	// changes only what it names. A form submission replaces the whole
	// set, because an unchecked box is reported by absence and there is
	// no way to tell it apart from a field the caller left alone.
	body := newPrefsView(stored)
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body = prefsView{
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
	}

	if err = h.svc.SavePreferences(r.Context(), body.apply(stored)); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, body)
}

// apiSecurity summarizes the caller's authentication state.
func (h *Handler) apiSecurity(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	account, err := h.svc.User(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	enabled, err := h.svc.TwoFactorEnabled(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	sessions, err := h.svc.ListSessions(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	tokens, err := h.svc.ListTokens(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	sendOK(w, map[string]any{
		"email":              account.Email,
		"email_verified":     account.EmailVerified,
		"two_factor_enabled": enabled,
		"last_login_at":      account.LastLoginAt,
		"active_sessions":    len(sessions),
		"api_tokens":         len(tokens),
	})
}

// apiChangePassword replaces the caller's password.
func (h *Handler) apiChangePassword(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Current string `json:"current_password"`
		Next    string `json:"new_password"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Current = r.PostFormValue("current_password")
		body.Next = r.PostFormValue("new_password")
	}

	if err = h.svc.ChangePassword(r.Context(), c.UserID, body.Current, body.Next); err != nil {
		sendErr(w, err)
		return
	}

	// A password change ends every session the account had, including
	// the one that made this request.
	h.clearSession(w, r)
	sendOK(w, map[string]any{"updated": true})
}

// apiStartEmailVerification issues a verification token for an address.
func (h *Handler) apiStartEmailVerification(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Email = formValue(r, "email")
	}

	if _, err = h.svc.StartEmailVerification(r.Context(), c.UserID, body.Email); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"sent": true})
}

// apiSessions lists the caller's active sessions.
func (h *Handler) apiSessions(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	sessions, err := h.svc.ListSessions(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	// The stored hash is omitted: it is half of a live credential and
	// belongs in no response.
	out := make([]map[string]any, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, map[string]any{
			"id":             sess.ID,
			"ip_address":     sess.IP,
			"user_agent":     sess.UserAgent,
			"two_factor_ok":  sess.TwoFactorOK,
			"created_at":     sess.CreatedAt,
			"last_active_at": sess.LastActive,
			"expires_at":     sess.ExpiresAt,
		})
	}
	sendOK(w, out)
}

// apiRevokeSession ends one of the caller's sessions.
func (h *Handler) apiRevokeSession(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "session_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RevokeSession(r.Context(), c.UserID, id); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"revoked": true})
}

// apiTwoFactorStart begins enrollment and returns the provisioning data.
func (h *Handler) apiTwoFactorStart(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	enrollment, err := h.svc.StartTwoFactor(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	// The seed and the recovery codes are shown here once. Only their
	// encrypted and hashed forms survive the request.
	sendOK(w, map[string]any{
		"secret":         enrollment.Secret,
		"uri":            enrollment.URI,
		"recovery_codes": enrollment.RecoveryCodes,
	})
}

// apiTwoFactorConfirm proves an enrollment with a code.
func (h *Handler) apiTwoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Code string `json:"code"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Code = formValue(r, "code")
	}

	if err = h.svc.ConfirmTwoFactor(r.Context(), c.UserID, body.Code); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"two_factor_enabled": true})
}

// apiTwoFactorDisable removes an enrollment.
func (h *Handler) apiTwoFactorDisable(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Password = r.PostFormValue("password")
	}

	if err = h.svc.DisableTwoFactor(r.Context(), c.UserID, body.Password); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"two_factor_enabled": false})
}

// apiListTokens lists the caller's API tokens.
func (h *Handler) apiListTokens(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tokens, err := h.svc.ListTokens(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, tokenViews(tokens))
}

// apiIssueToken creates an API token.
func (h *Handler) apiIssueToken(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Name       string `json:"name"`
		Org        string `json:"org"`
		ZoneID     int64  `json:"zone_id"`
		Capability string `json:"capability"`
		Role       string `json:"role"`
		Scope      string `json:"scope"`
		ExpiresIn  int    `json:"expires_in_days"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Name = formValue(r, "name")
		body.Org = formValue(r, "org")
		body.ZoneID, _ = strconv.ParseInt(formValue(r, "zone_id"), 10, 64)
		body.Capability = formValue(r, "capability")
		body.Role = formValue(r, "role")
		body.Scope = formValue(r, "scope")
		body.ExpiresIn, _ = strconv.Atoi(formValue(r, "expires_in_days"))
	}

	// The organization is named by slug, matching how every other route
	// addresses one, and resolved through the caller's own membership so
	// an unknown slug and a slug they cannot reach answer alike.
	access, err := h.svc.AccessBySlug(r.Context(), body.Org, c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	issued, err := h.svc.IssueToken(r.Context(), c.UserID, service.IssueTokenInput{
		Name:       body.Name,
		OrgID:      access.Org.ID,
		ZoneID:     body.ZoneID,
		Capability: body.Capability,
		Role:       body.Role,
		Scope:      body.Scope,
		ExpiresIn:  time.Duration(body.ExpiresIn) * 24 * time.Hour,
	})
	if err != nil {
		sendErr(w, err)
		return
	}

	// The secret appears in this response and nowhere else, ever.
	sendOK(w, map[string]any{
		"token":  newTokenView(issued.Token),
		"secret": issued.Secret,
	})
}

// apiRevokeToken revokes one of the caller's tokens.
func (h *Handler) apiRevokeToken(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "token_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RevokeToken(r.Context(), c.UserID, id); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"revoked": true})
}

// apiUserProfile serves a user's public vanity profile.
func (h *Handler) apiUserProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.svc.UserProfile(r.Context(), r.PathValue("username"), viewerID(r))
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{
		"username":     profile.Username,
		"display_name": profile.DisplayName,
		"bio":          profile.Bio,
		"location":     profile.Location,
		"website":      profile.Website,
		"avatar_url":   profile.AvatarURL,
		"email":        profile.Email,
		"orgs":         orgViews(profile.Orgs),
	})
}
