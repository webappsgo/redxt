package user

import "errors"

// RegistrationMode is the server-level policy that decides how a Regular
// User account can come into existence, per AI.md PART 34. All four
// values are valid operational states, not error conditions.
type RegistrationMode string

const (
	// RegistrationOpen lets anyone sign up unaided.
	RegistrationOpen RegistrationMode = "open"
	// RegistrationInvite requires a valid invite code. This is redxt's
	// default, set in IDEA.md, which overrides the spec-wide default.
	RegistrationInvite RegistrationMode = "invite"
	// RegistrationAdminOnly lets only a Server Admin create accounts.
	RegistrationAdminOnly RegistrationMode = "admin_only"
	// RegistrationDisabled closes account creation entirely, including
	// invites and activation links that were already issued.
	RegistrationDisabled RegistrationMode = "disabled"
)

// CreationMode is the server-level policy for creating an organization,
// per PART 35. It reuses the same four values as registration.
type CreationMode string

const (
	// CreationOpen lets any user create an organization.
	CreationOpen CreationMode = "open"
	// CreationInvite lets a user create one only with an invite.
	CreationInvite CreationMode = "invite"
	// CreationAdminOnly lets only a Server Admin create one.
	CreationAdminOnly CreationMode = "admin_only"
	// CreationDisabled closes organization creation entirely.
	CreationDisabled CreationMode = "disabled"
)

// ErrUnknownMode reports a configured mode outside the four valid values.
var ErrUnknownMode = errors.New("user: unknown mode")

// ParseRegistrationMode resolves a configured registration mode.
func ParseRegistrationMode(raw string) (RegistrationMode, error) {
	switch RegistrationMode(NormalizeName(raw)) {
	case RegistrationOpen:
		return RegistrationOpen, nil
	case RegistrationInvite:
		return RegistrationInvite, nil
	case RegistrationAdminOnly:
		return RegistrationAdminOnly, nil
	case RegistrationDisabled:
		return RegistrationDisabled, nil
	}
	return "", ErrUnknownMode
}

// ParseCreationMode resolves a configured organization creation mode.
func ParseCreationMode(raw string) (CreationMode, error) {
	switch CreationMode(NormalizeName(raw)) {
	case CreationOpen:
		return CreationOpen, nil
	case CreationInvite:
		return CreationInvite, nil
	case CreationAdminOnly:
		return CreationAdminOnly, nil
	case CreationDisabled:
		return CreationDisabled, nil
	}
	return "", ErrUnknownMode
}

// SelfServiceAllowed reports whether an unauthenticated visitor may
// reach the public registration form at all. In every mode but open the
// form answers 404, per PART 34: a mode that does not accept self-signup
// must not advertise that it exists.
func (m RegistrationMode) SelfServiceAllowed() bool {
	return m == RegistrationOpen
}

// InviteAllowed reports whether an invite code can be redeemed. The
// disabled mode rejects codes that were already issued, which is the
// explicit PART 34 requirement for that mode.
func (m RegistrationMode) InviteAllowed() bool {
	return m == RegistrationOpen || m == RegistrationInvite
}

// AdminCreateAllowed reports whether a Server Admin may create an
// account directly. Every mode but disabled permits it; an unrecognized
// mode fails closed rather than granting the widest privilege.
func (m RegistrationMode) AdminCreateAllowed() bool {
	return m == RegistrationOpen || m == RegistrationInvite || m == RegistrationAdminOnly
}

// SelfServiceAllowed reports whether a signed-in user may create an
// organization without an invite.
func (m CreationMode) SelfServiceAllowed() bool {
	return m == CreationOpen
}

// InviteAllowed reports whether an organization-creation invite can be
// redeemed.
func (m CreationMode) InviteAllowed() bool {
	return m == CreationOpen || m == CreationInvite
}

// AdminCreateAllowed reports whether a Server Admin may create an
// organization directly. An unrecognized mode fails closed.
func (m CreationMode) AdminCreateAllowed() bool {
	return m == CreationOpen || m == CreationInvite || m == CreationAdminOnly
}
