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

## Security Reporting

Report vulnerabilities to the maintainer email listed in `IDEA.md`
(`git-admin@casjaysdev.pro`) rather than filing a public issue.

## Transport Security

- Let's Encrypt HTTP-01 (port 80) and TLS-ALPN-01 (port 443) issuance
  is built in and auto-enabled — no external ACME client required
- A Tor hidden service is auto-enabled whenever a Tor binary is
  detected on the host; the hidden service is app-scoped only and never
  performs relay, exit, or proxy functions
- I2P eepsite support is optional and strictly opt-in via
  `features.i2p.enabled` — it is never auto-enabled

## Well-Known Namespace

Standard `/.well-known/` discovery documents are part of the PART 32/34
integration surface; consult `TODO.AI.md` for current implementation
status before relying on a specific well-known path in production.
