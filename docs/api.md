# API Reference

## REST API

Management REST routes are versioned under `/api/{api_version}/...`
(default `v1`). REST success and error responses use the shared API
envelope. Swagger returns an OpenAPI document, and GraphQL uses its
protocol-defined response format; both describe the same REST surface.

### Endpoints

The configured API version defaults to `v1`; replace `{api_version}` below
with that value. Routes and request/response schemas are also exposed by the
instance's OpenAPI document at `/api/{api_version}/server/swagger`.

#### Health and discovery

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET | `/api/{api_version}` | Public | API root; returns the same discovery envelope as `/api/autodiscover`. A trailing slash redirects to the canonical path. |
| GET | `/server/healthz` | Public | Content-negotiated health page/API response. |
| GET | `/healthz` | Public, when enabled | Root health alias. |
| GET | `/api/{api_version}/server/healthz` | Public | Versioned health response. |
| GET | `/api/healthz` | Public | Unversioned health alias. |
| GET | `/api/autodiscover` | Public | Client connection and server capability discovery. |

The shared health document reports `features.tor` and `features.i2p` with
`enabled`, `running`, `state`, `status`, and `hostname` fields; I2P also
reports its provider. `status` is always `disabled`, `starting`, `healthy`,
or `unavailable`, while `state` carries the operator lifecycle value.
`checks.dns_listener` reports whether the configured Do53 service is bound.
Unavailable overlays degrade health without taking down the clearnet server.

`/api/autodiscover` is unversioned. Both it and `/api/{api_version}` use the
standard `{"ok":true,"data":{...}}` JSON envelope and return the same discovery
document. The `data` object contains `api.current_version`, `name`,
`version`, public `urls`, CLI and agent version maps, and their minimum
versions. Onion and I2P addresses appear only when the corresponding
`server.tor.expose` or `server.i2p.expose` setting is enabled. The endpoint
does not publish configuration settings or a schema. It inventories
operator-provisioned assets from `{data_dir}/cli/` (or
`cli.binary_download.dir`) and publishes only files whose SHA-256 matches
`sha256.txt`. Missing files and checksum mismatches are omitted.

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET | `/api/{api_version}/cli/binaries/{project_name}-cli-{os}-{arch}[.exe]` | Public by default; Bearer token when `cli.binary_download.require_auth` is true | Download a verified CLI release binary. |
| GET | `/api/{api_version}/agent/binaries/{project_name}-agent-{os}-{arch}[.exe]` | Bearer token with `agent` scope | Download a verified agent release binary. |

#### Public server information

`/server` permanently redirects to `/server/about`. The server-rendered pages
are available at `/server/about`, `/server/help`, `/server/help/mcp`,
`/server/privacy`, `/server/terms`, and `/server/preferences`. The Terms page
uses the built-in United States default unless an operator override is set.
The MCP help page shows the
request-resolved endpoint, aliases, token setup, connection examples, live
tools, and the configured project guide. The privacy page displays the
effective deployment privacy settings. Its consent form posts to
`POST /server/consent`; when data sale is enabled, the privacy page provides
the `POST /server/ccpa` opt-out control. The preferences page includes consent
and CCPA toggles, consent-gated theme/language choices, and a guest-cookie reset
that clears browser storage. These browser forms require the double-submit
CSRF token. The sync page exports a canonical share URL and unpadded base64url
code, and imports links, query strings, or codes without storing preferences on
the server. Import is additive and writes cookies only when that browser has
accepted Preferences consent.

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET | `/api/{api_version}/server/about` | Public | Read the localized About source and format used by the public page. |
| GET | `/api/{api_version}/server/terms` | Public | Read localized Terms source and format; the built-in terms are returned when no override is configured. |
| GET | `/api/{api_version}/server/privacy` | Public | Read effective privacy, cookie, retention, sharing, and conditional CCPA data. |
| GET | `/api/{api_version}/server/help/mcp` | Public | Read MCP endpoint URL, aliases, Bearer scheme, live standard-token tools, and project guide. |
| GET | `/api/{api_version}/server/preferences` | Public | Read the current browser's effective guest theme, language, and application preferences. |
| GET | `/api/{api_version}/server/preferences/sync` | Public | Export guest preferences with a canonical share URL and short code. |
| GET | `/api/{api_version}/server/preferences/sync/import` | Public | Import guest preference cookies from direct query values or a shared `code`. |
| POST | `/api/{api_version}/server/preferences` | Public | Apply a partial guest preference update from JSON or form fields. |
| POST | `/api/{api_version}/server/mcp` | Bearer token required | MCP Streamable HTTP endpoint (JSON-RPC 2.0, protocol revision `2026-07-28`). The direct aliases are `/api/mcp` and `/mcp`. |

The matching server-rendered routes are `GET /server/preferences/sync`,
`POST /server/preferences/sync`, and `GET /server/preferences/sync/import`.
The sync page negotiates HTML, plain text, and JSON; web import and save
always redirect with `303` and no response body. API save returns
`400 VALIDATION_FAILED` only when none of the supplied values pass validation.
`theme`, `lang`, and `redxt_pref_*` values are portable; consent, CCPA, build,
session, CSRF, and other credential cookies are never exported or imported.

MCP help is also available as HTML at `GET /server/help/mcp`. Its project
expansion is configured at `server.pages.help.mcp` in `yaml.d/pages.yml`.

MCP accepts only `POST` and requires `Authorization: Bearer ...` on every
request. It has no session state or `initialize` handshake. Each request
supplies `MCP-Protocol-Version`, `Mcp-Method`, and the matching JSON-RPC
`_meta.io.modelcontextprotocol/protocolVersion`; `tools/call` also supplies
`Mcp-Name`. The caller's token scope filters the tools returned by
`tools/list` and accepted by `tools/call`. Browser `Origin` values must match
the request's own public origin or an explicit CORS origin; a wildcard CORS
default does not authorize MCP origins.

The privacy response is derived from `yaml.d/privacy.yml` (the effective
`server.privacy` configuration). The `ccpa` object is present only when
`data.sold` is true and reports the current request's `ccpa_opt_out` cookie.
The `content.data_usage` field switches between the built-in sold and not-sold
Markdown text.

#### Authentication

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| POST | `/api/{api_version}/server/auth/register` | Public | Register when registration is enabled. |
| POST | `/api/{api_version}/server/auth/invite/user/{code}` | Public | Register from an invitation. |
| POST | `/api/{api_version}/server/auth/login` | Public | Create a session or begin a second-factor challenge. |
| POST | `/api/{api_version}/server/auth/2fa` | Public challenge | Complete a pending second-factor challenge. |
| POST | `/api/{api_version}/server/auth/logout` | Session | End the current session. |
| POST | `/api/{api_version}/server/auth/password/forgot` | Public | Request a password reset. |
| POST | `/api/{api_version}/server/auth/password/reset` | Public reset token | Set a password using a reset token. |
| POST | `/api/{api_version}/server/auth/verify/{code}` | Public | Verify an email address. |

Account lockout responses use `403 ACCOUNT_LOCKED`. Temporary locks set the
remaining wait in the HTTP `Retry-After` header; the canonical error body does
not include retry timing. Permanent locks omit that header. The browser
sign-in form shows the remaining wait time or directs the user to reset the
password or contact an administrator for a permanent lock.

#### Server admin

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET, PATCH | `/api/{api_version}/server/{admin_path}/ssl` | Bearer operator token or stored Server Admin token | Read or update SSL/TLS and ACME settings. DNS credential values are write-only. |
| POST | `/api/{api_version}/server/{admin_path}/ssl/renew` | Global-scope Bearer operator or Server Admin token | Force renewal of managed ACME certificates. |
| GET | `/api/{api_version}/server/{admin_path}/{admin_username}/profile` | Bearer token owned by that Server Admin | Read the caller's username, account email, and OpenPGP public key; other admins are not visible. |
| PATCH | `/api/{api_version}/server/{admin_path}/{admin_username}/profile` | Non-read-only Bearer token owned by that Server Admin | Update the caller's username or set/clear its validated OpenPGP public key. Username changes use the admin reserved-name and route-collision checks. |
| POST | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/password` | Non-read-only Bearer token owned by that Server Admin | Change the caller's password after verifying `current_password`; `new_password` follows `server.security.password`. Existing browser sessions are revoked, the CSRF secret rotates, and the action is audited. |
| GET | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/token` | Bearer token owned by that Server Admin | Read the default token's prefix and mask. |
| POST | `/api/{api_version}/server/{admin_path}/{admin_username}/profile/token` | Global-scope Bearer token owned by that Server Admin | Replace the default token and return the new `adm_` value once. |
| GET | `/api/{api_version}/server/{admin_path}/security/tokens` | Bearer operator or stored Server Admin token | List all token metadata, including whether an active token has reached the configured rotation reminder age; secrets and hashes are never returned. |
| DELETE | `/api/{api_version}/server/{admin_path}/security/tokens/{id}` | Global-scope Bearer operator or Server Admin token | Revoke a token while retaining its audit history. |
| POST | `/api/{api_version}/server/{admin_path}/security/tokens/{id}/rotate` | Global-scope Bearer operator or Server Admin token | Replace the token secret and return it once; its scope and expiry are preserved. |
| GET | `/api/{api_version}/server/{admin_path}/security/locked-accounts` | Bearer operator or Server Admin token | List accounts with an active temporary or permanent lock, including account kind, numeric ID, expiry, and permanent-lock state. |
| DELETE | `/api/{api_version}/server/{admin_path}/security/locked-accounts/{kind}/{id}` | Global-scope Bearer operator or Server Admin token | Clear a user's or admin's lockout state and failure history; `kind` is `user` or `admin`. |
| GET | `/api/{api_version}/server/{admin_path}/pages` | Bearer operator or Server Admin token | Read configured page content, contact-form controls, and the configured MCP help section. |
| GET, PATCH | `/api/{api_version}/server/{admin_path}/pages/{page}` | Bearer operator or Server Admin token; PATCH requires global scope | Read or update About, Privacy, Terms, Contact, or Help settings. About, Privacy, and Terms accept `{ "format": "md|html|txt", "body": "..." }` or a locale-to-body object. Contact PATCH accepts enabled/captcha/site-key/secret/success-message fields; secret values are never returned. Help edits the MCP project guide. |
| DELETE | `/api/{api_version}/server/{admin_path}/pages/{page}` | Global-scope Bearer operator or Server Admin token | Remove an About, Privacy, or Terms override, or reset Help to its default project guide. |
| POST | `/api/{api_version}/server/{admin_path}/pages/{page}/preview` | Bearer operator or Server Admin token | Sanitize and render submitted page content without saving it; accepts `format` and `body`, returns rendered `html`. |
| GET, POST | `/api/{api_version}/server/{admin_path}/agents` | Bearer operator or stored Server Admin token | List admin-scoped agents or create a registration token. POST accepts `name` and optional `tags`; the new `adm_` token is shown once and expires after 24 hours until registration. |
| POST | `/api/{api_version}/server/{admin_path}/agents/register` | Admin agent token (`adm_`, scope `agent`) | Register the pending agent once with hostname, OS, architecture, version, tags, and optional display name. |
| POST | `/api/{api_version}/server/{admin_path}/agents/heartbeat` | Registered admin agent token | Update liveness, IP address, uptime, version, and update availability. |
| POST | `/api/{api_version}/server/{admin_path}/agents/report` | Registered admin agent token | Store up to 100 oldest-first metric samples. |
| GET, PATCH, DELETE | `/api/{api_version}/server/{admin_path}/agents/{name}` | Bearer operator or stored Server Admin token | Read details, update the name/tags, or remove the agent and its metric history. |
| POST | `/api/{api_version}/server/{admin_path}/agents/{name}/token` | Bearer operator or stored Server Admin token | Regenerate the agent credential and return it once with `Cache-Control: no-store`. |
| GET | `/api/{api_version}/server/{admin_path}/agents/{name}/metrics` | Bearer operator or stored Server Admin token | Read up to 1,000 most recent metric samples (`limit` defaults to 100). |
| GET, POST | `/api/{api_version}/server/{admin_path}/backup` | Bearer Server Admin token for GET; global-scope token for POST | List archive metadata or create and verify a manual backup. |
| POST | `/api/{api_version}/server/{admin_path}/backup/restore` | Global-scope Bearer operator or Server Admin token | Restore a named archive after verification; accepts `backup_file` and, when encrypted, `password`, then gracefully exits for service-manager restart. |
| GET, DELETE | `/api/{api_version}/server/{admin_path}/backup/{id}` | Bearer Server Admin token for GET; global-scope token for DELETE | Read archive metadata or delete an archive; deletion of a retained incremental base returns a conflict. |
| GET | `/api/{api_version}/server/{admin_path}/backup/{id}/download` | Bearer Server Admin token | Download the archive with private no-store caching. |

The operator token can call SSL routes but is not tied to a personal admin
account, so it cannot read or regenerate a profile token. Regeneration makes
the previous default token unusable. Token list and rotate responses omit
stored hashes; rotate responses use `Cache-Control: no-store`.
Backup creation accepts a JSON object with optional `filename`, `password`,
`include_ssl`, and `include_data` fields. An omitted password uses the
configured backup encryption password. Responses expose archive metadata only,
never an on-disk path or checksum. Restore accepts `{"backup_file":"...",
"password":"..."}`; an encrypted archive without a password returns
`VALIDATION_FAILED`. `POST /api/{api_version}/server/{admin_path}/backup/test`
accepts an optional `backup_file` and `password`. With no file it verifies the
newest full archive plus available daily/hourly incrementals and their bases;
with a file it verifies that archive and its required base. The response lists
the result and failed check for each tested archive without exposing paths.

Agent management currently covers CRUD, one-time credential creation,
registration, heartbeat, metrics reporting, and metric-history queries for the
admin scope. User- and organization-scoped agent routes are not available yet.

#### Users and credentials

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET, PATCH | `/api/{api_version}/users` | Bearer token or session | Read or update the current profile. |
| GET, PATCH | `/api/{api_version}/users/settings` | Bearer token or session | Read or update account preferences. |
| GET | `/api/{api_version}/users/security` | Bearer token or session | Read security settings. |
| POST | `/api/{api_version}/users/security/password` | Session | Change the password. |
| POST | `/api/{api_version}/users/security/email` | Session | Change the email address. |
| POST | `/api/{api_version}/users/security/2fa` | Session | Start enrollment after re-entering `current_password`; returns the one-time TOTP seed and provisioning URI. |
| POST | `/api/{api_version}/users/security/2fa/confirm` | Session | Verify the authenticator code; returns ten recovery keys once and leaves 2FA pending. |
| POST | `/api/{api_version}/users/security/recovery-keys/ack` | Session | Acknowledge that recovery keys were saved and activate the pending second factor. |
| POST | `/api/{api_version}/users/security/2fa/disable` | Session | Disable two-factor authentication after password confirmation. |
| GET | `/api/{api_version}/users/sessions` | Bearer token or session | List the current user's sessions. |
| DELETE | `/api/{api_version}/users/sessions/{session_id}` | Bearer token or session | Revoke a session. |
| GET, POST | `/api/{api_version}/users/tokens` | Bearer token or session for GET; session for POST | List or issue API tokens. |
| GET | `/api/{api_version}/users/tokens/{token_id}` | Bearer token or session | Read token metadata without returning its secret or hash. |
| POST | `/api/{api_version}/users/tokens/{token_id}/rotate` | Session | Replace the token secret, preserve its settings, and return the new secret once with `Cache-Control: no-store`. |
| DELETE | `/api/{api_version}/users/tokens/{token_id}` | Session | Revoke an API token. |
| POST | `/api/{api_version}/users/invites/{code}` | Session | Accept an organization invitation. |
| GET | `/api/{api_version}/users/{username}` | Public | Read a public user profile. |

User tokens default to `global` scope within the issuer's role permissions and
do not expire unless an expiry is selected or configured. `read` tokens can
call read operations; `read-write` tokens can read and write but cannot delete;
`global` uses all operations the issuer's role permits. Token creation accepts
`scope` and `expires_in_days`; the HTML form also offers common expiry choices
and a custom day count. `agent` scope is reserved for the agent registration
flow.

#### Organizations and custom domains

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET, POST | `/api/{api_version}/orgs` | Bearer token for GET; session for POST | List or create organizations. |
| GET, PATCH, DELETE | `/api/{api_version}/orgs/{slug}` | Organization membership; elevated role for changes | Read, update, or delete an organization. |
| GET, PATCH | `/api/{api_version}/orgs/{slug}/settings` | Organization membership; elevated role for changes | Read or update organization settings. |
| POST | `/api/{api_version}/orgs/{slug}/transfer` | Organization owner | Transfer ownership. |
| GET | `/api/{api_version}/orgs/{slug}/members` | Organization member | List members. |
| PATCH, DELETE | `/api/{api_version}/orgs/{slug}/members/{member_id}` | Organization admin or owner | Change a member role or remove a member. |
| GET, POST | `/api/{api_version}/orgs/{slug}/invites` | Organization member for GET; elevated role for POST | List or create invitations. |
| DELETE | `/api/{api_version}/orgs/{slug}/invites/{invite_id}` | Organization admin or owner | Revoke an invitation. |
| GET | `/api/{api_version}/orgs/{slug}/audit` | Organization member | Read the organization audit history. |
| GET | `/api/{api_version}/orgs/{slug}/tokens` | Organization member | List organization token metadata. |
| DELETE | `/api/{api_version}/orgs/{slug}/tokens/{token_id}` | Organization admin or owner | Revoke an organization token. |
| GET, POST | `/api/{api_version}/orgs/{slug}/zones` | Organization member for GET; admin or owner for POST | List or create DNS zones. Creation returns a pending zone and a one-time `_verify.<zone>` TXT challenge. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}` | Organization member | Read a DNS zone. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec` | Organization member | Read DNSSEC zone settings, public signing-key metadata, and SHA-256 DS records. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec/keys` | Organization member | List public signing-key metadata. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec/keys` | Organization admin or owner | Generate an encrypted-at-rest ECDSA P-256, ECDSA P-384, or Ed25519 signing key for an active DNSSEC-enabled zone. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/export` | Organization member | Download the zone and records as BIND (default), JSON, or CSV. |
| POST | `/api/{api_version}/orgs/{slug}/zones/import` | Organization admin or owner | Import a BIND master file; returns a pending zone, one-time verification challenge, and record count. |
| PATCH, DELETE | `/api/{api_version}/orgs/{slug}/zones/{zone_id}` | Organization admin or owner | Update or delete a DNS zone. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/verification` | Organization admin or owner | Verify the published TXT challenge and activate the zone. |
| GET, POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records` | Organization member for GET; editor, admin, or owner for POST | List or create validated DNS records. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records/{record_id}` | Organization member | Read a DNS record. |
| PATCH | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records/{record_id}` | Organization editor, admin, or owner | Update a DNS record. |
| DELETE | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records/{record_id}` | Organization admin or owner | Delete a DNS record. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/grants` | Organization admin or owner | Grant a member access to a zone. |
| DELETE | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/grants/{member_id}` | Organization admin or owner | Revoke a member's zone grant. |

DNSSEC key management currently stores encrypted private signing keys and
returns public DNSKEY metadata. It does not yet sign authoritative DNS
responses, publish DNSKEY/DS records, or automate rollovers; those operations
must not be treated as complete DNSSEC service.
| GET, POST | `/api/{api_version}/orgs/{slug}/domains` | Organization member for GET; elevated role for POST | List or add custom domains. |
| PATCH, DELETE | `/api/{api_version}/orgs/{slug}/domains/{domain_id}` | Organization admin or owner | Update or remove a custom domain. |
| GET | `/api/{api_version}/orgs/{slug}/domains/{domain_id}/verification` | Organization member | Read domain verification instructions. |
| POST | `/api/{api_version}/orgs/{slug}/domains/{domain_id}/verify` | Organization admin or owner | Verify a custom domain. |
| POST | `/api/{api_version}/orgs/{slug}/domains/{domain_id}/suspend` | Organization admin or owner | Suspend a custom domain. |

The API currently exposes user, organization, and custom-domain management;
Zone names are unique across organizations and remain pending until the owner
publishes the TXT value returned by zone creation at `_verify.<zone>`. The
server-rendered organization page also provides zone creation, record entry,
ownership verification forms, and BIND, JSON, and CSV downloads. After creating
a zone it redirects to the zone page, where the pending TXT instructions
remain available to authorized organization administrators. The recoverable
challenge is encrypted at rest and cleared after verification. Record owners
and RDATA are validated before storage, and every zone and record lookup is
organization-scoped.

Zone import accepts `application/json` with a zone name and a BIND
master-file string, up to 10 MiB:

```json
{
  "name": "example.test",
  "master_file": "$ORIGIN example.test.\n$TTL 3600\n@ IN SOA ns1.example.test. hostmaster.example.test. 1 86400 7200 3600000 3600\n@ IN NS ns1.example.test.\nns1 IN A 192.0.2.53\n"
}
```

The import requires one apex SOA, accepts only IN records within the named
zone, and rejects `$INCLUDE`. The zone and all records are stored atomically
and remain pending until the TXT ownership challenge is verified. The success
envelope's `data` contains `zone`, `verification` (`name`, `type`, `value`),
and `record_count`. The challenge value is returned once and encrypted at
rest. Imported records are normalized to DNS presentation data; comments and
source formatting are not retained.

Versioned REST endpoints default to JSON. `Accept: application/json` takes
priority over client detection; `Accept: text/plain`, detected HTTP tools, and
the `.txt` suffix select the API's plain-text serializer. A declared binary
route keeps its bytes and media type for every `Accept` value, and its `.txt`
form is `404`. Health, GraphQL, Swagger, autodiscovery, and MCP retain their
endpoint-specific response contracts; the remaining API negotiation coverage
is tracked in `TODO.AI.md`.

## DNS-over-HTTPS

The native DoH endpoint is served at `/dns-query` over HTTPS and uses the
same authoritative answer engine as the UDP/TCP DNS listener. Separate DoT
and DoQ listeners use that engine when enabled and a usable TLS certificate
is available. A trusted
reverse proxy may terminate TLS and report the effective scheme through the
trusted-proxy configuration.

- `GET /dns-query?dns={base64url-message}` — the `dns` value is unpadded
  base64url-encoded DNS wire data.
- `POST /dns-query` — send DNS wire data with
  `Content-Type: application/dns-message`.

Successful DNS exchanges return `application/dns-message`; a DNS-level error
such as REFUSED is carried in the DNS response rather than converted to an
HTTP error. Recursive resolution, DNSCrypt, and encrypted upstream transports
are not implemented yet.

## Swagger UI

- UI: `/server/docs/swagger`
- JSON: `/api/v1/server/swagger` (alias: `/api/swagger`)

## GraphQL

- GraphiQL UI: `/server/docs/graphql`
- Endpoint: `POST /api/v1/server/graphql` (alias: `POST /api/graphql`)

### Schema

The GraphQL schema mirrors the REST surface one-for-one; any endpoint
reachable over REST is reachable through the same operation in
GraphQL, and vice versa.
