# Installation

## Docker (Recommended)

Compose files live in `docker/`, but Compose must run from an isolated
deployment directory so its relative `volumes/` paths never create runtime
data in the source checkout. Choose a durable deployment directory outside
the repository for production:

```bash
PROJECT_ROOT="$(git rev-parse --show-toplevel)"
# Replace this with a durable path outside the source checkout.
DEPLOY_DIR="/path/to/redxt-deployment"
mkdir -p "$DEPLOY_DIR/volumes/config" "$DEPLOY_DIR/volumes/data"
cp "$PROJECT_ROOT/docker/docker-compose.yml" "$DEPLOY_DIR/docker-compose.yml"
cd "$DEPLOY_DIR" && docker compose up -d
```

The development Compose file uses DNS port `5353` so it can coexist with
a production resolver. Copy `docker/docker-compose.dev.yml` to a separate
isolated runtime directory before running it. `docker/docker-compose.test.yml`
is reserved for automated tests. See AI.md PART 27 for the temporary-directory
workflow used for local/test deployments. For a throwaway development run,
copy the Compose file into a temporary directory under
`${TMPDIR:-/tmp}/webappsgo/redxt-*`, create its `volumes/config` and
`volumes/data` directories there, and run Compose from that directory. Stop
the stack and remove only that generated temporary directory when finished.

## Binary

Release binaries are not yet published. Release builds are intended to
cover all 8 target platforms (linux/darwin/windows/freebsd × amd64/arm64).
For local development builds, use the Docker-based workflow in
[`development.md`](development.md).

```bash
./redxt-linux-amd64 --config server.yml
```

The server binary is designed to run without external runtime
dependencies once a release is available.

## Systemd Service

Running as a dedicated system user avoids permanent root. Escalation is
required only once, to install the service and bind privileged ports:

```bash
make local
sudo ./binaries/redxt --service --install
sudo systemctl start redxt
sudo systemctl status redxt
```

`--service --uninstall` and `--service --disable` reverse the install.
In `$USER` mode (no escalation), redxt binds only ports above 1024.

## Configuration

See [Configuration](configuration.md) for `server.yml`, environment
variables, and first-run behavior.
