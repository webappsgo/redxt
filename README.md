# redxt

**redxt** is a single-binary, fully RFC-compliant DNS platform that
replaces BIND/named, Unbound, Pi-hole, Technitium, AdGuard Home,
acme-dns, and hosted services like DuckDNS, No-IP, DynDNS, FreeDNS,
and redirect.center with one self-hosted application. It is
authoritative server, recursive resolver, forwarder, DNS firewall,
hosted dynamic-DNS provider, HTTP redirect engine, and
DNS-distributed data index in a single process — clustered,
encrypted, and observable by default.

One binary. Every DNS role. Zero external dependencies.

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
flags. See `docs/configuration.md` for the full reference once the
docs site is built out.

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
