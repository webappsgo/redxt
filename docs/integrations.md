# Integrations

## External Identity

External identity providers (OIDC, LDAP, SAML, passkey/WebAuthn) are
specified but not yet implemented — see `TODO.AI.md`. Local Argon2id
credentials with TOTP are the only supported sign-in method today.

## Discovery & Protocol Endpoints

- REST: `/api/v1/...`
- Swagger UI: `/server/docs/swagger`, JSON at `/api/v1/server/swagger`
  (alias `/api/swagger`)
- GraphiQL: `/server/docs/graphql`, endpoint at
  `POST /api/v1/server/graphql` (alias `/api/graphql`)
- Health: `/server/healthz` (root alias `/healthz`,
  versioned `/api/v1/server/healthz`)

## Platform Integrations

- **Terraform provider** — a first-class deliverable per `IDEA.md`;
  tracked separately from this in-process API surface
- **BIND interchange** — `named.conf` and master zone files are the
  canonical migration format; a working named deployment is expected to
  migrate without hand-editing
- **Cluster nodes** — other redxt instances joined for horizontal
  scaling, distinct from *managed nodes* (external DNS/redirect targets
  redxt monitors but does not run)

## Operator Notes

Overlay network integrations (Tor, I2P) are configuration-only from an
operator's perspective: Tor activates automatically when a Tor binary
is present on the host, while I2P requires explicitly setting
`features.i2p.enabled: true`. Neither overlay allows relay, exit, or
proxy traffic through the redxt process.
