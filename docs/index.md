# redxt

**redxt** is an in-development DNS platform. The current server provides
authoritative answers for configured primary and secondary zones over DNS
UDP/TCP and DNS-over-HTTPS. Recursive resolution, forwarding, DNS firewall,
hosted dynamic DNS, HTTP redirects, DNS-distributed data, clustering, and
managed-agent workflows are product requirements that remain in progress.

The product is intended to consolidate DNS roles in one service; only the
authoritative and DoH surfaces described above are currently implemented.

## Quick Start

```bash
# Docker
docker run --name "redxt-$(od -An -N4 -tx1 /dev/urandom | tr -d '[:space:]')" -p 172.17.0.1:64580:80 -p 53:53/tcp -p 53:53/udp ghcr.io/webappsgo/redxt:latest

# Local development binary
./redxt-linux-amd64 --config server.yml
```

On first run redxt selects a random port in the 64000-64999 range
(persisted to `server.yml`), prints a one-time setup token to the
console, and serves the setup wizard until the Primary Admin account is
created.

## Current Features

- Authoritative DNS over UDP and TCP
- RFC 8484 DNS-over-HTTPS at `/dns-query` on HTTPS
- Server-rendered setup and administration surfaces
- Companion CLI and agent binaries with functionality still under
  development

See the [implementation backlog](https://github.com/webappsgo/redxt/blob/main/TODO.AI.md)
for current status and [IDEA.md](https://github.com/webappsgo/redxt/blob/main/IDEA.md)
for the product requirements.

## Documentation

- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
- [CLI Reference](cli.md)
- [Admin Panel](admin.md)
- [Security](security.md)
- [Integrations](integrations.md)
- [Development](development.md)

## Links

- Source: <https://github.com/webappsgo/redxt>
- Reference deployment: `redxt.us`

## License

MIT
