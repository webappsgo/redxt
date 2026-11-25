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

- **Terraform provider** — required by `IDEA.md`, but not implemented or
  published yet.
- **BIND interchange** — BIND `named.conf` and master-zone-file support is
  a product requirement; import/export is not implemented yet.
- **Cluster nodes** — horizontal clustering between redxt instances is a
  product requirement, but cluster membership and synchronization are not
  implemented yet. Managed nodes (external DNS/redirect targets) are a
  separate product concept and are not currently managed by the server.

The current management API covers user, organization, and custom-domain
operations. DNS zone/record control-plane APIs and provider-compatible DDNS
routes are not available yet; see [API Reference](api.md) for the implemented
surface.

## Operator Notes

Overlay network integrations (Tor, I2P) are managed by the server:
Tor activates automatically when a Tor binary is available. I2P defaults
off on bare metal and VMs and on in application containers; override its
default with `server.i2p.enabled` or `I2P_ENABLED`. Neither overlay allows
relay, exit, or proxy traffic through the redxt process.
