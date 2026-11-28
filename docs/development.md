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

## Local Binary Checks

```bash
make test
```

Runs the quality gates and unit tests in the Go toolchain container, then
builds the host-platform debug binaries directly in one Go toolchain
container and checks them in an isolated Alpine container. This basic test
tier builds only the server, CLI, and agent binaries it exercises; use
`make build` for all eight platform artifacts.
Use `make build` when you need the local debug binary and all eight platform
artifacts without running the checks. The server, CLI, and agent are not run
directly on the development host.

The development Compose stack is a separate manual workflow. Copy
`docker/docker-compose.dev.yml` into an isolated runtime directory and run
Compose from there; its relative `volumes/` mounts must not resolve inside
the source checkout. See the Docker volume rules in `AI.md` PART 27.

## Testing

`make test` runs the quality gates (`go vet`, `staticcheck`,
`govulncheck`, shell lint and formatting), the Go unit tests with the
coverage threshold, and the basic binary checks in `tests/run_tests.sh`.
The binary checks build the host-platform server, CLI, and agent directly
with the Docker Go toolchain, then run them in a disposable Docker container.
They use a project-scoped temporary directory for Go build temporary files;
`make build` remains the separate command for all-platform artifacts.

The full systemd/OpenRC lifecycle checks are run explicitly with
`./tests/incus.sh`; clean-OS VM checks use `./tests/virt.sh` when Incus VM
support is available. Browser end-to-end tests use `./tests/e2e.sh`.
These tiers are separate from the commit gate.

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
