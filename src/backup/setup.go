package backup

import "github.com/webappsgo/redxt/src/security"

// AdminRecovery clears every Server Admin's password hash and API tokens
// for RunSetup, per AI.md PART 22 "Admin Recovery Command". Implementations
// typically wrap the caller's users.db handle: UPDATE admins SET
// password_hash=” and DELETE FROM api_tokens WHERE owner_type='admin'. It
// must not touch user accounts, user data, or configuration.
type AdminRecovery interface {
	ClearAdminCredentials() error
}

// SetupAuthContext is the caller-determined state the AI.md PART 22 "Setup
// Authorization" table decides on.
type SetupAuthContext struct {
	// DatabaseEmpty is true on a first-run server with nothing to protect.
	DatabaseEmpty bool
	// IsRoot is true when the calling process has root/Administrator
	// privileges.
	IsRoot bool
	// ValidSetupToken is true when the caller supplied a still-valid,
	// unused, unexpired setup token.
	ValidSetupToken bool
}

// AuthorizeSetup implements the AI.md PART 22 "Setup Authorization" table
// exactly: empty database, root, or a valid setup token are allowed; a
// random user with none of those is denied.
func AuthorizeSetup(ctx SetupAuthContext) error {
	switch {
	case ctx.DatabaseEmpty:
		return nil
	case ctx.IsRoot:
		return nil
	case ctx.ValidSetupToken:
		return nil
	default:
		return ErrNotAuthorized
	}
}

// RunSetup implements `{project_name} --maintenance setup`: it clears the
// admin password and API token via s.Admin and returns a fresh one-time
// setup token. Everything else — user accounts, user data, configuration,
// SSL certificates — is left untouched, since s.Admin's contract is scoped
// to admin credentials only.
func (s *Service) RunSetup(ctx SetupAuthContext, actor string) (setupToken string, err error) {
	if err := AuthorizeSetup(ctx); err != nil {
		return "", err
	}
	if s.Admin == nil {
		return "", ErrNotAuthorized
	}
	if err := s.Admin.ClearAdminCredentials(); err != nil {
		return "", err
	}
	token, err := security.GenerateSetupToken()
	if err != nil {
		return "", err
	}
	s.audit(EventRestored, LevelWarn, actorOr(actor), map[string]any{
		"action": "admin_credentials_reset",
	})
	return token, nil
}
