package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/user"
)

// challengeLifetime is how long a password reset or an email
// confirmation stays redeemable.
const challengeLifetime = 2 * time.Hour

// recoveryCodeCount is how many single-use codes an enrollment issues.
const recoveryCodeCount = 10

// recoveryCodeLength is the length of one recovery code.
const recoveryCodeLength = 10

// User returns one account by id.
func (s *Service) User(ctx context.Context, id int64) (model.User, error) {
	account, err := s.store.UserByID(ctx, id)
	return account, mapStoreErr(err)
}

// ProfileInput is the editable part of a profile.
type ProfileInput struct {
	DisplayName       string
	Bio               string
	Location          string
	Website           string
	AvatarURL         string
	Timezone          string
	Language          string
	NotificationEmail string
	Visibility        string
	OrgVisibility     bool
}

// UpdateProfile writes a user's own profile.
//
// Fields the operator has switched off in server.users.profile are
// cleared rather than rejected, so turning a field off removes the data
// already stored in it instead of leaving stale content published.
func (s *Service) UpdateProfile(ctx context.Context, userID int64, in ProfileInput) (model.User, error) {
	account, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return model.User{}, mapStoreErr(err)
	}

	profile := s.users().Profile

	account.DisplayName = strings.TrimSpace(in.DisplayName)
	if account.DisplayName == "" {
		account.DisplayName = account.Username
	}

	account.Bio = ""
	if profile.AllowBio {
		account.Bio = strings.TrimSpace(in.Bio)
	}
	account.Website = ""
	if profile.AllowWebsite {
		account.Website = strings.TrimSpace(in.Website)
	}
	account.Location = ""
	if profile.AllowLocation {
		account.Location = strings.TrimSpace(in.Location)
	}
	account.AvatarURL = ""
	if profile.AllowAvatar {
		account.AvatarURL = strings.TrimSpace(in.AvatarURL)
	}

	account.Timezone = strings.TrimSpace(in.Timezone)
	account.Language = strings.TrimSpace(in.Language)

	if trimmed := strings.TrimSpace(in.NotificationEmail); trimmed != "" {
		normalized, mailErr := user.ValidateEmail(trimmed)
		if mailErr != nil {
			return model.User{}, validationError(mailErr.Error())
		}
		account.NotificationEmail = normalized
	} else {
		account.NotificationEmail = ""
	}

	switch in.Visibility {
	case model.VisibilityPublic, model.VisibilityPrivate:
		account.Visibility = in.Visibility
	default:
		return model.User{}, validationError("visibility must be public or private")
	}
	account.OrgVisibility = in.OrgVisibility

	if err = s.store.UpdateProfile(ctx, account); err != nil {
		return model.User{}, mapStoreErr(err)
	}
	return account, nil
}

// Preferences returns a user's saved preferences, or the documented
// defaults when they have never changed one.
func (s *Service) Preferences(ctx context.Context, userID int64) (model.Preferences, error) {
	prefs, err := s.store.Preferences(ctx, userID)
	return prefs, mapStoreErr(err)
}

// SavePreferences writes a user's preferences.
func (s *Service) SavePreferences(ctx context.Context, prefs model.Preferences) error {
	return mapStoreErr(s.store.SavePreferences(ctx, prefs))
}

// ChangePassword replaces a password after verifying the current one.
//
// Every session other than the one making the change is ended, because a
// password change is the response to a suspected compromise and leaving
// the attacker's session open would defeat it. The caller re-opens its
// own session afterwards.
func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next string) error {
	account, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return mapStoreErr(err)
	}

	ok, verifyErr := security.VerifyPassword(current, account.PasswordHash)
	if verifyErr != nil || !ok {
		return ErrInvalidCredentials
	}
	if err = s.passwordPolicy().ValidatePassword(next); err != nil {
		return validationError(err.Error())
	}

	hash, err := security.HashPassword(next)
	if err != nil {
		return err
	}
	if err = s.store.UpdatePassword(ctx, userID, hash); err != nil {
		return mapStoreErr(err)
	}

	if err = s.store.DeleteUserPasswordResets(ctx, userID); err != nil {
		return err
	}
	return mapStoreErr(s.store.DeleteUserSessions(ctx, userID))
}

// StartPasswordReset issues a reset token for an identifier.
//
// The returned token is empty when no account matched. The caller must
// still report success to the requester either way: telling them whether
// the address is registered turns the reset form into an account
// enumeration oracle.
func (s *Service) StartPasswordReset(ctx context.Context, identifier string) (string, model.User, error) {
	account, found := s.lookupIdentifier(ctx, identifier)
	if !found || !account.Active() {
		return "", model.User{}, nil
	}

	token, err := security.RandomString(security.RandomLength)
	if err != nil {
		return "", model.User{}, err
	}
	if err = s.store.CreatePasswordReset(ctx, account.ID,
		security.HashToken(token), s.now().Add(challengeLifetime)); err != nil {
		return "", model.User{}, err
	}
	return token, account, nil
}

// CompletePasswordReset redeems a reset token and sets a new password.
func (s *Service) CompletePasswordReset(ctx context.Context, token, password string) error {
	challenge, err := s.store.PasswordReset(ctx, security.HashToken(token))
	if err != nil {
		return ErrInvalidCredentials
	}
	if !challenge.Usable(s.now()) {
		return ErrInvalidCredentials
	}
	if err = s.passwordPolicy().ValidatePassword(password); err != nil {
		return validationError(err.Error())
	}

	// Consuming the challenge before writing the password makes the
	// single-use guard authoritative: two requests racing with the same
	// token cannot both reach the update.
	if err = s.store.ConsumePasswordReset(ctx, challenge.ID); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	if err = s.store.UpdatePassword(ctx, challenge.UserID, hash); err != nil {
		return mapStoreErr(err)
	}
	return mapStoreErr(s.store.DeleteUserSessions(ctx, challenge.UserID))
}

// StartEmailVerification issues a confirmation token for an address.
func (s *Service) StartEmailVerification(ctx context.Context, userID int64, email string) (string, error) {
	normalized, err := user.ValidateEmail(email)
	if err != nil {
		return "", validationError(err.Error())
	}

	token, err := security.RandomString(security.RandomLength)
	if err != nil {
		return "", err
	}
	if err = s.store.CreateEmailVerification(ctx, userID, normalized,
		security.HashToken(token), s.now().Add(challengeLifetime)); err != nil {
		return "", err
	}
	return token, nil
}

// CompleteEmailVerification redeems a confirmation token and activates
// the account if it was waiting on the address.
func (s *Service) CompleteEmailVerification(ctx context.Context, token string) (model.User, error) {
	challenge, err := s.store.EmailVerification(ctx, security.HashToken(token))
	if err != nil {
		return model.User{}, ErrInvalidCredentials
	}
	if !challenge.Usable(s.now()) {
		return model.User{}, ErrInvalidCredentials
	}
	if err = s.store.ConsumeEmailVerification(ctx, challenge.ID); err != nil {
		return model.User{}, ErrInvalidCredentials
	}
	if err = s.store.MarkEmailVerified(ctx, challenge.UserID); err != nil {
		return model.User{}, mapStoreErr(err)
	}

	account, err := s.store.UserByID(ctx, challenge.UserID)
	if err != nil {
		return model.User{}, mapStoreErr(err)
	}
	if account.Status == model.StatusPending {
		if err = s.store.UpdateStatus(ctx, account.ID, model.StatusActive); err != nil {
			return model.User{}, mapStoreErr(err)
		}
		account.Status = model.StatusActive
	}
	return account, nil
}

// twoFactorEnabled reports whether a confirmed second factor gates this
// account's sign-in.
func (s *Service) twoFactorEnabled(ctx context.Context, userID int64) (bool, error) {
	if !s.users().Auth.Allow2FA {
		return false, nil
	}

	enrollment, err := s.store.TOTPForUser(ctx, userID)
	if err != nil {
		if mapStoreErr(err) == ErrNotFound {
			return false, nil
		}
		return false, err
	}
	return enrollment.Confirmed, nil
}

// TwoFactorEnabled reports whether the account has a confirmed second
// factor, for the security page and the admin user list.
func (s *Service) TwoFactorEnabled(ctx context.Context, userID int64) (bool, error) {
	return s.twoFactorEnabled(ctx, userID)
}

// Enrollment is a started, unconfirmed second-factor setup.
type Enrollment struct {
	// Secret is the base32 seed, shown once so it can be typed in.
	Secret string
	// URI is the otpauth:// value the QR code encodes.
	URI string
	// RecoveryCodes are the plaintext single-use codes, shown once. Only
	// their hashes are stored.
	RecoveryCodes []string
}

// StartTwoFactor begins a TOTP enrollment.
//
// The seed is stored encrypted rather than hashed, because verifying a
// time-based code requires recomputing it, which requires the seed. That
// is the one secret in PART 34 that cannot be a one-way hash, and it is
// why server.security.encryption_key must be configured before 2FA can
// be offered.
func (s *Service) StartTwoFactor(ctx context.Context, userID int64) (Enrollment, error) {
	if !s.users().Auth.Allow2FA {
		return Enrollment{}, ErrDisabled
	}
	if s.cipher == nil {
		return Enrollment{}, errors.New("service: encryption key required for two-factor enrollment")
	}

	account, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return Enrollment{}, mapStoreErr(err)
	}

	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		return Enrollment{}, err
	}
	encrypted, err := s.cipher.EncryptString(secret)
	if err != nil {
		return Enrollment{}, err
	}

	codes, hashed, err := generateRecoveryCodes()
	if err != nil {
		return Enrollment{}, err
	}
	encoded, err := json.Marshal(hashed)
	if err != nil {
		return Enrollment{}, err
	}

	if err = s.store.SaveTOTP(ctx, store.TOTP{
		UserID:          userID,
		SecretEncrypted: encrypted,
		KeyVersion:      s.cipher.Version(),
		Confirmed:       false,
		RecoveryCodes:   string(encoded),
	}); err != nil {
		return Enrollment{}, err
	}

	issuer := s.config.Server.ApplicationName
	if issuer == "" {
		issuer = "redxt"
	}

	return Enrollment{
		Secret:        secret,
		URI:           security.TOTPProvisioningURI(issuer, account.Username, secret),
		RecoveryCodes: codes,
	}, nil
}

// ConfirmTwoFactor proves an enrollment by checking one code. An
// enrollment that is never confirmed never gates a sign-in, so an
// abandoned setup cannot lock a user out of their own account.
func (s *Service) ConfirmTwoFactor(ctx context.Context, userID int64, code string) error {
	secret, _, err := s.totpSecret(ctx, userID)
	if err != nil {
		return err
	}
	if !security.VerifyTOTP(secret, code, s.now()) {
		return ErrInvalidCredentials
	}
	return mapStoreErr(s.store.ConfirmTOTP(ctx, userID))
}

// VerifyTwoFactor checks a code or a recovery code against a pending
// session and completes it.
func (s *Service) VerifyTwoFactor(ctx context.Context, userID int64, sessionToken, code string) error {
	secret, enrollment, err := s.totpSecret(ctx, userID)
	if err != nil {
		return err
	}
	if !enrollment.Confirmed {
		return ErrInvalidCredentials
	}

	if !security.VerifyTOTP(secret, code, s.now()) {
		used, recoveryErr := s.consumeRecoveryCode(ctx, userID, enrollment.RecoveryCodes, code)
		if recoveryErr != nil {
			return recoveryErr
		}
		if !used {
			return ErrInvalidCredentials
		}
	}
	return mapStoreErr(s.store.MarkSessionTwoFactor(ctx, security.HashToken(sessionToken)))
}

// DisableTwoFactor removes a second factor after re-checking the
// password, so a borrowed session cannot strip the protection on its
// own.
func (s *Service) DisableTwoFactor(ctx context.Context, userID int64, password string) error {
	account, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return mapStoreErr(err)
	}
	ok, verifyErr := security.VerifyPassword(password, account.PasswordHash)
	if verifyErr != nil || !ok {
		return ErrInvalidCredentials
	}
	if s.users().Auth.Require2FA {
		return ErrForbidden
	}
	return mapStoreErr(s.store.DeleteTOTP(ctx, userID))
}

// totpSecret decrypts a user's stored seed.
func (s *Service) totpSecret(ctx context.Context, userID int64) (string, storedTOTP, error) {
	if s.cipher == nil {
		return "", storedTOTP{}, ErrDisabled
	}

	enrollment, err := s.store.TOTPForUser(ctx, userID)
	if err != nil {
		return "", storedTOTP{}, mapStoreErr(err)
	}
	secret, err := s.cipher.DecryptString(enrollment.SecretEncrypted)
	if err != nil {
		return "", storedTOTP{}, err
	}
	return secret, storedTOTP{
		Confirmed:     enrollment.Confirmed,
		RecoveryCodes: enrollment.RecoveryCodes,
	}, nil
}

// storedTOTP is the part of an enrollment the service reasons about,
// kept separate from the store row so the seed is not carried around
// after it has been decrypted.
type storedTOTP struct {
	Confirmed     bool
	RecoveryCodes string
}

// consumeRecoveryCode spends a single-use recovery code, reporting
// whether one matched.
func (s *Service) consumeRecoveryCode(ctx context.Context, userID int64, encoded, submitted string) (bool, error) {
	var hashes []string
	if err := json.Unmarshal([]byte(encoded), &hashes); err != nil {
		return false, nil
	}

	candidate := strings.TrimSpace(submitted)

	remaining := make([]string, 0, len(hashes))
	matched := false
	for _, held := range hashes {
		if !matched && security.VerifyTokenHash(candidate, held) {
			matched = true
			continue
		}
		remaining = append(remaining, held)
	}
	if !matched {
		return false, nil
	}

	next, err := json.Marshal(remaining)
	if err != nil {
		return false, err
	}
	if err = s.store.SetRecoveryCodes(ctx, userID, string(next)); err != nil {
		return false, mapStoreErr(err)
	}
	return true, nil
}

// generateRecoveryCodes returns plaintext codes and their hashes. Only
// the hashes are stored, so a database read cannot recover a usable
// code.
func generateRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)

	for i := 0; i < recoveryCodeCount; i++ {
		code, err := security.RandomString(recoveryCodeLength)
		if err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
		hashes = append(hashes, security.HashToken(code))
	}
	return codes, hashes, nil
}
