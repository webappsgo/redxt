# Admin Panel

## Access

The admin panel is isolated under `/server/{admin_path}` — the path
segment defaults to `administration` but is fully configurable in
`yaml.d/accounts.yml` (or a merged configuration drop-in).

The path segment must be 2–32 lowercase letters, numbers, or interior
hyphens. Server routes such as `api`, `health`, `about`, `privacy`,
`security`, and `preferences` are reserved; invalid configuration falls back
to `administration`.

On first run, redxt serves a setup wizard at `/server/{admin_path}/setup`
and prints a one-time setup token to the console. It also creates the
operator API token (`server.token`) and prints that credential once. The
operator token is stored in `secrets.yml`, never in `server.yml` or the
database. Completing the wizard creates the **Primary Admin** account, which
cannot be deleted.
The setup form defaults the username to `steward`; operators may choose
another name that passes the same checks.
Admin usernames must pass the shared reserved-name and route-collision
checks. The additional blocklist used for Regular User usernames does not
apply to Server Admins; a reserved or conflicting setup name receives the
same generic conflict response as another name collision.

Sign-in for admins and regular users happens through the server-rendered
login form at `/server/auth/login` (`GET` renders the form, `POST`
authenticates and starts a session). Admin sessions have a 30-day absolute
lifetime, matching the server admin session policy in AI.md PART 17.

## Available pages

The current admin panel provides the first-run setup wizard, dashboard,
application name and tagline settings, editable SSL/TLS and ACME settings at
`/server/{admin_path}/ssl`, Server Admin API token management at
`/server/{admin_path}/security/tokens`, server information, theme choice, per-admin
appearance preferences, scheduler controls, log viewer, email delivery
settings, update controls, and logout. The
Pages → Help editor at `/server/{admin_path}/pages/help` updates the public
MCP guide stored in `server.pages.help.mcp` under `yaml.d/pages.yml`.
The SSL/TLS form includes the complete
lego DNS provider list and a free-form `VARIABLE=value` credential editor;
credential values are never returned to the page. Operators can set certificate
and private-key paths or upload a matching PEM pair; uploaded files are stored
under the configured SSL directory with private-key access restricted to the
server owner. Listener, certificate, private-key, and TLS version changes require
a server restart. The preference page is
`/server/{admin_path}/{admin_username}/preferences`; it stores theme,
font size, reduced motion, date format, and time format in the administrator's
own preferences. Pages require an authenticated Server Admin session; a
Regular User is redirected to their own account area. An administrator can
only read or update their own preference page.

Each Server Admin can register an OpenPGP public key at
`/server/{admin_path}/{admin_username}/profile/security/pgp` for encrypted
coordinated-disclosure notifications. The matching bearer API profile GET
returns that admin's key; PATCH accepts `pgp_public_key`, validates that it is
public-key material, and only updates the token owner's own profile.

The coordinated-disclosure keypair is managed at
`/server/{admin_path}/security/keypair`. Deletion requires the administrator's
password and typing `DELETE`; it revokes publication and removes both key
files. Stored PGP-encrypted reports remain in retention but cannot be
decrypted after deletion.

The Email page at `/server/{admin_path}/email` edits SMTP host, port,
credentials, TLS mode, and sender identity. SMTP passwords are never shown
after saving and are persisted through the protected secrets file. The test
action sends a message to the signed-in administrator's email address.
`/server/{admin_path}/email/templates` lists embedded and customized message
templates. Each editor supports server-rendered preview, validation, sending a
sample test message to a chosen address (recorded in the audit log), saving a
custom template, and restoring its embedded default.

The scheduler page at `/server/{admin_path}/scheduler` lists registered
internal tasks, their schedules, enabled state, last run, and next run. A
Server Admin can choose an hourly, daily, weekly, monthly, or custom schedule,
enable or disable a task, inspect its recent run history, and trigger it
immediately. Schedule edits update the live scheduler and persist for the next
restart. Manual runs are started asynchronously and remain available when a
task's regular schedule is disabled.

The Updates page is available at `/server/{admin_path}/updates`; the singular
`/update` route is an alias. It stores the release channel, scheduled install
choice, defer window, and minimum CLI and agent versions. Server Admins can
check for the latest channel release or explicitly confirm the verified
install flow, which validates the release checksum before replacing the
server binary and restarting the service.

The log viewer at `/server/{admin_path}/logs` filters the configured Access,
Error, Audit, Security, and (when enabled) Debug logs by text, selects the
last 100, 500, or 1,000 matching entries, displays them with pagination, and
supports download. It can clear
any selected log except Audit; the audit log is append-only and may be removed
only by its configured rotation policy. Auto-refresh reloads the current view
every 15 seconds when enabled.

The Agents page at `/server/{admin_path}/agents` lists admin-scoped agents,
their connection state, last-seen time, and health summary. `/agents/add`
creates a pending agent and a 24-hour, one-use registration token; the token is
shown once. Selecting an agent opens its details page. Admins can edit an
agent's name and tags, remove it (revoking its active token and deleting its
metrics), regenerate its token, and read recent metrics through the bearer
API. Registration, heartbeat, and metrics reports use an `adm_` token with
scope `agent`. User- and organization-scoped agent management is not
implemented yet.

## Database maintenance dashboard

If either database cannot be opened or checked during startup, the usual
database-backed admin panel is unavailable. The configured admin path still
serves a read-only maintenance dashboard with the retry count, next attempt,
and general recovery checks. It does not display credentials or raw database
errors. State-changing requests receive HTTP 503 with the canonical
`MAINTENANCE` error and `Retry-After` header; the health endpoint also returns
503 until recovery succeeds. The dashboard does not provide database repair
controls, so operators must resolve the database or storage issue at the
service or configuration level.

The bearer-authenticated SSL API is available at
`/api/{api_version}/server/{admin_path}/ssl` (`GET` and `PATCH`) and
`/api/{api_version}/server/{admin_path}/ssl/renew` (`POST`). It accepts the
operator token or a usable stored Server Admin API token in the
`Authorization: Bearer` header. DNS credential values can be updated through
the API but are never included in its responses; responses include configured
credential variable names only.

An admin can inspect the masked default API token and replace it at
`/api/{api_version}/server/{admin_path}/{admin_username}/profile/token`.
`GET` returns only the token prefix and a mask. `POST` returns a newly
generated `adm_` token once and invalidates the previous default token. These
profile routes require a stored token belonging to the named admin; the
operator token cannot read or rotate an admin's personal token.

The API Tokens page creates `adm_` tokens with global, read-write, or read
scope and the configured default, Never, preset, or custom expiration. A new token is shown once; the
database stores only its SHA-256 hash. The page lists only the signed-in
administrator's own tokens and revokes them without deleting their audit
history. Creation and revocation are recorded as `token.created` and
`token.revoked` events. Active tokens are marked for rotation when they reach
the configured reminder age. The bearer API also provides the server-wide token
inventory, revocation, and rotation routes documented in [the API reference](api.md).
The inventory returns metadata only; rotation preserves the token's current
scope and expiry, invalidates its prior secret, and returns the replacement
once with `Cache-Control: no-store`.

Guest appearance and language preferences are separate from the admin
panel and live at `/server/preferences`. This path is reserved and cannot
be configured as the admin path.

The backup page supports manual creation, listing, archive verification,
restore, deletion with incremental-base protection, and archive downloads.
Leaving the test selection blank verifies the newest full backup and every
available daily/hourly incremental with its base; choosing an archive verifies
only that archive and its required base.
Restore requires the typed confirmation `RESTORE`; encrypted archives also
require their password. A successful restore drains the current server and
exits with the service restart code after returning its response. The bearer
API at `/api/{api_version}/server/{admin_path}/backup` supports archive listing,
creation, metadata lookup, verification, deletion, and download. `POST
/api/{api_version}/server/{admin_path}/backup/restore` accepts `backup_file` and
an optional `password`; it requires a global-scope token and requests the same
graceful restart after the restore completes. Backup policy editing remains
unavailable in the panel/API. The wider settings, audit, and cluster surfaces
remain incomplete. The browser UI is server-rendered
and its forms work without JavaScript; clipboard support for one-time tokens is
an optional enhancement.

Admin REST routes default to JSON. `Accept: text/plain`, a detected HTTP tool,
or a `.txt` suffix returns the shared plain-text API representation; an
explicit `Accept: application/json` takes priority over client detection. An
authorized `.txt` request for a backup download returns `404`; an unauthenticated
request keeps its authentication error. Ordinary backup downloads retain their
binary media type and bytes for every `Accept` value.
