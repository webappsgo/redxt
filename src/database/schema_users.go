package database

import "slices"

// usersTables is the full users.db object list: accounts and organizations
// first, then the org-owned DNS data that depends on them by foreign key.
var usersTables = slices.Concat(usersCoreTables, usersDNSTables)

// usersUpdates is the full users.db additive update list. The same PART 10
// rules apply as for serverUpdates: idempotent, defaulted, add-only.
var usersUpdates = slices.Concat(usersCoreUpdates, usersDNSUpdates)

// usersCoreTables holds accounts, sessions, and organizations. Per the PART 5
// "Full Database Schema Summary" rule, anything a user owns lives here.
var usersCoreTables = []string{
	// admins holds the server administrator accounts. password_hash is an
	// Argon2id encoded string ("$argon2id$v=19$m=...,t=...,p=...$salt$hash"),
	// which carries its own parameters and salt, so no separate columns are
	// needed and no plaintext ever reaches the database.
	`CREATE TABLE IF NOT EXISTS admins (
		id             INTEGER PRIMARY KEY,
		username       TEXT NOT NULL UNIQUE,
		email          TEXT NOT NULL DEFAULT '',
		password_hash  TEXT NOT NULL,
		must_change_pw INTEGER NOT NULL DEFAULT 0,
		disabled       INTEGER NOT NULL DEFAULT 0,
		last_login_at  TIMESTAMP,
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// admin_sessions holds the Server Admin web sessions (PART 17). It lives
	// alongside admins in users.db rather than in server.db, since a session
	// cannot outlive the account it authenticates and the two are always
	// read together. The session id is stored hashed for the same reason
	// tokens are: a database read must not yield a usable credential.
	`CREATE TABLE IF NOT EXISTS admin_sessions (
		id              INTEGER PRIMARY KEY,
		session_hash    TEXT NOT NULL UNIQUE,
		admin_id        INTEGER NOT NULL,
		ip_address      TEXT NOT NULL DEFAULT '',
		user_agent      TEXT NOT NULL DEFAULT '',
		two_factor_ok   INTEGER NOT NULL DEFAULT 0,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_active_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at      TIMESTAMP NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON admin_sessions(expires_at)`,

	// users holds the regular end-user accounts (PART 34). password_hash is
	// an Argon2id encoded string, as for admins. locked_until drives the
	// brute-force lockout; failed_logins is the counter behind it.
	// visibility is the PART 34 profile privacy setting, "public" or
	// "private"; a private profile answers 404 rather than 403 so its
	// existence is not disclosed. org_visibility is the PART 35 switch
	// that lets a private user still show basic info to fellow org
	// members. notification_email is optional and falls back to the
	// account email, which is the security-critical address.
	`CREATE TABLE IF NOT EXISTS users (
		id                 INTEGER PRIMARY KEY,
		username           TEXT NOT NULL UNIQUE,
		email              TEXT NOT NULL UNIQUE,
		email_verified     INTEGER NOT NULL DEFAULT 0,
		notification_email TEXT NOT NULL DEFAULT '',
		password_hash      TEXT NOT NULL,
		display_name       TEXT NOT NULL DEFAULT '',
		bio                TEXT NOT NULL DEFAULT '',
		location           TEXT NOT NULL DEFAULT '',
		website            TEXT NOT NULL DEFAULT '',
		avatar_url         TEXT NOT NULL DEFAULT '',
		timezone           TEXT NOT NULL DEFAULT '',
		language           TEXT NOT NULL DEFAULT '',
		role               TEXT NOT NULL DEFAULT 'user',
		visibility         TEXT NOT NULL DEFAULT 'public',
		org_visibility     INTEGER NOT NULL DEFAULT 1,
		status             TEXT NOT NULL DEFAULT 'active',
		failed_logins      INTEGER NOT NULL DEFAULT 0,
		locked_until       TIMESTAMP,
		last_login_at      TIMESTAMP,
		created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
	`CREATE INDEX IF NOT EXISTS idx_users_org_visibility ON users(visibility, org_visibility)`,

	// user_preferences is created lazily on first read, per PART 34's
	// "Preferences" section: a row exists only once a user has changed
	// something away from the defaults.
	`CREATE TABLE IF NOT EXISTS user_preferences (
		user_id        INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		show_email     INTEGER NOT NULL DEFAULT 0,
		show_activity  INTEGER NOT NULL DEFAULT 1,
		show_orgs      INTEGER NOT NULL DEFAULT 1,
		searchable     INTEGER NOT NULL DEFAULT 1,
		email_security INTEGER NOT NULL DEFAULT 1,
		email_org      INTEGER NOT NULL DEFAULT 1,
		email_product  INTEGER NOT NULL DEFAULT 0,
		theme          TEXT NOT NULL DEFAULT 'auto',
		font_size      TEXT NOT NULL DEFAULT 'medium',
		reduce_motion  INTEGER NOT NULL DEFAULT 0,
		date_format    TEXT NOT NULL DEFAULT 'iso',
		time_format    TEXT NOT NULL DEFAULT '24h',
		updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// user_sessions holds the end-user web sessions, storing only the hash of
	// the session identifier.
	`CREATE TABLE IF NOT EXISTS user_sessions (
		id             INTEGER PRIMARY KEY,
		session_hash   TEXT NOT NULL UNIQUE,
		user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		ip_address     TEXT NOT NULL DEFAULT '',
		user_agent     TEXT NOT NULL DEFAULT '',
		two_factor_ok  INTEGER NOT NULL DEFAULT 0,
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_active_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at     TIMESTAMP NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON user_sessions(expires_at)`,

	// password_resets holds single-use password reset tokens, stored hashed
	// so a database read cannot be replayed as a reset.
	`CREATE TABLE IF NOT EXISTS password_resets (
		id         INTEGER PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE,
		used_at    TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL
	)`,

	// email_verifications holds single-use email confirmation tokens, stored
	// hashed for the same reason.
	`CREATE TABLE IF NOT EXISTS email_verifications (
		id         INTEGER PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		email      TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		used_at    TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL
	)`,

	// totp_secrets holds the TOTP second factor.
	//
	// SECURITY: secret_encrypted is the AES-256-GCM ciphertext of the TOTP
	// secret under server.security.encryption_key, never the plaintext
	// secret and never a hash. A hash is not an option here because the
	// server must recover the secret to compute the expected code.
	// key_version records which encryption key generation produced the
	// ciphertext so a key rotation can still decrypt during its grace window.
	//
	// recovery_codes holds the hashes of the one-time recovery codes; those
	// are verification-only, so hashing suffices and encryption is not used.
	`CREATE TABLE IF NOT EXISTS totp_secrets (
		user_id          INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
		secret_encrypted TEXT NOT NULL,
		key_version      INTEGER NOT NULL DEFAULT 1,
		confirmed        INTEGER NOT NULL DEFAULT 0,
		recovery_codes   TEXT NOT NULL DEFAULT '[]',
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// passkeys holds WebAuthn/FIDO2 credentials. Only public key material is
	// stored; the authenticator keeps the private half.
	`CREATE TABLE IF NOT EXISTS passkeys (
		id             INTEGER PRIMARY KEY,
		user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		credential_id  TEXT NOT NULL UNIQUE,
		public_key     TEXT NOT NULL,
		sign_count     INTEGER NOT NULL DEFAULT 0,
		name           TEXT NOT NULL DEFAULT '',
		transports     TEXT NOT NULL DEFAULT '[]',
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_used_at   TIMESTAMP
	)`,

	// trusted_devices remembers a device that has already satisfied the
	// second factor, keyed by a hashed device token.
	`CREATE TABLE IF NOT EXISTS trusted_devices (
		id          INTEGER PRIMARY KEY,
		user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash  TEXT NOT NULL UNIQUE,
		label       TEXT NOT NULL DEFAULT '',
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at  TIMESTAMP NOT NULL
	)`,

	// organizations is the PART 35 tenancy boundary and the owner of every
	// zone, key, policy, and DDNS host in redxt. personal marks the org
	// automatically created for a single user.
	`CREATE TABLE IF NOT EXISTS organizations (
		id          INTEGER PRIMARY KEY,
		slug        TEXT NOT NULL UNIQUE,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		website     TEXT NOT NULL DEFAULT '',
		location    TEXT NOT NULL DEFAULT '',
		avatar_url  TEXT NOT NULL DEFAULT '',
		visibility  TEXT NOT NULL DEFAULT 'public',
		personal    INTEGER NOT NULL DEFAULT 0,
		owner_id    INTEGER NOT NULL REFERENCES users(id),
		status      TEXT NOT NULL DEFAULT 'active',
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// organization_members maps users to organizations with one of the four
	// IDEA.md org roles: owner, admin, editor, viewer.
	`CREATE TABLE IF NOT EXISTS organization_members (
		org_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role       TEXT NOT NULL DEFAULT 'viewer',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (org_id, user_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_org_members_user ON organization_members(user_id)`,

	// organization_audit records every membership and settings change, as
	// the IDEA.md threat model requires: "membership changes audited".
	`CREATE TABLE IF NOT EXISTS organization_audit (
		id         INTEGER PRIMARY KEY,
		org_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		event      TEXT NOT NULL,
		actor_type TEXT NOT NULL DEFAULT 'user',
		actor_id   INTEGER,
		target_id  INTEGER,
		details    TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_org_audit_org ON organization_audit(org_id)`,

	// api_tokens is the single credential table for the PART 34 token
	// prefixes: adm_ for a Server Admin, usr_ for a Regular User, org_
	// for an organization. Only the SHA-256 hash is stored;
	// token_prefix is the leading characters kept solely so the UI can
	// show which token a row is without holding the secret.
	//
	// org_id, zone_id, and capability implement the IDEA.md credential
	// scoping rule: a token is scoped to one organization or narrower —
	// one zone, or one capability such as records:write. A zero zone_id
	// or an empty capability means the token is not narrowed on that
	// axis. role records the authority the token was capped to at
	// issue time and can never exceed the issuer's own role.
	//
	// The scope column is deliberately wider than the API tokens that
	// exist today: the DNS credential types in the IDEA.md table (TSIG
	// keys, GSS-TSIG identities, DDNS host tokens, and the agent
	// prefixes) attach to this same row shape when the PARTs that own
	// them are implemented, without a schema change here.
	`CREATE TABLE IF NOT EXISTS api_tokens (
		id           INTEGER PRIMARY KEY,
		owner_type   TEXT NOT NULL,
		owner_id     INTEGER NOT NULL,
		name         TEXT NOT NULL DEFAULT '',
		token_hash   TEXT NOT NULL UNIQUE,
		token_prefix TEXT NOT NULL DEFAULT '',
		scope        TEXT NOT NULL DEFAULT 'read',
		role         TEXT NOT NULL DEFAULT '',
		org_id       INTEGER NOT NULL DEFAULT 0,
		zone_id      INTEGER NOT NULL DEFAULT 0,
		capability   TEXT NOT NULL DEFAULT '',
		last_used_at TIMESTAMP,
		revoked_at   TIMESTAMP,
		expires_at   TIMESTAMP,
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_api_tokens_owner ON api_tokens(owner_type, owner_id)`,

	// zone_grants narrows an Editor's authority to specific zones, which is
	// what the IDEA.md Editor role requires: create and edit records "in
	// assigned zones" rather than across the whole organization.
	`CREATE TABLE IF NOT EXISTS zone_grants (
		org_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		zone_id    INTEGER NOT NULL,
		permission TEXT NOT NULL DEFAULT 'edit',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (org_id, user_id, zone_id)
	)`,

	// invitations backs the invite-default registration mode from IDEA.md.
	// The invite code is stored hashed; the plaintext code exists only in the
	// invitation message.
	// max_uses defaults to a single redemption; zero means unlimited, per
	// the PART 34 invite rules. use_count is the redemption counter that
	// max_uses is compared against.
	`CREATE TABLE IF NOT EXISTS invitations (
		id          INTEGER PRIMARY KEY,
		org_id      INTEGER REFERENCES organizations(id) ON DELETE CASCADE,
		email       TEXT NOT NULL DEFAULT '',
		role        TEXT NOT NULL DEFAULT 'viewer',
		code_hash   TEXT NOT NULL UNIQUE,
		max_uses    INTEGER NOT NULL DEFAULT 1,
		use_count   INTEGER NOT NULL DEFAULT 0,
		invited_by  INTEGER REFERENCES users(id),
		accepted_at TIMESTAMP,
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at  TIMESTAMP NOT NULL
	)`,

	// custom_domains holds the PART 36 org-owned hostnames used for DDNS
	// signup, the redirector, parking pages, and the data gateways.
	// verification_token is the TXT challenge value proving control.
	// A domain is owned by an organization rather than by the
	// owner_type/owner_id pair PART 36 sketches, because IDEA.md is
	// narrower than the spec here: in redxt every zone, policy, key, and
	// DDNS host belongs to an organization, and every user has a
	// personal organization, so org_id expresses both cases with one
	// foreign key that the database can actually enforce.
	//
	// verification_status must reach "verified" before status may become
	// "active": PART 36 never activates an unverified domain.
	`CREATE TABLE IF NOT EXISTS custom_domains (
		id                  INTEGER PRIMARY KEY,
		org_id              INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		domain              TEXT NOT NULL UNIQUE,
		purpose             TEXT NOT NULL DEFAULT 'ui',
		is_apex             INTEGER NOT NULL DEFAULT 0,
		is_wildcard         INTEGER NOT NULL DEFAULT 0,
		verification_status TEXT NOT NULL DEFAULT 'pending',
		verification_token  TEXT NOT NULL DEFAULT '',
		verified_at         TIMESTAMP,
		last_check_at       TIMESTAMP,
		check_count         INTEGER NOT NULL DEFAULT 0,
		ssl_enabled         INTEGER NOT NULL DEFAULT 0,
		ssl_status          TEXT NOT NULL DEFAULT 'none',
		ssl_challenge       TEXT NOT NULL DEFAULT '',
		ssl_issued_at       TIMESTAMP,
		ssl_expires_at      TIMESTAMP,
		ssl_last_error      TEXT NOT NULL DEFAULT '',
		status              TEXT NOT NULL DEFAULT 'pending',
		suspended_reason    TEXT NOT NULL DEFAULT '',
		created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_custom_domains_org ON custom_domains(org_id)`,
	`CREATE INDEX IF NOT EXISTS idx_custom_domains_status ON custom_domains(status)`,
	`CREATE INDEX IF NOT EXISTS idx_custom_domains_ssl_expires ON custom_domains(ssl_expires_at)`,

	// custom_domain_audit records every state change of a custom domain.
	`CREATE TABLE IF NOT EXISTS custom_domain_audit (
		id         INTEGER PRIMARY KEY,
		domain_id  INTEGER NOT NULL REFERENCES custom_domains(id) ON DELETE CASCADE,
		event      TEXT NOT NULL,
		actor_type TEXT NOT NULL DEFAULT 'user',
		actor_id   INTEGER,
		details    TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_domain_audit_domain ON custom_domain_audit(domain_id)`,
}

// usersCoreUpdates holds the additive users.db schema changes. It is empty at
// the initial schema version; new columns append here with a comment naming
// the version that introduced them.
var usersCoreUpdates = []string{}
