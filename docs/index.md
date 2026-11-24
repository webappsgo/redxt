# redxt

**redxt** is a fully RFC-compliant DNS platform that replaces
BIND/named, Unbound, Pi-hole, Technitium, AdGuard Home, acme-dns, and
hosted services like DuckDNS, No-IP, DynDNS, FreeDNS, and redirect.center
with one self-hosted application. Its server handles authoritative DNS,
recursive resolution, forwarding, DNS firewall, hosted dynamic DNS, HTTP
redirects, and DNS-distributed data in one process. Companion CLI and
data-plane agent binaries provide operator access and distributed DNS
service.

One server process for every DNS role, with no required external service dependency.

## Quick Start

```bash
# Docker
docker run --name "redxt-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" -p 172.17.0.1:64580:80 ghcr.io/webappsgo/redxt:latest

# Binary
./redxt-linux-amd64 --config server.yml
```

On first run redxt selects a random port in the 64000-64999 range
(persisted to `server.yml`), prints a one-time setup token to the
console, and serves the setup wizard until the Primary Admin account is
created.

## Features

- Authoritative primary/secondary, recursive resolver, forwarder, and
  hybrid/split-horizon views in one process
- DNS firewall/blocker with RPZ-style policies
- Hosted dynamic-DNS provider and HTTP redirect engine
- Data-zone publisher (DNS-distributed datasets)
- Multi-user accounts, organizations, and custom domains
- Cluster support for horizontal scaling across redxt instances
- Built-in Let's Encrypt issuance, Tor hidden service, Prometheus
  metrics, scheduler, GeoIP risk signals, backup/restore, self-update

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
