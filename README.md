# redxt

[![CI](https://github.com/webappsgo/redxt/actions/workflows/ci.yml/badge.svg)](https://github.com/webappsgo/redxt/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/webappsgo/redxt)](LICENSE.md)
[![Documentation Status](https://readthedocs.org/projects/webappsgo-redxt/badge/?version=latest)](https://webappsgo-redxt.readthedocs.io/en/latest/?badge=latest)

redxt is a self-hosted DNS platform. The server provides authoritative DNS
over UDP/TCP and DNS-over-HTTPS, with DNS-over-TLS and DNS-over-QUIC
listeners when certificates are configured. It includes server-rendered
setup, account, organization, DNS zone, and administration pages. The
repository also provides the `redxt-cli` client and `redxt-agent` companion
for managed nodes.

On first run, redxt chooses and persists an available high HTTP port, prints
a one-time setup token, and creates an operator API credential. Use the
setup token in the setup wizard to create the Primary Admin account. The
operator API credential is separate and is intended for programmatic
administration.

## Features

- Authoritative DNS over UDP and TCP, plus RFC 8484 DNS-over-HTTPS
- DNS-over-TLS and DNS-over-QUIC listeners with configured certificates
- Server-rendered public, account, organization, setup, and administration
  pages, with forms that work without JavaScript
- Organization zone and record management, domain verification, and API
  tokens
- Backup administration and installable PWA support
- Companion `redxt-cli` and `redxt-agent` binaries

See the [documentation](https://webappsgo-redxt.readthedocs.io/) for
installation, configuration, API, client, server pages, security, and
operations.

## Quick Start

Build binaries with the project Docker toolchain or use the Docker image.
See the [installation guide](docs/installation.md) for Docker, binary, and
service setup instructions. Read the
[configuration reference](docs/configuration.md) before exposing an
instance to a network.

## Languages

The server, CLI, and agent include English, Spanish, Chinese, French,
Arabic, German, Italian, and Japanese translations. Arabic uses a
right-to-left interface. See the [translation guide](docs/translating.md) to
contribute.

## API

The versioned REST API is rooted at `/api/v1/`. Public health and discovery
routes, DNS endpoints, and the administration API are documented in the
[API reference](docs/api.md). The browser interface and its public pages are
described in the [web app guide](docs/web-app.md).

## Development

Builds and tests run in the project Docker toolchain. Common commands are:

```bash
make test
make build
make docker
```

See the [development guide](docs/development.md) for prerequisites and
verification tiers.

## Documentation

- [Installation](docs/installation.md)
- [Configuration](docs/configuration.md)
- [API reference](docs/api.md)
- [CLI reference](docs/cli.md)
- [Backup and restore](docs/backup.md)
- [Admin panel](docs/admin.md)
- [Web app and offline behavior](docs/web-app.md)
- [Security](docs/security.md)
- [Integrations](docs/integrations.md)
- [Development](docs/development.md)
- [Translating](docs/translating.md)

## License

MIT — see [LICENSE.md](LICENSE.md).
