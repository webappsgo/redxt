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

`/api/autodiscover` is unversioned but uses the standard `{"ok":true,"data":{...}}`
JSON envelope. The `data` object contains `api.current_version`, `name`,
`version`, public `urls`, CLI and agent version maps, and their minimum
versions. Onion and I2P addresses appear only when the corresponding
`server.tor.expose` or `server.i2p.expose` setting is enabled. The endpoint
does not publish configuration settings or a schema. Release asset inventory
and binary download routes are not implemented yet, so both version maps are
currently empty.

#### Public server information

The server-rendered pages are available at `/server/about`, `/server/help`,
`/server/privacy`, and `/server/preferences`. The privacy page displays the
effective deployment privacy settings. Its consent form posts to
`POST /server/consent`; when data sale is enabled, the page and preferences
page provide a `POST /server/ccpa` opt-out control. These browser forms require
the double-submit CSRF token.

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET | `/api/{api_version}/server/privacy` | Public | Read effective privacy, cookie, retention, sharing, and conditional CCPA data. |

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

#### Users and credentials

| Method | Path | Authentication | Purpose |
|---|---|---|---|
| GET, PATCH | `/api/{api_version}/users` | Bearer token or session | Read or update the current profile. |
| GET, PATCH | `/api/{api_version}/users/settings` | Bearer token or session | Read or update account preferences. |
| GET | `/api/{api_version}/users/security` | Bearer token or session | Read security settings. |
| POST | `/api/{api_version}/users/security/password` | Session | Change the password. |
| POST | `/api/{api_version}/users/security/email` | Session | Change the email address. |
| POST | `/api/{api_version}/users/security/2fa` | Session | Start two-factor enrollment. |
| POST | `/api/{api_version}/users/security/2fa/confirm` | Session | Confirm two-factor enrollment. |
| DELETE | `/api/{api_version}/users/security/2fa` | Session | Disable two-factor authentication. |
| GET | `/api/{api_version}/users/sessions` | Bearer token or session | List the current user's sessions. |
| DELETE | `/api/{api_version}/users/sessions/{session_id}` | Bearer token or session | Revoke a session. |
| GET, POST | `/api/{api_version}/users/tokens` | Bearer token or session for GET; session for POST | List or issue API tokens. |
| DELETE | `/api/{api_version}/users/tokens/{token_id}` | Session | Revoke an API token. |
| POST | `/api/{api_version}/users/invites/{code}` | Session | Accept an organization invitation. |
| GET | `/api/{api_version}/users/{username}` | Public | Read a public user profile. |

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

Content negotiation is honored on every route (`Accept:
application/json`, `text/plain`, `text/html` where applicable).

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
