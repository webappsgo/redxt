# Development

## Prerequisites

- Docker (all builds and tests run in `casjaysdev/go:latest` — never
  build or test on the bare host)
- Optionally Incus, for full-OS/systemd integration testing

## Build

```bash
make build
```

Builds `redxt`, `redxt-cli`, and `redxt-agent` for all 8 target
platforms (linux/darwin/windows/freebsd × amd64/arm64) inside the
`casjaysdev/go:latest` toolchain container, with `CGO_ENABLED=0` and
version/commit/build-date stamped via `-ldflags`.

## Run Locally

```bash
make dev
```

Builds the server, CLI, and agent (when present) inside the Go toolchain
container and writes them to a unique directory under
`${TMPDIR:-/tmp}/webappsgo/redxt-*`. It does not start a server or a Compose
stack. Run the generated server binary to exercise it locally.

The development Compose stack is a separate manual workflow. Copy
`docker/docker-compose.dev.yml` into an isolated runtime directory and run
Compose from there; its relative `volumes/` mounts must not resolve inside
the source checkout. See the Docker volume rules in `AI.md` PART 27.

## Testing

Two phases:

1. **Unit tests** — `make test` runs `go test -v -cover ./...` inside
   Docker and enforces at least 60% total statement coverage. CI runs
   `go vet`, `staticcheck`, and `govulncheck` as separate gates.
2. **Binary smoke tests** — `./tests/run_tests.sh` runs the Docker
   Alpine tier. It builds the binaries with `make build`, starts the
   server, and checks version/help, public health endpoints, browser
   admin gating, binary rename behavior, and CLI health. The Incus
   service-lifecycle suite is separate and must be invoked explicitly
   with `./tests/incus.sh`; it is not selected by `run_tests.sh`.
3. **Browser E2E tests** — `./tests/e2e.sh` is a separate manual tier
   against a running server. It is not part of `run_tests.sh`.

The current binary smoke suite does not yet implement the full PART 29
operator setup, authenticated CRUD, Tor/I2P, and agent workflows. See
`TODO.AI.md` before treating those behaviors as covered.

## Contributing

- Read `AI.md` (source of truth) and `IDEA.md` (business logic) before
  implementing any feature
- Every code change should trace to a specific `AI.md` PART or
  `IDEA.md` line
- Follow the project's `CLAUDE.md` and `.claude/rules/*.md` conventions

## Code Style

- `gofmt`-clean, `go vet`-clean, `CGO_ENABLED=0`
- Table-driven Go tests; unit tests live as `*_test.go` next to the
  code they cover
- Comments go above the code they document, never inline
- No TODO/FIXME/stub code in committed lines — deferred work is
  recorded in `TODO.AI.md` with its blocking dependency

## Project-Specific Customization

See `IDEA.md` for redxt's product scope, non-goals, and the roles that
distinguish this project from the generic AI.md spec (DDNS, redirect
engine, data zones, cluster vs. managed nodes).
