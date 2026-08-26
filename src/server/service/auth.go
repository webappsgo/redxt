package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/user"
)

// dummyHash is a valid Argon2id encoding of a password nobody holds.
//
// A sign-in attempt for an account that does not exist verifies against
// this hash instead of returning early. Without it, an unknown username
// answers in microseconds while a known one spends the full Argon2id
// cost, and that difference alone enumerates the user table. PART 11
// requires the two paths to be indistinguishable, and the only reliable
// way to make them so is to do the same work in both.
var dummyHash = mustDummyHash()

// mustDummyHash computes the comparison hash once at startup.
func mustDummyHash() string {
	hash, err := security.HashPassword("redxt-timing-equalizer")
	if err != nil {
		return ""
	}
	return hash
}

// passwordPolicy builds the PART 34 password rules from configuration.
func (s *Service) passwordPolicy() user.PasswordPolicy {
	auth := s.users().Auth
	return user.PasswordPolicy{
		MinLength:        auth.PasswordMinLength,
		RequireUppercase: auth.PasswordRequireUppercase,
		RequireLowercase: auth.PasswordRequireLowercase,
		RequireNumber:    auth.PasswordRequireNumber,
		RequireSpecial:   auth.PasswordRequireSpecial,
	}
}

// RegistrationMode returns the effective PART 34 registration mode. An
// unparsable configured value falls back to disabled rather than to a
// permissive default, so a typo closes registration instead of opening
// it.
func (s *Service) RegistrationMode() user.RegistrationMode {
	mode, err := user.ParseRegistrationMode(s.users().Registration.Mode)
	if err != nil {
		return user.RegistrationDisabled
	}
	return mode
}

// RegisterInput is one self-service or admin-initiated signup.
type RegisterInput struct {
	Username string
	Email    string
	Password string
	// InviteCode is the plaintext invite. It is required in invite mode
	// and ignored otherwise.
	InviteCode string
	// ByAdmin marks a registration performed by a Server Admin, which is
	// the only path allowed in admin_only mode.
	ByAdmin bool
}

// Register creates a Regular User account and its personal organization.
//
// The account and the organization are created together because PART 35
// gives every user a personal org and redxt attaches every zone, policy,
// key and DDNS host to an organization. A user without one would have
// nowhere to put anything.
func (s *Service) Register(ctx context.Context, in RegisterInput) (model.User, error) {
	if !s.users().Enabled {
		return model.User{}, ErrDisabled
	}

	// Which policy a signup must satisfy is decided by how it arrived,
	// not by the mode alone. Open mode both accepts invites and allows
	// unaided signup, so testing the invite policy first would demand a
	// code from every visitor to an open server, which PART 34 lists as
	// optional there rather than required.
	mode := s.RegistrationMode()
	switch {
	case in.ByAdmin:
		if !mode.AdminCreateAllowed() {
			return model.User{}, ErrDisabled
		}
	case in.InviteCode != "":
		if !mode.InviteAllowed() {
			return model.User{}, ErrDisabled
		}
	case !mode.SelfServiceAllowed():
		return model.User{}, ErrForbidden
	}

	username, err := user.ValidateName(in.Username)
	if err != nil {
		return model.User{}, validationError(err.Error())
	}
	email, err := user.ValidateEmail(in.Email)
	if err != nil {
		return model.User{}, validationError(err.Error())
	}
	if err = s.passwordPolicy().ValidatePassword(in.Password); err != nil {
		return model.User{}, validationError(err.Error())
	}

	reg := s.users().Registration
	if !user.EmailDomainAllowed(email, reg.AllowedDomains, reg.BlockedDomains) {
		return model.User{}, validationError("email domain is not accepted")
	}

	// An invite is consumed before the account is written. Redeeming
	// first means a failed account insert wastes a use, which is the
	// safe direction: the alternative lets two racing signups share one
	// single-use invite.
	// Only a signup that actually presented a code redeems one. Open
	// mode accepts invites without requiring them, so keying this on the
	// mode instead of the code would send every unaided signup to redeem
	// the empty string.
	var invite model.Invite
	if !in.ByAdmin && in.InviteCode != "" {
		invite, err = s.redeemInvite(ctx, in.InviteCode)
		if err != nil {
			return model.User{}, err
		}
	}

	hash, err := security.HashPassword(in.Password)
	if err != nil {
		return model.User{}, err
	}

	status := model.StatusActive
	if reg.RequireEmailVerification && !in.ByAdmin {
		status = model.StatusPending
	}

	created, err := s.store.CreateUser(ctx, model.User{
		Username:      username,
		Email:         email,
		PasswordHash:  hash,
		DisplayName:   username,
		Role:          string(user.RoleViewer),
		Visibility:    s.defaultUserVisibility(),
		OrgVisibility: true,
		Status:        status,
	})
	if err != nil {
		return model.User{}, mapStoreErr(err)
	}

	if _, err = s.createPersonalOrg(ctx, created); err != nil {
		return model.User{}, err
	}

	// An invite naming an organization also grants membership in it, so
	// the invitee lands inside the team that invited them rather than
	// alone in their personal org.
	if invite.OrgID != 0 {
		if err = s.joinInvitedOrg(ctx, invite, created); err != nil {
			return model.User{}, err
		}
	}

	return created, nil
}

// defaultUserVisibility returns the configured profile visibility,
// falling back to private when the value is unrecognized: an unknown
// setting must not publish a profile the operator did not ask to
// publish.
func (s *Service) defaultUserVisibility() string {
	if s.users().Profile.DefaultVisibility == model.VisibilityPublic {
		return model.VisibilityPublic
	}
	return model.VisibilityPrivate
}

// LoginInput is one sign-in attempt.
type LoginInput struct {
	// Identifier is a username, an email address, or a numeric user id.
	Identifier string
	Password   string
	IP         string
	UserAgent  string
}

// LoginResult carries the outcome of a successful sign-in.
type LoginResult struct {
	User model.User
	// SessionToken is the plaintext session value for the cookie. Only
	// its SHA-256 is stored, so this is the one moment it exists.
	SessionToken string
	Session      model.Session
	// TwoFactorPending reports that a TOTP code is still required before
	// the session is fully authenticated.
	TwoFactorPending bool
}

// Login verifies a password and opens a session.
//
// Every failure mode returns ErrInvalidCredentials and takes the same
// Argon2id verification path, so an unknown account, a wrong password
// and a suspended account are indistinguishable to a caller. The one
// exception is a locked account, which must be reported so a legitimate
// user learns why waiting is required — and lockout is only reachable
// after the correct identifier has already been supplied repeatedly, so
// it reveals nothing an attacker did not already establish.
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	if !s.users().Enabled {
		return LoginResult{}, ErrDisabled
	}

	account, found := s.lookupIdentifier(ctx, in.Identifier)

	stored := account.PasswordHash
	if !found || stored == "" {
		stored = dummyHash
	}
	ok, verifyErr := security.VerifyPassword(in.Password, stored)
	if verifyErr != nil {
		ok = false
	}

	if !found {
		return LoginResult{}, ErrInvalidCredentials
	}

	now := s.now()
	if account.Locked(now) {
		return LoginResult{}, ErrAccountLocked
	}

	if !ok {
		auth := s.users().Auth
		if failErr := s.store.RecordLoginFailure(ctx, account.ID,
			auth.MaxFailedLogins, auth.LockoutDuration.Duration()); failErr != nil {
			return LoginResult{}, failErr
		}
		return LoginResult{}, ErrInvalidCredentials
	}

	// A suspended or unverified account is rejected only after the
	// password has been checked, so the rejection cannot be used to
	// discover which accounts exist in which state.
	if !account.Active() {
		return LoginResult{}, ErrInvalidCredentials
	}

	// A password verified against outdated Argon2id parameters is
	// rehashed while the plaintext is briefly available. Skipping this
	// would leave old accounts permanently at the weaker cost.
	if needs, rehashErr := security.NeedsRehash(stored, security.DefaultParams()); rehashErr == nil && needs {
		if newHash, hashErr := security.HashPassword(in.Password); hashErr == nil {
			_ = s.store.UpdatePassword(ctx, account.ID, newHash)
		}
	}

	if err := s.store.RecordLoginSuccess(ctx, account.ID); err != nil {
		return LoginResult{}, err
	}

	twoFactor, err := s.twoFactorEnabled(ctx, account.ID)
	if err != nil {
		return LoginResult{}, err
	}

	token, session, err := s.openSession(ctx, account.ID, in.IP, in.UserAgent, !twoFactor)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:             account,
		SessionToken:     token,
		Session:          session,
		TwoFactorPending: twoFactor,
	}, nil
}

// lookupIdentifier resolves a username, email, or numeric id to an
// account, reporting whether one was found.
func (s *Service) lookupIdentifier(ctx context.Context, raw string) (model.User, bool) {
	var (
		found model.User
		err   error
	)

	switch user.DetectIdentifier(raw) {
	case user.IdentifierID:
		id, convErr := strconv.ParseInt(raw, 10, 64)
		if convErr != nil {
			return model.User{}, false
		}
		found, err = s.store.UserByID(ctx, id)
	case user.IdentifierEmail:
		found, err = s.store.UserByEmail(ctx, user.NormalizeEmail(raw))
	default:
		found, err = s.store.UserByUsername(ctx, user.NormalizeName(raw))
	}
	if err != nil {
		return model.User{}, false
	}
	return found, true
}

// openSession issues a session token and stores its hash.
func (s *Service) openSession(ctx context.Context, userID int64, ip, agent string, twoFactorOK bool) (string, model.Session, error) {
	token, err := security.RandomString(security.RandomLength)
	if err != nil {
		return "", model.Session{}, err
	}

	now := s.now()
	lifetime := s.users().Auth.SessionDuration.Duration()
	if lifetime <= 0 {
		lifetime = 7 * 24 * time.Hour
	}

	session, err := s.store.CreateSession(ctx, model.Session{
		UserID:      userID,
		Hash:        security.HashToken(token),
		IP:          ip,
		UserAgent:   agent,
		TwoFactorOK: twoFactorOK,
		CreatedAt:   now,
		LastActive:  now,
		ExpiresAt:   now.Add(lifetime),
	})
	if err != nil {
		return "", model.Session{}, err
	}
	return token, session, nil
}

// Logout ends one session.
func (s *Service) Logout(ctx context.Context, token string) error {
	return mapStoreErr(s.store.DeleteSession(ctx, security.HashToken(token)))
}

// LogoutAll ends every session a user holds, which a password change and
// a compromised-account response both need.
func (s *Service) LogoutAll(ctx context.Context, userID int64) error {
	return mapStoreErr(s.store.DeleteUserSessions(ctx, userID))
}

// ListSessions returns a user's active sessions for the security page.
func (s *Service) ListSessions(ctx context.Context, userID int64) ([]model.Session, error) {
	sessions, err := s.store.ListSessions(ctx, userID)
	return sessions, mapStoreErr(err)
}

// RevokeSession ends one named session on behalf of its owner.
//
// The ownership check is what keeps this from being an IDOR: a session
// id belonging to another account is reported as not found rather than
// refused, so the identifier space cannot be probed.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID int64) error {
	sessions, err := s.store.ListSessions(ctx, userID)
	if err != nil {
		return mapStoreErr(err)
	}
	for _, sess := range sessions {
		if sess.ID == sessionID {
			return mapStoreErr(s.store.DeleteSession(ctx, sess.Hash))
		}
	}
	return ErrNotFound
}

// ResolveSession returns the account behind a session token, refreshing
// the activity stamp. An expired session is deleted rather than merely
// refused, so the row does not linger until the next purge.
func (s *Service) ResolveSession(ctx context.Context, token string) (model.User, model.Session, error) {
	hash := security.HashToken(token)

	session, err := s.store.SessionByHash(ctx, hash)
	if err != nil {
		return model.User{}, model.Session{}, mapStoreErr(err)
	}
	if session.Expired(s.now()) {
		_ = s.store.DeleteSession(ctx, hash)
		return model.User{}, model.Session{}, ErrInvalidCredentials
	}

	account, err := s.store.UserByID(ctx, session.UserID)
	if err != nil {
		return model.User{}, model.Session{}, mapStoreErr(err)
	}
	if !account.Active() {
		return model.User{}, model.Session{}, ErrInvalidCredentials
	}

	if err = s.store.TouchSession(ctx, hash); err != nil {
		return model.User{}, model.Session{}, err
	}
	return account, session, nil
}

// PurgeSessions deletes expired sessions and spent challenges. The
// PART 19 scheduler calls it; nothing about it is request-scoped.
func (s *Service) PurgeSessions(ctx context.Context) (int64, error) {
	now := s.now()

	sessions, err := s.store.PurgeExpiredSessions(ctx, now)
	if err != nil {
		return 0, err
	}
	challenges, err := s.store.PurgeExpiredChallenges(ctx, now)
	if err != nil {
		return sessions, err
	}
	invites, err := s.store.PurgeExpiredInvites(ctx)
	if err != nil {
		return sessions + challenges, err
	}
	return sessions + challenges + invites, nil
}

// redeemInvite validates and consumes an invite code.
func (s *Service) redeemInvite(ctx context.Context, code string) (model.Invite, error) {
	invite, err := s.store.InviteByHash(ctx, security.HashToken(code))
	if err != nil {
		if errors.Is(err, ErrNotFound) || mapStoreErr(err) == ErrNotFound {
			return model.Invite{}, ErrForbidden
		}
		return model.Invite{}, err
	}
	if !invite.Redeemable(s.now()) {
		return model.Invite{}, ErrForbidden
	}
	if err = s.store.RedeemInvite(ctx, invite.ID); err != nil {
		return model.Invite{}, ErrForbidden
	}
	return invite, nil
}
