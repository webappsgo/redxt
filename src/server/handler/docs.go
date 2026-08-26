package handler

import (
	"net/http"

	"github.com/webappsgo/redxt/src/swagger"
)

// apiRoute binds one REST route to the operation that documents it.
//
// The mux and the OpenAPI and GraphQL documents are all generated from
// this one table, so a route cannot exist without documentation and the
// documentation cannot describe a route that is not served. PART 14
// requires the three surfaces to describe the same API; deriving them
// from a single declaration is what makes that true by construction
// rather than by review.
type apiRoute struct {
	// op is the transport-neutral declaration of the endpoint.
	op swagger.Operation
	// handler selects the method that serves it.
	handler func(*Handler) http.HandlerFunc
}

// RegisterAPIOperations declares every PART 34, 35 and 36 endpoint and
// every payload shape they exchange into a registry.
//
// The swagger package documents the fixed server endpoints itself and
// leaves the rest of the surface to the route package that owns it,
// which is this one.
func RegisterAPIOperations(r *swagger.Registry) error {
	for _, t := range apiTypes {
		if err := r.RegisterType(t); err != nil {
			return err
		}
	}
	for _, rt := range apiRoutes {
		if err := r.Register(rt.op); err != nil {
			return err
		}
	}
	return nil
}

// apiTypes are the payload shapes exchanged by the routes below. Every
// field mirrors a JSON tag on the corresponding view or request struct.
//
// Credential material is absent throughout. A password hash, a session
// hash, and a token hash have no field here because they have no field
// in the responses either.
var apiTypes = []swagger.ObjectType{
	{
		Name:        "User",
		Description: "A Regular User account as seen by its owner.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Numeric account identifier."},
			{Name: "username", Kind: swagger.KindString, Required: true, Description: "Login name, also the vanity URL segment."},
			{Name: "email", Kind: swagger.KindString, Required: true, Description: "Primary email address."},
			{Name: "email_verified", Kind: swagger.KindBool, Required: true, Description: "Whether the address has completed verification."},
			{Name: "display_name", Kind: swagger.KindString, Required: true, Description: "Name shown in the interface."},
			{Name: "bio", Kind: swagger.KindString, Description: "Free-text profile biography."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "avatar_url", Kind: swagger.KindString, Description: "Profile image URL."},
			{Name: "timezone", Kind: swagger.KindString, Description: "IANA timezone used to render timestamps."},
			{Name: "language", Kind: swagger.KindString, Description: "Preferred interface language."},
			{Name: "visibility", Kind: swagger.KindString, Required: true, Description: "public or private profile visibility."},
			{Name: "org_visibility", Kind: swagger.KindBool, Required: true, Description: "Whether organization memberships appear on the public profile."},
			{Name: "status", Kind: swagger.KindString, Required: true, Description: "Account status, for example active or suspended."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the account was created."},
		},
	},
	{
		Name:        "PublicProfile",
		Description: "The vanity profile of a user, filtered by that user's privacy settings.",
		Fields: []swagger.Field{
			{Name: "username", Kind: swagger.KindString, Required: true, Description: "Vanity URL segment."},
			{Name: "display_name", Kind: swagger.KindString, Required: true, Description: "Name shown in the interface."},
			{Name: "bio", Kind: swagger.KindString, Description: "Free-text profile biography."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "avatar_url", Kind: swagger.KindString, Description: "Profile image URL."},
			{Name: "email", Kind: swagger.KindString, Description: "Present only when the owner chose to show it."},
			{Name: "orgs", Kind: swagger.KindObject, Ref: "Org", List: true, Description: "Memberships, present only when the owner chose to show them."},
		},
	},
	{
		Name:        "Preferences",
		Description: "A user's privacy, notification, and presentation settings.",
		Fields:      preferenceFields,
	},
	{
		Name:        "PreferencesInput",
		Description: "The complete preference set to store. Every field is written, so a partial body resets what it omits.",
		Input:       true,
		Fields:      preferenceFields,
	},
	{
		Name:        "SessionInfo",
		Description: "One live browser session belonging to the caller.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Session identifier used to revoke it."},
			{Name: "ip_address", Kind: swagger.KindString, Description: "Address the session was last seen from."},
			{Name: "user_agent", Kind: swagger.KindString, Description: "Client the session was opened with."},
			{Name: "two_factor_ok", Kind: swagger.KindBool, Required: true, Description: "Whether the second factor has been satisfied."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the session was opened."},
			{Name: "last_active_at", Kind: swagger.KindTime, Required: true, Description: "When the session was last used."},
			{Name: "expires_at", Kind: swagger.KindTime, Required: true, Description: "When the session stops being accepted."},
		},
	},
	{
		Name:        "SecurityOverview",
		Description: "A summary of the caller's authentication state.",
		Fields: []swagger.Field{
			{Name: "email", Kind: swagger.KindString, Required: true, Description: "Primary email address."},
			{Name: "email_verified", Kind: swagger.KindBool, Required: true, Description: "Whether the address has completed verification."},
			{Name: "two_factor_enabled", Kind: swagger.KindBool, Required: true, Description: "Whether a confirmed second factor is in force."},
			{Name: "last_login_at", Kind: swagger.KindTime, Required: true, Description: "When the account last signed in."},
			{Name: "active_sessions", Kind: swagger.KindInt, Required: true, Description: "Number of live sessions."},
			{Name: "api_tokens", Kind: swagger.KindInt, Required: true, Description: "Number of usable API tokens."},
		},
	},
	{
		Name:        "TwoFactorEnrollment",
		Description: "The one-time enrollment material for a second factor. It is served by the start call and never again.",
		Fields: []swagger.Field{
			{Name: "secret", Kind: swagger.KindString, Required: true, Description: "Base32 TOTP seed. Only its encrypted form is stored."},
			{Name: "uri", Kind: swagger.KindString, Required: true, Description: "otpauth URI for an authenticator application."},
			{Name: "recovery_codes", Kind: swagger.KindString, List: true, Required: true, Description: "Single-use recovery codes. Only their hashes are stored."},
		},
	},
	{
		Name:        "TwoFactorState",
		Description: "Whether a second factor is in force after the call.",
		Fields: []swagger.Field{
			{Name: "two_factor_enabled", Kind: swagger.KindBool, Required: true, Description: "Resulting second-factor state."},
		},
	},
	{
		Name:        "SignInResult",
		Description: "The outcome of a credential check. A session cookie accompanies it.",
		Fields: []swagger.Field{
			{Name: "user", Kind: swagger.KindObject, Ref: "User", Required: true, Description: "The account that signed in."},
			{Name: "two_factor_pending", Kind: swagger.KindBool, Required: true, Description: "True when the session is still awaiting a second factor."},
		},
	},
	{
		Name:        "Org",
		Description: "An organization. Every zone, policy, key, and dynamic host belongs to one.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Numeric organization identifier."},
			{Name: "slug", Kind: swagger.KindString, Required: true, Description: "URL segment that addresses the organization."},
			{Name: "name", Kind: swagger.KindString, Required: true, Description: "Display name."},
			{Name: "description", Kind: swagger.KindString, Description: "Free-text description."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
			{Name: "avatar_url", Kind: swagger.KindString, Description: "Profile image URL."},
			{Name: "visibility", Kind: swagger.KindString, Required: true, Description: "public or private profile visibility."},
			{Name: "personal", Kind: swagger.KindBool, Required: true, Description: "True for the organization created with the account, which cannot be shared or deleted."},
			{Name: "owner_id", Kind: swagger.KindInt, Required: true, Description: "Account that holds the owner role."},
			{Name: "status", Kind: swagger.KindString, Required: true, Description: "Organization status, for example active or suspended."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the organization was created."},
		},
	},
	{
		Name:        "OrgAccess",
		Description: "An organization together with the caller's standing inside it.",
		Fields: []swagger.Field{
			{Name: "org", Kind: swagger.KindObject, Ref: "Org", Required: true, Description: "The organization."},
			{Name: "role", Kind: swagger.KindString, Required: true, Description: "owner, admin, editor, or viewer."},
			{Name: "permissions", Kind: swagger.KindString, List: true, Required: true, Description: "Permissions the role carries, so a client need not restate the role table."},
		},
	},
	{
		Name:        "OrgSettings",
		Description: "The editable profile of an organization plus whether the caller may change it.",
		Fields: []swagger.Field{
			{Name: "slug", Kind: swagger.KindString, Required: true, Description: "URL segment that addresses the organization."},
			{Name: "name", Kind: swagger.KindString, Required: true, Description: "Display name."},
			{Name: "description", Kind: swagger.KindString, Description: "Free-text description."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
			{Name: "visibility", Kind: swagger.KindString, Required: true, Description: "public or private profile visibility."},
			{Name: "personal", Kind: swagger.KindBool, Required: true, Description: "True for the organization created with the account."},
			{Name: "can_edit", Kind: swagger.KindBool, Required: true, Description: "Whether the caller's role carries the settings permission."},
		},
	},
	{
		Name:        "Member",
		Description: "One account's membership of an organization.",
		Fields: []swagger.Field{
			{Name: "user_id", Kind: swagger.KindInt, Required: true, Description: "The member's account identifier."},
			{Name: "username", Kind: swagger.KindString, Required: true, Description: "The member's login name."},
			{Name: "email", Kind: swagger.KindString, Required: true, Description: "The member's email address."},
			{Name: "role", Kind: swagger.KindString, Required: true, Description: "owner, admin, editor, or viewer."},
			{Name: "joined_at", Kind: swagger.KindTime, Required: true, Description: "When the membership began."},
		},
	},
	{
		Name:        "Invite",
		Description: "A pending invitation. The code itself is absent, because only its hash is stored.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Invitation identifier used to revoke it."},
			{Name: "email", Kind: swagger.KindString, Description: "Address the invitation is restricted to, when it is restricted."},
			{Name: "role", Kind: swagger.KindString, Required: true, Description: "Role the invitation grants on acceptance."},
			{Name: "max_uses", Kind: swagger.KindInt, Required: true, Description: "How many times the code may be redeemed."},
			{Name: "use_count", Kind: swagger.KindInt, Required: true, Description: "How many times it already has been."},
			{Name: "invited_by", Kind: swagger.KindInt, Required: true, Description: "Account that issued the invitation."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the invitation was issued."},
			{Name: "expires_at", Kind: swagger.KindTime, Required: true, Description: "When it stops being redeemable."},
		},
	},
	{
		Name:        "IssuedInvite",
		Description: "A freshly issued invitation together with its code. The code appears here once and nowhere else.",
		Fields: []swagger.Field{
			{Name: "invite", Kind: swagger.KindObject, Ref: "Invite", Required: true, Description: "The stored invitation record."},
			{Name: "code", Kind: swagger.KindString, Required: true, Description: "Plaintext invitation code."},
		},
	},
	{
		Name:        "Token",
		Description: "An API token record. The secret is absent, because only its SHA-256 hash is stored.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Token identifier used to revoke it."},
			{Name: "name", Kind: swagger.KindString, Required: true, Description: "Label chosen when the token was issued."},
			{Name: "prefix", Kind: swagger.KindString, Required: true, Description: "Leading segment of the secret, enough to recognise a token without revealing it."},
			{Name: "scope", Kind: swagger.KindString, Required: true, Description: "Breadth of the token: the whole account, one organization, or one zone."},
			{Name: "role", Kind: swagger.KindString, Description: "Role the token acts with, never above the issuer's own."},
			{Name: "org_id", Kind: swagger.KindInt, Description: "Organization the token is confined to, when it is confined."},
			{Name: "zone_id", Kind: swagger.KindInt, Description: "Zone the token is confined to, when it is confined."},
			{Name: "capability", Kind: swagger.KindString, Description: "Narrower capability within the resource, reserved for the credential types that attach to a zone."},
			{Name: "last_used_at", Kind: swagger.KindTime, Description: "When the token was last accepted."},
			{Name: "expires_at", Kind: swagger.KindTime, Description: "When the token stops being accepted."},
			{Name: "revoked_at", Kind: swagger.KindTime, Description: "When the token was revoked, if it was."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the token was issued."},
		},
	},
	{
		Name:        "IssuedToken",
		Description: "A freshly issued token together with its secret. The secret appears here once and nowhere else.",
		Fields: []swagger.Field{
			{Name: "token", Kind: swagger.KindObject, Ref: "Token", Required: true, Description: "The stored token record."},
			{Name: "secret", Kind: swagger.KindString, Required: true, Description: "Plaintext token secret."},
		},
	},
	{
		Name:        "Domain",
		Description: "A custom domain owned by an organization. Custom domains change branding only; they grant no additional access.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Domain identifier."},
			{Name: "org_id", Kind: swagger.KindInt, Required: true, Description: "Organization that owns the domain."},
			{Name: "domain", Kind: swagger.KindString, Required: true, Description: "The domain name itself."},
			{Name: "purpose", Kind: swagger.KindString, Required: true, Description: "What the domain fronts."},
			{Name: "is_apex", Kind: swagger.KindBool, Required: true, Description: "Whether the name is a zone apex."},
			{Name: "is_wildcard", Kind: swagger.KindBool, Required: true, Description: "Whether the name is a wildcard."},
			{Name: "verification_status", Kind: swagger.KindString, Required: true, Description: "pending, verified, or failed. A domain is not served until it is verified."},
			{Name: "verified_at", Kind: swagger.KindTime, Description: "When ownership was proven."},
			{Name: "ssl_enabled", Kind: swagger.KindBool, Required: true, Description: "Whether a certificate has been requested for the domain."},
			{Name: "ssl_status", Kind: swagger.KindString, Required: true, Description: "State of the certificate held by the server's ACME manager."},
			{Name: "ssl_expires_at", Kind: swagger.KindTime, Description: "When the current certificate expires."},
			{Name: "status", Kind: swagger.KindString, Required: true, Description: "active or suspended."},
			{Name: "suspend_reason", Kind: swagger.KindString, Description: "Why the domain was suspended, when it was."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the domain was claimed."},
		},
	},
	{
		Name:        "DomainChallenge",
		Description: "The DNS record that proves ownership of a domain.",
		Fields: []swagger.Field{
			{Name: "name", Kind: swagger.KindString, Required: true, Description: "Owner name the record is published at."},
			{Name: "type", Kind: swagger.KindString, Required: true, Description: "Record type, TXT."},
			{Name: "value", Kind: swagger.KindString, Required: true, Description: "Token the record must carry."},
		},
	},
	{
		Name:        "AuditEntry",
		Description: "One recorded change inside an organization.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Entry identifier."},
			{Name: "event", Kind: swagger.KindString, Required: true, Description: "What happened."},
			{Name: "actor_type", Kind: swagger.KindString, Required: true, Description: "Whether the actor was a user, a token, or the server."},
			{Name: "actor_id", Kind: swagger.KindInt, Required: true, Description: "Identifier of the actor."},
			{Name: "target_id", Kind: swagger.KindInt, Description: "Identifier of what was acted on."},
			{Name: "details", Kind: swagger.KindString, Description: "Human-readable detail of the change."},
			{Name: "created_at", Kind: swagger.KindTime, Required: true, Description: "When the change was recorded."},
		},
	},
	{
		Name:        "ZoneGrant",
		Description: "A per-zone permission held by one member of an organization.",
		Fields: []swagger.Field{
			{Name: "zone_id", Kind: swagger.KindInt, Required: true, Description: "Zone the grant applies to."},
			{Name: "member_id", Kind: swagger.KindInt, Required: true, Description: "Member the grant belongs to."},
			{Name: "permission", Kind: swagger.KindString, Required: true, Description: "Permission granted on that zone."},
		},
	},
	{
		Name:        "SignOutResult",
		Description: "Confirms the session was ended.",
		Fields: []swagger.Field{
			{Name: "signed_out", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "MailSentResult",
		Description: "Confirms a message was dispatched. It is returned whether or not the address exists, so it reveals nothing about who is registered.",
		Fields: []swagger.Field{
			{Name: "sent", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "UpdateResult",
		Description: "Confirms the change was written.",
		Fields: []swagger.Field{
			{Name: "updated", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "RevokeResult",
		Description: "Confirms the credential was revoked.",
		Fields: []swagger.Field{
			{Name: "revoked", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "RemoveResult",
		Description: "Confirms the record was removed.",
		Fields: []swagger.Field{
			{Name: "removed", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "DeleteResult",
		Description: "Confirms the organization was deleted.",
		Fields: []swagger.Field{
			{Name: "deleted", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "SuspendResult",
		Description: "Confirms the domain stopped being served.",
		Fields: []swagger.Field{
			{Name: "suspended", Kind: swagger.KindBool, Required: true, Description: "Always true."},
		},
	},
	{
		Name:        "VisibilityResult",
		Description: "The visibility the organization now has.",
		Fields: []swagger.Field{
			{Name: "visibility", Kind: swagger.KindString, Required: true, Description: "public or private."},
		},
	},
	{
		Name:        "TransferResult",
		Description: "The account that now holds the owner role.",
		Fields: []swagger.Field{
			{Name: "owner_id", Kind: swagger.KindInt, Required: true, Description: "New owner's account identifier."},
		},
	},
	{
		Name:        "MemberRoleResult",
		Description: "The role a member now holds.",
		Fields: []swagger.Field{
			{Name: "user_id", Kind: swagger.KindInt, Required: true, Description: "The member's account identifier."},
			{Name: "role", Kind: swagger.KindString, Required: true, Description: "owner, admin, editor, or viewer."},
		},
	},
	{
		Name:        "DomainPurposeResult",
		Description: "What a domain is now used for.",
		Fields: []swagger.Field{
			{Name: "id", Kind: swagger.KindInt, Required: true, Description: "Domain identifier."},
			{Name: "purpose", Kind: swagger.KindString, Required: true, Description: "The stored purpose."},
		},
	},
	{
		Name:        "RegisterInput",
		Description: "An account signup. Which fields are required depends on the registration mode in force.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "username", Kind: swagger.KindString, Required: true, Description: "Desired login name."},
			{Name: "email", Kind: swagger.KindString, Required: true, Description: "Email address to register."},
			{Name: "password", Kind: swagger.KindString, Required: true, Description: "Password, stored as an Argon2id hash."},
			{Name: "invite", Kind: swagger.KindString, Description: "Invitation code. Required in invite mode, accepted in open mode."},
		},
	},
	{
		Name:        "SignInInput",
		Description: "A credential check. A wrong password and an unknown account are answered identically.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "identifier", Kind: swagger.KindString, Required: true, Description: "Username or email address."},
			{Name: "password", Kind: swagger.KindString, Required: true, Description: "The account password."},
		},
	},
	{
		Name:        "TwoFactorCodeInput",
		Description: "A one-time code from an authenticator application or a recovery code.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "code", Kind: swagger.KindString, Required: true, Description: "The code to check."},
		},
	},
	{
		Name:        "ForgotPasswordInput",
		Description: "A request for a password reset link.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "identifier", Kind: swagger.KindString, Required: true, Description: "Username or email address."},
		},
	},
	{
		Name:        "ResetPasswordInput",
		Description: "A password reset redeeming a mailed token.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "token", Kind: swagger.KindString, Required: true, Description: "Single-use token from the reset email."},
			{Name: "password", Kind: swagger.KindString, Required: true, Description: "The new password."},
		},
	},
	{
		Name:        "ProfileInput",
		Description: "The editable parts of an account profile.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "display_name", Kind: swagger.KindString, Description: "Name shown in the interface."},
			{Name: "bio", Kind: swagger.KindString, Description: "Free-text profile biography."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "avatar_url", Kind: swagger.KindString, Description: "Profile image URL."},
			{Name: "timezone", Kind: swagger.KindString, Description: "IANA timezone used to render timestamps."},
			{Name: "language", Kind: swagger.KindString, Description: "Preferred interface language."},
			{Name: "notification_email", Kind: swagger.KindString, Description: "Address notifications are sent to, when it differs from the primary address."},
			{Name: "visibility", Kind: swagger.KindString, Description: "public or private profile visibility."},
			{Name: "org_visibility", Kind: swagger.KindBool, Description: "Whether organization memberships appear on the public profile."},
		},
	},
	{
		Name:        "ChangePasswordInput",
		Description: "A password change. The current password is required, so a stolen session cannot lock the owner out.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "current_password", Kind: swagger.KindString, Required: true, Description: "The password in force."},
			{Name: "new_password", Kind: swagger.KindString, Required: true, Description: "The replacement password."},
		},
	},
	{
		Name:        "EmailInput",
		Description: "An email address to verify.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "email", Kind: swagger.KindString, Required: true, Description: "Address a verification message is sent to."},
		},
	},
	{
		Name:        "PasswordInput",
		Description: "A password confirmation guarding a security change.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "password", Kind: swagger.KindString, Required: true, Description: "The account password."},
		},
	},
	{
		Name:        "TokenInput",
		Description: "A request for an API token. The result is capped at the issuer's own role and never exceeds it.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "name", Kind: swagger.KindString, Required: true, Description: "Label for the token."},
			{Name: "org", Kind: swagger.KindString, Description: "Slug of the organization the token is confined to."},
			{Name: "zone_id", Kind: swagger.KindInt, Description: "Zone the token is confined to, narrower than an organization."},
			{Name: "capability", Kind: swagger.KindString, Description: "Narrower capability within the resource."},
			{Name: "role", Kind: swagger.KindString, Description: "Role requested. A higher role than the issuer holds is capped, not refused."},
			{Name: "scope", Kind: swagger.KindString, Description: "Breadth of the token: the whole account, one organization, or one zone."},
			{Name: "expires_in_days", Kind: swagger.KindInt, Description: "Lifetime in days. Absent means the configured default."},
		},
	},
	{
		Name:        "CreateOrgInput",
		Description: "A new shared organization. Which fields are required depends on the organization creation mode in force.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "slug", Kind: swagger.KindString, Required: true, Description: "URL segment that will address the organization."},
			{Name: "name", Kind: swagger.KindString, Required: true, Description: "Display name."},
			{Name: "description", Kind: swagger.KindString, Description: "Free-text description."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
			{Name: "invite", Kind: swagger.KindString, Description: "Invitation code. Required in invite mode, accepted in open mode."},
		},
	},
	{
		Name:        "OrgProfileInput",
		Description: "The editable parts of an organization profile.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "name", Kind: swagger.KindString, Description: "Display name."},
			{Name: "description", Kind: swagger.KindString, Description: "Free-text description."},
			{Name: "website", Kind: swagger.KindString, Description: "Profile link."},
			{Name: "location", Kind: swagger.KindString, Description: "Self-declared location."},
		},
	},
	{
		Name:        "OrgVisibilityInput",
		Description: "The visibility an organization profile should have.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "visibility", Kind: swagger.KindString, Required: true, Description: "public or private."},
		},
	},
	{
		Name:        "TransferOrgInput",
		Description: "The member who should hold the owner role.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "owner_id", Kind: swagger.KindInt, Required: true, Description: "Account identifier of an existing member."},
		},
	},
	{
		Name:        "MemberRoleInput",
		Description: "The role a member should hold.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "role", Kind: swagger.KindString, Required: true, Description: "owner, admin, editor, or viewer."},
		},
	},
	{
		Name:        "CreateInviteInput",
		Description: "An invitation into an organization.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "email", Kind: swagger.KindString, Description: "Restrict the invitation to one address."},
			{Name: "role", Kind: swagger.KindString, Required: true, Description: "Role granted on acceptance, never above the issuer's own."},
			{Name: "max_uses", Kind: swagger.KindInt, Description: "How many times the code may be redeemed."},
		},
	},
	{
		Name:        "ZoneGrantInput",
		Description: "A per-zone permission for one member.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "member_id", Kind: swagger.KindInt, Required: true, Description: "Member the grant belongs to."},
			{Name: "permission", Kind: swagger.KindString, Required: true, Description: "Permission granted on that zone."},
		},
	},
	{
		Name:        "AddDomainInput",
		Description: "A custom domain claim. The domain is stored unverified and is not served until ownership is proven.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "domain", Kind: swagger.KindString, Required: true, Description: "The domain name to claim."},
			{Name: "purpose", Kind: swagger.KindString, Description: "What the domain should front."},
		},
	},
	{
		Name:        "DomainPurposeInput",
		Description: "What a verified domain should be used for.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "purpose", Kind: swagger.KindString, Required: true, Description: "The purpose to store."},
		},
	},
	{
		Name:        "SuspendDomainInput",
		Description: "A reason for taking a domain out of service without releasing the claim.",
		Input:       true,
		Fields: []swagger.Field{
			{Name: "reason", Kind: swagger.KindString, Description: "Why the domain is being suspended."},
		},
	},
}

// preferenceFields are shared by the stored preferences and the body
// that writes them, because the two are the same shape. Declaring them
// once keeps the request and the response from drifting apart.
var preferenceFields = []swagger.Field{
	{Name: "show_email", Kind: swagger.KindBool, Required: true, Description: "Show the email address on the public profile."},
	{Name: "show_activity", Kind: swagger.KindBool, Required: true, Description: "Show recent activity on the public profile."},
	{Name: "show_orgs", Kind: swagger.KindBool, Required: true, Description: "Show organization memberships on the public profile."},
	{Name: "searchable", Kind: swagger.KindBool, Required: true, Description: "Allow the profile to be found by search."},
	{Name: "email_security", Kind: swagger.KindBool, Required: true, Description: "Receive security notices by email."},
	{Name: "email_org", Kind: swagger.KindBool, Required: true, Description: "Receive organization notices by email."},
	{Name: "email_product", Kind: swagger.KindBool, Required: true, Description: "Receive product announcements by email."},
	{Name: "theme", Kind: swagger.KindString, Required: true, Description: "light, dark, or auto."},
	{Name: "font_size", Kind: swagger.KindString, Required: true, Description: "Interface font size."},
	{Name: "reduce_motion", Kind: swagger.KindBool, Required: true, Description: "Suppress non-essential animation."},
	{Name: "date_format", Kind: swagger.KindString, Required: true, Description: "How dates are rendered."},
	{Name: "time_format", Kind: swagger.KindString, Required: true, Description: "How times are rendered."},
}

// Reusable parameter declarations. A path parameter that means the same
// thing on twenty routes is described once.
var (
	// slugParam addresses an organization.
	slugParam = swagger.Param{Name: "slug", In: swagger.ParamPath, Kind: swagger.KindString, Required: true, Description: "Slug of the organization. An organization the caller does not belong to is reported as absent rather than forbidden."}
	// memberParam addresses a member within an organization.
	memberParam = swagger.Param{Name: "member_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Account identifier of a member of that organization."}
	// domainParam addresses a custom domain within an organization.
	domainParam = swagger.Param{Name: "domain_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Identifier of a domain owned by that organization."}
	// zoneParam addresses a zone within an organization.
	zoneParam = swagger.Param{Name: "zone_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Identifier of a zone owned by that organization."}
)

// apiRoutes is the whole PART 34, 35 and 36 REST surface.
//
// Auth records the strongest credential that is refused: a route marked
// session accepts only a browser session, because it changes a
// credential and an API token must not be able to escalate itself. A
// route marked bearer accepts either a token or a session.
var apiRoutes = []apiRoute{
	{
		op: swagger.Operation{
			ID: "users.register", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/register", Tag: "auth", Auth: swagger.AuthNone,
			Summary:     "Register an account",
			Description: "Creates a Regular User account subject to the registration mode in force. Refused outright when registration is disabled.",
			RequestType: "RegisterInput", ResponseKind: swagger.KindObject, ResponseType: "User",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRegister },
	},
	{
		op: swagger.Operation{
			ID: "users.register.invite", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/invite/user/{code}", Tag: "auth", Auth: swagger.AuthNone,
			Summary:     "Register from an invitation link",
			Description: "Creates an account redeeming the invitation code carried in the path, so a mailed link needs no further input beyond the account details.",
			Params:      []swagger.Param{{Name: "code", In: swagger.ParamPath, Kind: swagger.KindString, Required: true, Description: "Invitation code. Only its hash is stored."}},
			RequestType: "RegisterInput", ResponseKind: swagger.KindObject, ResponseType: "User",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRegisterWithInvite },
	},
	{
		op: swagger.Operation{
			ID: "users.login", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/login", Tag: "auth", Auth: swagger.AuthNone,
			Summary:     "Sign in",
			Description: "Checks a credential and opens a session. A wrong password and an unknown account take the same work and give the same answer, so neither reveals whether the account exists.",
			RequestType: "SignInInput", ResponseKind: swagger.KindObject, ResponseType: "SignInResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiLogin },
	},
	{
		op: swagger.Operation{
			ID: "users.twofactor.challenge", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/2fa", Tag: "auth", Auth: swagger.AuthNone,
			Summary:     "Answer a second-factor challenge",
			Description: "Completes a session that signed in successfully but is still awaiting its second factor. Accepts a one-time code or a recovery code.",
			RequestType: "TwoFactorCodeInput", ResponseKind: swagger.KindObject, ResponseType: "SignInResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiTwoFactorChallenge },
	},
	{
		op: swagger.Operation{
			ID: "users.logout", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/logout", Tag: "auth", Auth: swagger.AuthSession,
			Summary:      "Sign out",
			Description:  "Ends the session carrying the request and clears its cookie.",
			ResponseKind: swagger.KindObject, ResponseType: "SignOutResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiLogout },
	},
	{
		op: swagger.Operation{
			ID: "users.password.forgot", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/password/forgot", Tag: "auth", Auth: swagger.AuthNone,
			Summary:     "Request a password reset",
			Description: "Sends a single-use reset token to the address on file. The same answer is given whether or not the account exists.",
			RequestType: "ForgotPasswordInput", ResponseKind: swagger.KindObject, ResponseType: "MailSentResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiPasswordForgot },
	},
	{
		op: swagger.Operation{
			ID: "users.password.reset", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/password/reset", Tag: "auth", Auth: swagger.AuthNone,
			Summary:     "Reset a password",
			Description: "Redeems a reset token and stores a new Argon2id hash. The token is single use and every existing session is ended.",
			RequestType: "ResetPasswordInput", ResponseKind: swagger.KindObject, ResponseType: "UpdateResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiPasswordReset },
	},
	{
		op: swagger.Operation{
			ID: "users.email.verify", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/server/auth/verify/{code}", Tag: "auth", Auth: swagger.AuthNone,
			Summary:      "Verify an email address",
			Description:  "Redeems the single-use code from a verification message and marks the address verified.",
			Params:       []swagger.Param{{Name: "code", In: swagger.ParamPath, Kind: swagger.KindString, Required: true, Description: "Verification code. Only its hash is stored."}},
			ResponseKind: swagger.KindObject, ResponseType: "UpdateResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiEmailVerify },
	},
	{
		op: swagger.Operation{
			ID: "users.me", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/users", Tag: "users", Auth: swagger.AuthBearer,
			Summary:      "Read the signed-in account",
			Description:  "Returns the account the credential names. No identifier appears in the path, so a caller cannot address somebody else's record by changing a segment.",
			ResponseKind: swagger.KindObject, ResponseType: "User",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiMe },
	},
	{
		op: swagger.Operation{
			ID: "users.profile.update", Method: "PATCH", Scope: swagger.ScopeAPI,
			Path: "/users", Tag: "users", Auth: swagger.AuthBearer,
			Summary:     "Update the signed-in account",
			Description: "Writes the editable parts of the caller's own profile.",
			RequestType: "ProfileInput", ResponseKind: swagger.KindObject, ResponseType: "User",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiUpdateProfile },
	},
	{
		op: swagger.Operation{
			ID: "users.preferences", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/users/settings", Tag: "users", Auth: swagger.AuthBearer,
			Summary:      "Read preferences",
			Description:  "Returns the caller's privacy, notification, and presentation settings.",
			ResponseKind: swagger.KindObject, ResponseType: "Preferences",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiPreferences },
	},
	{
		op: swagger.Operation{
			ID: "users.preferences.update", Method: "PATCH", Scope: swagger.ScopeAPI,
			Path: "/users/settings", Tag: "users", Auth: swagger.AuthBearer,
			Summary:     "Write preferences",
			Description: "Stores the caller's settings. The owner of the row is taken from the credential, never from the body.",
			RequestType: "PreferencesInput", ResponseKind: swagger.KindObject, ResponseType: "Preferences",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiSavePreferences },
	},
	{
		op: swagger.Operation{
			ID: "users.security", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/users/security", Tag: "users", Auth: swagger.AuthBearer,
			Summary:      "Summarize authentication state",
			Description:  "Reports address verification, second-factor state, and how many sessions and tokens are live.",
			ResponseKind: swagger.KindObject, ResponseType: "SecurityOverview",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiSecurity },
	},
	{
		op: swagger.Operation{
			ID: "users.password.change", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/users/security/password", Tag: "users", Auth: swagger.AuthSession,
			Summary:     "Change the password",
			Description: "Replaces the password after checking the current one. A token cannot reach this route, so a leaked token cannot take the account.",
			RequestType: "ChangePasswordInput", ResponseKind: swagger.KindObject, ResponseType: "UpdateResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiChangePassword },
	},
	{
		op: swagger.Operation{
			ID: "users.email.change", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/users/security/email", Tag: "users", Auth: swagger.AuthSession,
			Summary:     "Start email verification",
			Description: "Sends a verification message to an address. The address is not adopted until the code is redeemed.",
			RequestType: "EmailInput", ResponseKind: swagger.KindObject, ResponseType: "MailSentResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiStartEmailVerification },
	},
	{
		op: swagger.Operation{
			ID: "users.twofactor.start", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/users/security/2fa", Tag: "users", Auth: swagger.AuthSession,
			Summary:      "Begin second-factor enrollment",
			Description:  "Issues a TOTP seed and recovery codes. The seed is stored encrypted and the codes are stored hashed, so this response is the only place either appears in the clear.",
			ResponseKind: swagger.KindObject, ResponseType: "TwoFactorEnrollment",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiTwoFactorStart },
	},
	{
		op: swagger.Operation{
			ID: "users.twofactor.confirm", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/users/security/2fa/confirm", Tag: "users", Auth: swagger.AuthSession,
			Summary:     "Confirm second-factor enrollment",
			Description: "Proves the authenticator was configured before the second factor starts being demanded.",
			RequestType: "TwoFactorCodeInput", ResponseKind: swagger.KindObject, ResponseType: "TwoFactorState",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiTwoFactorConfirm },
	},
	{
		op: swagger.Operation{
			ID: "users.twofactor.disable", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/users/security/2fa", Tag: "users", Auth: swagger.AuthSession,
			Summary:     "Remove the second factor",
			Description: "Drops the stored seed and recovery codes after checking the account password.",
			RequestType: "PasswordInput", ResponseKind: swagger.KindObject, ResponseType: "TwoFactorState",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiTwoFactorDisable },
	},
	{
		op: swagger.Operation{
			ID: "users.sessions", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/users/sessions", Tag: "users", Auth: swagger.AuthBearer,
			Summary:      "List sessions",
			Description:  "Lists the caller's live sessions. The stored session hash is absent, because it is half of a live credential.",
			ResponseKind: swagger.KindObject, ResponseType: "SessionInfo", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiSessions },
	},
	{
		op: swagger.Operation{
			ID: "users.sessions.revoke", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/users/sessions/{session_id}", Tag: "users", Auth: swagger.AuthBearer,
			Summary:      "End a session",
			Description:  "Ends one of the caller's own sessions. A session belonging to anybody else is reported as absent.",
			Params:       []swagger.Param{{Name: "session_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Identifier of one of the caller's sessions."}},
			ResponseKind: swagger.KindObject, ResponseType: "RevokeResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRevokeSession },
	},
	{
		op: swagger.Operation{
			ID: "users.tokens", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/users/tokens", Tag: "tokens", Auth: swagger.AuthBearer,
			Summary:      "List API tokens",
			Description:  "Lists the caller's tokens. Only the prefix of each secret is shown; the secret itself is stored as a SHA-256 hash.",
			ResponseKind: swagger.KindObject, ResponseType: "Token", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiListTokens },
	},
	{
		op: swagger.Operation{
			ID: "users.tokens.issue", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/users/tokens", Tag: "tokens", Auth: swagger.AuthSession,
			Summary:     "Issue an API token",
			Description: "Creates a token scoped to the account, to one organization, or to one zone. The role is capped at the issuer's own, so a token can never exceed the member who made it.",
			RequestType: "TokenInput", ResponseKind: swagger.KindObject, ResponseType: "IssuedToken",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiIssueToken },
	},
	{
		op: swagger.Operation{
			ID: "users.tokens.revoke", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/users/tokens/{token_id}", Tag: "tokens", Auth: swagger.AuthSession,
			Summary:      "Revoke an API token",
			Description:  "Revokes one of the caller's own tokens. A token belonging to anybody else is reported as absent.",
			Params:       []swagger.Param{{Name: "token_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Identifier of one of the caller's tokens."}},
			ResponseKind: swagger.KindObject, ResponseType: "RevokeResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRevokeToken },
	},
	{
		op: swagger.Operation{
			ID: "users.invites.accept", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/users/invites/{code}", Tag: "orgs", Auth: swagger.AuthSession,
			Summary:      "Accept an invitation",
			Description:  "Joins the caller to an organization. It lives in the user scope because it acts on the caller, who is not yet a member of the organization named by the code.",
			Params:       []swagger.Param{{Name: "code", In: swagger.ParamPath, Kind: swagger.KindString, Required: true, Description: "Invitation code. Only its hash is stored."}},
			ResponseKind: swagger.KindObject, ResponseType: "Org",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiAcceptInvite },
	},
	{
		op: swagger.Operation{
			ID: "users.profile", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/users/{username}", Tag: "users", Auth: swagger.AuthNone,
			Summary:      "Read a public profile",
			Description:  "Serves a user's vanity profile filtered by that user's own privacy settings. A private profile is reported as absent, so the setting does not itself disclose that the account exists.",
			Params:       []swagger.Param{{Name: "username", In: swagger.ParamPath, Kind: swagger.KindString, Required: true, Description: "Vanity URL segment of the account."}},
			ResponseKind: swagger.KindObject, ResponseType: "PublicProfile",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiUserProfile },
	},
	{
		op: swagger.Operation{
			ID: "orgs.list", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "List organizations",
			Description:  "Lists the organizations the caller belongs to, personal organization included.",
			ResponseKind: swagger.KindObject, ResponseType: "Org", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiListOrgs },
	},
	{
		op: swagger.Operation{
			ID: "orgs.create", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs", Tag: "orgs", Auth: swagger.AuthSession,
			Summary:     "Create an organization",
			Description: "Creates a shared organization subject to the creation mode in force. The caller becomes its owner.",
			RequestType: "CreateOrgInput", ResponseKind: swagger.KindObject, ResponseType: "Org",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiCreateOrg },
	},
	{
		op: swagger.Operation{
			ID: "orgs.read", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "Read an organization",
			Description:  "Returns an organization the caller belongs to, together with the role and permissions they hold in it.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "OrgAccess",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiOrg },
	},
	{
		op: swagger.Operation{
			ID: "orgs.update", Method: "PATCH", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Update an organization profile",
			Description: "Writes the organization's display fields. Requires a role carrying the settings permission.",
			Params:      []swagger.Param{slugParam},
			RequestType: "OrgProfileInput", ResponseKind: swagger.KindObject, ResponseType: "Org",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiUpdateOrg },
	},
	{
		op: swagger.Operation{
			ID: "orgs.delete", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "Delete an organization",
			Description:  "Deletes a shared organization. A personal organization cannot be deleted, because the account it belongs to would be left with nowhere to hold its zones.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "DeleteResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiDeleteOrg },
	},
	{
		op: swagger.Operation{
			ID: "orgs.settings", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/settings", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "Read organization settings",
			Description:  "Returns the editable profile together with whether the caller's role may change it.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "OrgSettings",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiOrgSettings },
	},
	{
		op: swagger.Operation{
			ID: "orgs.settings.update", Method: "PATCH", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/settings", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Write organization settings",
			Description: "Changes whether the organization's profile is public.",
			Params:      []swagger.Param{slugParam},
			RequestType: "OrgVisibilityInput", ResponseKind: swagger.KindObject, ResponseType: "VisibilityResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiUpdateOrgSettings },
	},
	{
		op: swagger.Operation{
			ID: "orgs.transfer", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/transfer", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Transfer ownership",
			Description: "Moves the owner role to another existing member. Only the current owner may do it, and the change is written to the audit log.",
			Params:      []swagger.Param{slugParam},
			RequestType: "TransferOrgInput", ResponseKind: swagger.KindObject, ResponseType: "TransferResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiTransferOrg },
	},
	{
		op: swagger.Operation{
			ID: "orgs.members", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/members", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "List members",
			Description:  "Lists the accounts belonging to the organization and the role each holds.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "Member", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiMembers },
	},
	{
		op: swagger.Operation{
			ID: "orgs.members.role", Method: "PATCH", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/members/{member_id}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Change a member's role",
			Description: "Sets the role of one member. The new role can never exceed the actor's own, and the change is written to the audit log.",
			Params:      []swagger.Param{slugParam, memberParam},
			RequestType: "MemberRoleInput", ResponseKind: swagger.KindObject, ResponseType: "MemberRoleResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiSetMemberRole },
	},
	{
		op: swagger.Operation{
			ID: "orgs.members.remove", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/members/{member_id}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "Remove a member",
			Description:  "Withdraws a membership. Every token that member holds against this organization is revoked with it, so leaving the organization also leaves nothing behind that could still reach it.",
			Params:       []swagger.Param{slugParam, memberParam},
			ResponseKind: swagger.KindObject, ResponseType: "RemoveResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRemoveMember },
	},
	{
		op: swagger.Operation{
			ID: "orgs.invites", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/invites", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "List invitations",
			Description:  "Lists pending invitations into the organization. Their codes are absent, because only the hashes are stored.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "Invite", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiListInvites },
	},
	{
		op: swagger.Operation{
			ID: "orgs.invites.create", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/invites", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Issue an invitation",
			Description: "Creates an invitation code granting a role no higher than the issuer's own. The code is returned once and stored only as a hash.",
			Params:      []swagger.Param{slugParam},
			RequestType: "CreateInviteInput", ResponseKind: swagger.KindObject, ResponseType: "IssuedInvite",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiCreateInvite },
	},
	{
		op: swagger.Operation{
			ID: "orgs.invites.revoke", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/invites/{invite_id}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "Revoke an invitation",
			Description:  "Stops a pending invitation from being redeemable.",
			Params:       []swagger.Param{slugParam, {Name: "invite_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Identifier of an invitation into that organization."}},
			ResponseKind: swagger.KindObject, ResponseType: "RevokeResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRevokeInvite },
	},
	{
		op: swagger.Operation{
			ID: "orgs.audit", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/audit", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Read the audit log",
			Description: "Lists recorded changes inside the organization, newest first. Membership and role changes always appear here.",
			Params: []swagger.Param{
				slugParam,
				{Name: "limit", In: swagger.ParamQuery, Kind: swagger.KindInt, Description: "How many entries to return. Defaults to 50."},
				{Name: "offset", In: swagger.ParamQuery, Kind: swagger.KindInt, Description: "How many entries to skip. Defaults to 0."},
			},
			ResponseKind: swagger.KindObject, ResponseType: "AuditEntry", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiOrgAudit },
	},
	{
		op: swagger.Operation{
			ID: "orgs.tokens", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/tokens", Tag: "tokens", Auth: swagger.AuthBearer,
			Summary:      "List organization tokens",
			Description:  "Lists the tokens issued against this organization by any member.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "Token", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiListOrgTokens },
	},
	{
		op: swagger.Operation{
			ID: "orgs.tokens.revoke", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/tokens/{token_id}", Tag: "tokens", Auth: swagger.AuthBearer,
			Summary:      "Revoke an organization token",
			Description:  "Revokes a token issued against this organization. Requires a role carrying the token permission.",
			Params:       []swagger.Param{slugParam, {Name: "token_id", In: swagger.ParamPath, Kind: swagger.KindInt, Required: true, Description: "Identifier of a token issued against that organization."}},
			ResponseKind: swagger.KindObject, ResponseType: "RevokeResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRevokeOrgToken },
	},
	{
		op: swagger.Operation{
			ID: "orgs.zones.grant", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/zones/{zone_id}/grants", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:     "Grant a member access to one zone",
			Description: "Narrows a member's reach to a single zone within the organization. The zone must belong to the organization in the path.",
			Params:      []swagger.Param{slugParam, zoneParam},
			RequestType: "ZoneGrantInput", ResponseKind: swagger.KindObject, ResponseType: "ZoneGrant",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiGrantZone },
	},
	{
		op: swagger.Operation{
			ID: "orgs.zones.revoke", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/zones/{zone_id}/grants/{member_id}", Tag: "orgs", Auth: swagger.AuthBearer,
			Summary:      "Withdraw a member's zone access",
			Description:  "Removes a per-zone grant from one member.",
			Params:       []swagger.Param{slugParam, zoneParam, memberParam},
			ResponseKind: swagger.KindObject, ResponseType: "RevokeResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRevokeZone },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:      "List custom domains",
			Description:  "Lists the domains claimed by the organization. The verification token is absent, so an ordinary listing does not disclose the challenge value.",
			Params:       []swagger.Param{slugParam},
			ResponseKind: swagger.KindObject, ResponseType: "Domain", ResponseList: true,
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiListDomains },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains.add", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:     "Claim a custom domain",
			Description: "Records a domain as pending. Claiming it proves nothing and serves nothing; the domain is inert until ownership is verified.",
			Params:      []swagger.Param{slugParam},
			RequestType: "AddDomainInput", ResponseKind: swagger.KindObject, ResponseType: "Domain",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiAddDomain },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains.update", Method: "PATCH", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains/{domain_id}", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:     "Change what a domain fronts",
			Description: "Writes the purpose of a domain the organization already owns.",
			Params:      []swagger.Param{slugParam, domainParam},
			RequestType: "DomainPurposeInput", ResponseKind: swagger.KindObject, ResponseType: "DomainPurposeResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiUpdateDomain },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains.remove", Method: "DELETE", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains/{domain_id}", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:      "Release a custom domain",
			Description:  "Drops the claim and stops serving the name.",
			Params:       []swagger.Param{slugParam, domainParam},
			ResponseKind: swagger.KindObject, ResponseType: "RemoveResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiRemoveDomain },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains.challenge", Method: "GET", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains/{domain_id}/verification", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:      "Read the ownership challenge",
			Description:  "Returns the TXT record that must be published to prove control of the domain. It requires the organization settings permission, so a viewer cannot read the challenge value.",
			Params:       []swagger.Param{slugParam, domainParam},
			ResponseKind: swagger.KindObject, ResponseType: "DomainChallenge",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiDomainVerification },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains.verify", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains/{domain_id}/verify", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:      "Prove ownership of a domain",
			Description:  "Resolves the published TXT record and activates the domain only if it carries the issued token. On success a certificate is requested through the server's existing ACME manager, never through a second TLS path.",
			Params:       []swagger.Param{slugParam, domainParam},
			ResponseKind: swagger.KindObject, ResponseType: "Domain",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiVerifyDomain },
	},
	{
		op: swagger.Operation{
			ID: "orgs.domains.suspend", Method: "POST", Scope: swagger.ScopeAPI,
			Path: "/orgs/{slug}/domains/{domain_id}/suspend", Tag: "domains", Auth: swagger.AuthBearer,
			Summary:     "Stop serving a domain",
			Description: "Takes a domain out of service while leaving the claim and its verification in place.",
			Params:      []swagger.Param{slugParam, domainParam},
			RequestType: "SuspendDomainInput", ResponseKind: swagger.KindObject, ResponseType: "SuspendResult",
		},
		handler: func(h *Handler) http.HandlerFunc { return h.apiSuspendDomain },
	},
}
