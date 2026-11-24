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

The official site and flagship public instance run at
[redxt.us](https://redxt.us).

## Status

This project is in active bootstrap/development. See `TODO.AI.md`
for the current implementation backlog and `IDEA.md` for the full
business-logic specification.

## Installation

Production builds are not yet published. Once released, single
static binaries will be available for linux/darwin/windows/freebsd
on amd64/arm64.

## Configuration

redxt is configured via `server.yml`, environment variables, and CLI
flags. See [`docs/configuration.md`](docs/configuration.md) for the
configuration reference.

## Client

`redxt-cli` is the companion CLI/TUI client, required for all
`redxt` deployments. See `docs/cli.md`.

## Development

```bash
make build   # all platforms, via casjaysdev/go:latest in Docker
make test    # go vet + govulncheck + go test, via Docker
make docker  # multi-arch image build (no push)
```

See `docs/development.md` for the full development guide.

## Disclaimer

redxt is provided "as is" without warranty of any kind. See
`LICENSE.md`.

## License

MIT — see `LICENSE.md`.
