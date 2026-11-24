# redxt

**redxt** is an in-development DNS platform. The current server provides
authoritative answers for configured primary and secondary zones over DNS
UDP/TCP and DNS-over-HTTPS. Recursive resolution, forwarding, DNS firewall,
hosted dynamic DNS, HTTP redirects, DNS-distributed data, clustering, and
managed-agent workflows are product requirements that remain in progress.

One server process for every DNS role, with no required external service dependency.

## Quick Start

```bash
# Docker
docker run --name "redxt-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" -p 172.17.0.1:64580:80 -p 53:53/tcp -p 53:53/udp ghcr.io/webappsgo/redxt:latest

# Binary
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

See [`TODO.AI.md`](../TODO.AI.md) for the current implementation backlog and
[`IDEA.md`](../IDEA.md) for the complete product requirements.

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
