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

Starts the stack via `docker-compose.dev.yml` with hot-reload and
`MODE=development`.

## Testing

Two phases:

1. **Unit tests** — `make test` runs `go vet`, `govulncheck`, and
   `go test -v -cover` inside Docker; ≥60% total coverage is required
   before any commit.
2. **Integration tests** — `./tests/run_tests.sh` auto-detects Incus or
   Docker and exercises the compiled binaries end-to-end (service
   install, health checks, admin-route gating, CLI/agent smoke tests).
   `./tests/e2e.sh` runs headless-Chromium browser tests against a
   running server.

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
