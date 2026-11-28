# Integrations

## Identity providers

The server currently authenticates Server Admins and Regular Users with local
accounts. Passwords use Argon2id; optional TOTP second-factor authentication
is supported. External OIDC, LDAP, SAML, and passkey providers are not
available in this release.

## DNS protocols

- Authoritative DNS is available over UDP and TCP on the configured DNS
  listener.
- DNS-over-HTTPS uses `/dns-query` over HTTPS and accepts RFC 8484 GET and
  POST requests.
- DNS-over-TLS and DNS-over-QUIC use separate configured listeners and
  require a usable server certificate.
- BIND master files can be imported for zone provisioning. Zones can be
  exported as BIND master files, JSON, or CSV.

Recursive resolution, DNSCrypt, and encrypted upstream transports are not
available. See [DNS API details](api.md#dns-over-https) and the
[configuration reference](configuration.md) for listener settings.

## Client and agent discovery

`GET /api/autodiscover` and `GET /api/v1/` return the same public discovery
document. It describes the API version, public origins, and minimum supported
client and agent versions. Overlay addresses are included only when their
exposure settings are enabled. The document does not expose configuration or
an API schema.

The server provides a Streamable HTTP MCP endpoint at `/api/v1/server/mcp`,
with `/api/mcp` and `/mcp` aliases. It uses bearer-token authentication and
filters tools by token scope. The public guide at `/server/help/mcp` lists the
active endpoint, connection examples, and available tools.

Managed agents register with a one-use token created by a Server Admin and
report heartbeats and metrics. This is separate from clustering between redxt
server instances; cluster membership and synchronization are not available.
See the [Admin Panel guide](admin.md#available-pages) for agent management.

## Other integrations

- Tor hidden-service support activates when Tor is detected. Address
  publication is controlled by the Tor exposure setting.
- I2P support is controlled by the resolved I2P configuration; address
  publication is separately opt-in.
- The Terraform provider is not currently distributed.
- No webhook, federation, or external identity-provider integration is
  enabled by this release.

Neither Tor nor I2P enables relay, exit, or proxy traffic through redxt.
