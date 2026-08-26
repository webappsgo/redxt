package handler

import (
	"context"
	"strconv"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/service"
)

// ScopeTwoFactorPending marks a session that has signed in with a
// password but has not yet answered its second factor.
//
// Such a session exists — the challenge route needs it — but it stands
// for nothing else, so it is carried as a scope the request helpers
// refuse rather than as an ordinary identity.
const ScopeTwoFactorPending = "2fa_pending"

// Verifier resolves Regular User credentials for the PART 12
// authentication stage.
//
// It answers only for Regular Users. Server Admins live in their own
// table with their own sessions, and nothing here can produce an admin
// identity, which is what keeps the two account systems separate at the
// authentication layer and not merely in storage.
type Verifier struct {
	svc *service.Service
	// cookieName is the Regular User session cookie. A session presented
	// under any other cookie is not this verifier's to resolve.
	cookieName string
}

// NewVerifier builds the credential verifier for Regular Users.
func NewVerifier(svc *service.Service, cookieName string) *Verifier {
	return &Verifier{svc: svc, cookieName: cookieName}
}

// Ensure the verifier satisfies the interface the Auth stage expects.
var _ middleware.TokenVerifier = (*Verifier)(nil)

// VerifySession resolves a browser session cookie.
//
// A cookie under any other name is declined without a lookup, so an
// admin session value can never be resolved into a Regular User.
func (v *Verifier) VerifySession(ctx context.Context, cookieName, value string) (middleware.AuthInfo, bool) {
	if v == nil || v.svc == nil || cookieName != v.cookieName || value == "" {
		return middleware.AuthInfo{}, false
	}

	account, session, err := v.svc.ResolveSession(ctx, value)
	if err != nil {
		return middleware.AuthInfo{}, false
	}

	info := middleware.AuthInfo{
		Subject:   strconv.FormatInt(account.ID, 10),
		Kind:      middleware.SubjectUser,
		Source:    "cookie:" + cookieName,
		SessionID: strconv.FormatInt(session.ID, 10),
	}

	// A session that owes a second factor is reported as pending rather
	// than refused: the challenge route has to be able to find it, and
	// every other route has to refuse it.
	if !session.TwoFactorOK {
		enabled, twoFactorErr := v.svc.TwoFactorEnabled(ctx, account.ID)
		if twoFactorErr != nil {
			return middleware.AuthInfo{}, false
		}
		if enabled {
			info.Scopes = []string{ScopeTwoFactorPending}
		}
	}

	return info, true
}

// VerifyToken resolves an API token.
//
// The identity carries the permissions the credential actually grants —
// the token's own narrowing capped by the role of the member who issued
// it — so a handler asking what a token may do gets the capped answer,
// never the issuer's full standing.
func (v *Verifier) VerifyToken(ctx context.Context, token, source string) (middleware.AuthInfo, bool) {
	if v == nil || v.svc == nil || token == "" {
		return middleware.AuthInfo{}, false
	}

	auth, err := v.svc.AuthenticateToken(ctx, token)
	if err != nil {
		return middleware.AuthInfo{}, false
	}
	// Only a token a Regular User owns resolves to a Regular User. An
	// organization-owned or admin-owned credential is somebody else's to
	// answer for, and answering for it here would hand it a user identity
	// it was never issued.
	if auth.Token.OwnerType != string(security.OwnerUser) {
		return middleware.AuthInfo{}, false
	}

	scopes := make([]string, 0, len(auth.Role.Permissions())+1)
	for _, permission := range auth.Role.Permissions() {
		scopes = append(scopes, string(permission))
	}
	if auth.Token.Capability != "" {
		scopes = append(scopes, auth.Token.Capability)
	}

	return middleware.AuthInfo{
		Subject: strconv.FormatInt(auth.Token.OwnerID, 10),
		Kind:    middleware.SubjectUser,
		Source:  source,
		Scopes:  scopes,
	}, true
}
