# Integrations

## External Identity

External identity providers (OIDC, LDAP, SAML, passkey/WebAuthn) are
specified but not yet implemented — see `TODO.AI.md`. Local Argon2id
credentials with TOTP are the only supported sign-in method today.

## Discovery & Protocol Endpoints

- REST: `/api/v1/...`
- DNS-over-HTTPS: `/dns-query` over HTTPS; DNS-over-TLS and DNS-over-QUIC
  use dedicated encrypted DNS listeners when a certificate is available.
- Swagger UI: `/server/docs/swagger`, JSON at `/api/v1/server/swagger`
  (alias `/api/swagger`)
- GraphiQL: `/server/docs/graphql`, endpoint at
  `POST /api/v1/server/graphql` (alias `/api/graphql`)
- Health: `/server/healthz` (root alias `/healthz`,
  versioned `/api/v1/server/healthz`)

## Platform Integrations

- **Terraform provider** — required by `IDEA.md`, but not implemented or
  published yet.
- **BIND interchange** — zone import accepts BIND master files, and zone
  export is available in BIND master format, JSON, and CSV. `named.conf`
  migration remains unimplemented.
- **Cluster nodes** — horizontal clustering between redxt instances is a
  product requirement, but cluster membership and synchronization are not
  implemented yet. Managed nodes (external DNS/redirect targets) are a
  separate product concept and are not currently managed by the server.

The current management API covers user, organization, custom-domain, and DNS
zone/record operations, including ownership verification, BIND master-file
import, and zone export. Provider-compatible DDNS routes and the other DNS
control-plane surfaces remain unimplemented; see [API Reference](api.md) for
available routes.

## Operator Notes

Overlay network integrations (Tor, I2P) are managed by the server:
Tor activates automatically when a Tor binary is available. I2P defaults
off on bare metal and VMs and on in application containers; override its
default with `server.i2p.enabled` or `I2P_ENABLED`. Neither overlay allows
relay, exit, or proxy traffic through the redxt process.
