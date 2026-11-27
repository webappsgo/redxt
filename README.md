# redxt

[![CI](https://github.com/webappsgo/redxt/actions/workflows/ci.yml/badge.svg)](https://github.com/webappsgo/redxt/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/webappsgo/redxt)](LICENSE.md)
[![Documentation Status](https://readthedocs.org/projects/webappsgo-redxt/badge/?version=latest)](https://webappsgo-redxt.readthedocs.io/en/latest/?badge=latest)

## About

redxt is an in-development DNS platform. The current server provides
authoritative answers for configured primary and secondary zones over DNS
UDP/TCP and DNS-over-HTTPS, with optional DoT/DoQ listeners when a usable
certificate is available. The broader product specification includes
recursive resolution, forwarding, DNS firewall, dynamic DNS, redirects,
data zones, clustering, and managed agents; those capabilities are not yet
complete. See [`TODO.AI.md`](TODO.AI.md) for the implementation status and
[`IDEA.md`](IDEA.md) for the product requirements.

## Official Site

[redxt.us](https://redxt.us)

## Features

- Authoritative DNS over UDP and TCP, with configurable listener address
  and port
- DNS-over-HTTPS at `/dns-query` on HTTPS, using RFC 8484 GET and POST
- Configurable DNS-over-TLS and DNS-over-QUIC listeners using the shared
  certificate manager
- Organization-scoped DNSSEC key generation with encrypted private-key
  storage and public DS calculation; DNS response signing and validation
  remain in development
- Server-rendered setup and administration pages
- Public landing page at `/`, plus about, help, privacy, and preferences pages with no-JavaScript
  cookie consent, CCPA opt-out controls, and Global Privacy Control handling
- Public versioned privacy API that reports the effective privacy settings
- Health endpoints for monitoring the current server
- Companion `redxt-cli` and `redxt-agent` binaries are part of the required
  project deliverables; see the backlog for incomplete behavior

The project is not yet a replacement for a production DNS stack. Review the
current feature status before deploying it.

## Production

Production-ready releases are not yet available. For development deployment
instructions, see [`docs/installation.md`](docs/installation.md). The
production Compose configuration is in `docker/docker-compose.yml`.

## Client

The repository includes the `redxt-cli` companion client and `redxt-agent`.
Their current commands and flags are documented in [`docs/cli.md`](docs/cli.md);
the full operator and managed-node workflows remain in development.

## Configuration

redxt reads `server.yml`, environment variables, and CLI flags. See
[`docs/configuration.md`](docs/configuration.md) for current settings.

## API

Current server endpoints include:

- Health: `GET https://redxt.us/server/healthz`
- API discovery: `GET https://redxt.us/api/v1` (or `/api/autodiscover`)
- Versioned health: `GET https://redxt.us/api/v1/server/healthz`
- Privacy details: `GET https://redxt.us/api/v1/server/privacy`
- Public pages: `/`, `/server/about`, `/server/help`, `/server/privacy`, and
  `/server/preferences`
- DNS-over-HTTPS: `GET https://redxt.us/dns-query?dns={base64url-message}`
  or `POST https://redxt.us/dns-query` with `Content-Type: application/dns-message`
- DNS-over-TLS and DNS-over-QUIC: dedicated listeners on configured DNS
  addresses when a usable certificate is available

These URLs represent the configured official hostname. The same paths are
available on a self-hosted instance. See [`docs/api.md`](docs/api.md) for the
implemented API surface.

## Other

- [Installation](docs/installation.md)
- [API reference](docs/api.md)
- [Security](docs/security.md)
- [Integrations](docs/integrations.md)
- [Implementation backlog](TODO.AI.md)

## Development

Go builds and tests run in the project’s Docker toolchain. Common commands:

```bash
make test
make build
make docker
```

See [`docs/development.md`](docs/development.md) for the development workflow.

## Disclaimer

This software is provided "as is" without warranty of any kind. Use it only
after evaluating whether it is suitable for your environment.

- **No Warranty**: The authors are not responsible for damages, data loss,
  or other issues arising from use of this software.
- **Not Professional Advice**: This software does not provide legal,
  financial, medical, or other professional advice.
- **Third-Party Services**: External services used with redxt are subject to
  their own terms.
- **Security**: No software can be guaranteed free of vulnerabilities.
- **Production Use**: The project is under active development; evaluate it
  thoroughly before production use.

By using this software, you acknowledge that you have read and understood
this disclaimer.

## License

MIT — see [`LICENSE.md`](LICENSE.md).
