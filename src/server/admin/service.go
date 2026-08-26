package admin

import (
	"context"
	"errors"
	"time"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
)

// SessionLifetime is the absolute lifetime of an admin web session.
// Admin sessions are shorter-lived than Regular User sessions because
// the account they protect can reconfigure the whole instance.
const SessionLifetime = 24 * time.Hour

var (
	// ErrSetupComplete reports that first-run setup already happened: at
	// least one admin row already exists, so CompleteSetup refuses to
	// create a second Primary Admin through the token path.
	ErrSetupComplete = errors.New("admin: setup already complete")
	// ErrInvalidSetupToken reports that the supplied setup token does not
	// match the hash minted at startup and shown once on the console.
	ErrInvalidSetupToken = errors.New("admin: invalid setup token")
	// ErrInvalidCredentials reports a login that failed for any reason.
	// Wrong username and wrong password are deliberately
	// indistinguishable to the caller, so a failed login never discloses
	// which admin usernames exist.
	ErrInvalidCredentials = errors.New("admin: invalid credentials")
	// ErrAccountDisabled reports a login attempt against a disabled
	// admin account.
	ErrAccountDisabled = errors.New("admin: account disabled")
)

// Service implements the PART 17 admin account lifecycle: first-run
// setup, login, and logout. Admin accounts and sessions live in
// users.db; the first-run setup-token hash is minted at startup into
// server.db's app_secrets table alongside the other installation
// secrets, so the service needs both handles.
type Service struct {
	store    *Store
	serverDB *database.DB
}

// NewService returns a Service backed by an open users.db handle (for
// admin accounts/sessions) and an open server.db handle (to read the
// first-run setup-token hash).
func NewService(usersDB, serverDB *database.DB) *Service {
	return &Service{store: NewStore(usersDB), serverDB: serverDB}
}

// NeedsSetup reports whether no Server Admin exists yet, meaning the
// first-run setup wizard must run before the admin panel accepts a
// login.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.store.CountAdmins(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// CompleteSetup verifies the one-time setup token against the hash
// minted at startup, then creates the first (Primary) admin with the
// supplied credentials. It refuses if an admin already exists, so the
// token cannot be replayed to create a second account after setup.
func (s *Service) CompleteSetup(ctx context.Context, token, username, email, password string) (Admin, error) {
	needsSetup, err := s.NeedsSetup(ctx)
	if err != nil {
		return Admin{}, err
	}
	if !needsSetup {
		return Admin{}, ErrSetupComplete
	}

	storedHash, _, _, err := database.GetSecret(ctx, s.serverDB, security.SecretSetupToken)
	if err != nil {
		return Admin{}, ErrInvalidSetupToken
	}
	if !security.VerifyTokenHash(token, storedHash) {
		return Admin{}, ErrInvalidSetupToken
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return Admin{}, err
	}

	return s.store.CreateAdmin(ctx, Admin{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
	})
}

// Login verifies a username/password pair and, on success, issues a
// session token. The plaintext token is returned once; only its
// SHA-256 hash is persisted.
func (s *Service) Login(ctx context.Context, username, password, ip, agent string) (string, Admin, error) {
	found, err := s.store.AdminByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Run the same Argon2id verification path against a fixed
			// hash even on an unknown username, so the response time
			// does not disclose which usernames exist.
			_, _ = security.VerifyPassword(password, argon2idDecoyHash)
			return "", Admin{}, ErrInvalidCredentials
		}
		return "", Admin{}, err
	}
	if found.Disabled {
		return "", Admin{}, ErrAccountDisabled
	}

	ok, err := security.VerifyPassword(password, found.PasswordHash)
	if err != nil || !ok {
		return "", Admin{}, ErrInvalidCredentials
	}

	if err := s.store.RecordLoginSuccess(ctx, found.ID); err != nil {
		return "", Admin{}, err
	}

	token, err := s.openSession(ctx, found.ID, ip, agent)
	if err != nil {
		return "", Admin{}, err
	}
	return token, found, nil
}

// openSession issues a session token and stores its hash.
func (s *Service) openSession(ctx context.Context, adminID int64, ip, agent string) (string, error) {
	token, err := security.RandomString(security.RandomLength)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC().Truncate(time.Second)
	_, err = s.store.CreateSession(ctx, Session{
		AdminID:    adminID,
		Hash:       security.HashToken(token),
		IP:         ip,
		UserAgent:  agent,
		CreatedAt:  now,
		LastActive: now,
		ExpiresAt:  now.Add(SessionLifetime),
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// CurrentAdmin resolves a session token to its admin, or reports
// ErrInvalidCredentials if the token is unknown or expired.
func (s *Service) CurrentAdmin(ctx context.Context, token string) (Admin, error) {
	sess, err := s.store.SessionByHash(ctx, security.HashToken(token))
	if err != nil {
		return Admin{}, ErrInvalidCredentials
	}
	if sess.Expired(time.Now().UTC()) {
		return Admin{}, ErrInvalidCredentials
	}
	found, err := s.store.AdminByID(ctx, sess.AdminID)
	if err != nil {
		return Admin{}, ErrInvalidCredentials
	}
	if found.Disabled {
		return Admin{}, ErrAccountDisabled
	}
	_ = s.store.TouchSession(ctx, sess.Hash)
	return found, nil
}

// Logout ends one session.
func (s *Service) Logout(ctx context.Context, token string) error {
	err := s.store.DeleteSession(ctx, security.HashToken(token))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// argon2idDecoyHash is a fixed, valid Argon2id PHC string with no real
// account behind it, used to keep the login-failure timing for an
// unknown username indistinguishable from a wrong password.
const argon2idDecoyHash = "$argon2id$v=19$m=65536,t=1,p=4$MDAwMDAwMDAwMDAwMDAwMA$b25seSBhIGRlY295IGhhc2ggdmFsdWU"
