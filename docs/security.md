# Security

## Authentication & Identity

- Passwords are hashed with **Argon2id** (never bcrypt); API tokens and
  other secrets are hashed with **SHA-256** — raw tokens are never
  logged or stored in plaintext
- Local username/password with TOTP is the currently implemented
  authentication method; external identity providers (OIDC, LDAP, SAML,
  passkey/WebAuthn) are tracked in `TODO.AI.md`
- Server Admins and Regular Users are kept in separate database tables
  and are never merged
- Credentials (keys, tokens, passwords, secrets) are redacted from logs
  and error output in every mode, including debug

## Public Security Endpoints

- `GET /server/healthz` (and root alias `GET /healthz`) — unauthenticated
  health check, safe to expose to uptime monitors and load balancers
- `GET /api/v1/server/healthz` and `GET /api/healthz` — public health
  response for API clients and probes
- `GET /api/autodiscover` — public server discovery document

Health responses include a fixed Tor and I2P state/status contract and a
Do53 listener check. Overlay status is one of `disabled`, `starting`,
`healthy`, or `unavailable`; diagnostic errors and filesystem paths stay
out of the public response.

## Security Reporting

Report vulnerabilities to the maintainer email listed in `IDEA.md`
(`git-admin@casjaysdev.pro`) rather than filing a public issue. The
server-rendered security-reporting page and `/.well-known/security.txt` are
not implemented yet; the email address is the available reporting channel.

## Transport Security

- Let's Encrypt HTTP-01 (port 80) and TLS-ALPN-01 (port 443) issuance
  is built in and auto-enabled — no external ACME client required
- A Tor hidden service is auto-enabled whenever a Tor binary is
  detected on the host; the hidden service is app-scoped only and never
  performs relay, exit, or proxy functions
- I2P eepsite support is optional: it defaults off on bare metal and VMs,
  and on in application containers. Set `server.i2p.enabled` or
  `I2P_ENABLED` to override the resolved default

## Well-Known Namespace

No `/.well-known/` documents are currently mounted. Planned discovery and
security-reporting endpoints are tracked in `TODO.AI.md`; do not rely on
those paths until the corresponding routes are implemented.
