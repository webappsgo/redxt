# redxt

**redxt** is a self-hosted DNS platform. Its server provides authoritative
answers for configured primary and secondary zones over DNS UDP/TCP and
DNS-over-HTTPS, with optional DNS-over-TLS and DNS-over-QUIC listeners when a
usable certificate is available. The server also provides a web interface for
setup, administration, regular-user accounts, organizations, DNS zones and
records, custom-domain verification, and API tokens. The repository also
ships the companion `redxt-cli` and `redxt-agent` binaries; their available
commands and configuration are documented in the [CLI reference](cli.md).

## Quick Start

```bash
# Docker
docker run --name "redxt-$(od -An -N4 -tx1 /dev/urandom | tr -d '[:space:]')" -p 172.17.0.1:64580:80 -p 53:53/tcp -p 53:53/udp ghcr.io/webappsgo/redxt:latest

# Local development binary
./redxt-linux-amd64 --config server.yml
```

On first run redxt selects a random port in the 64000-64999 range
(persisted to `server.yml`), prints a one-time setup token to the
console, creates the operator API credential in `secrets.yml` and prints it
once, then serves the setup wizard until the Primary Admin account is
created. These are separate credentials: the setup token completes the
wizard, while the operator token is the built-in `adm_` API credential.

## Current Features

- Authoritative DNS over UDP and TCP
- RFC 8484 DNS-over-HTTPS at `/dns-query` on HTTPS
- DNS-over-TLS and DNS-over-QUIC on configured listeners when a certificate
  is available
- Server-rendered public, account, organization, setup, and administration
  pages, with forms that work without JavaScript
- Public landing page at `/` with configured project content
- Organization zone and DNS record management, including domain verification
- Custom-domain verification, API token management, and backup administration
- Installable PWA with offline handling for eligible public pages
- Companion `redxt-cli` and `redxt-agent` binaries

See the guides below for the current browser, administration, configuration,
and API behavior.

## Documentation

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
- [CLI Reference](cli.md)
- [Backup and Restore](backup.md)
- [Admin Panel](admin.md)
- [Web app and offline behavior](web-app.md)
- [Security](security.md)
- [Integrations](integrations.md)
- [Development](development.md)
- [Translating](translating.md)

## Links

- Source: <https://github.com/webappsgo/redxt>
- Reference deployment: `redxt.us`

## License

MIT
