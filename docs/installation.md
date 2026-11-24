# Installation

## Docker (Recommended)

```bash
docker run -d \
  --name "redxt-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" \
  -p 172.17.0.1:64580:80 \
  -p 53:53/tcp \
  -p 53:53/udp \
  -v ./docker/volumes/config:/config \
  -v ./docker/volumes/data:/data \
  ghcr.io/webappsgo/redxt:latest
```

The production and development compose files are in `docker/`; the test
compose file is reserved for automated tests. The development compose uses
DNS port `5353` so it can coexist with a production resolver on the host.

```bash
docker compose -f docker/docker-compose.yml up -d
```

## Binary

Static binaries are published for all 8 target platforms
(linux/darwin/windows/freebsd × amd64/arm64):

```bash
./redxt-linux-amd64 --config server.yml
```

No external runtime dependencies — a single binary is the full
deployment artifact.

## Systemd Service

Running as a dedicated system user avoids permanent root. Escalation is
required only once, to install the service and bind privileged ports:

```bash
sudo ./redxt-linux-amd64 --service --install
sudo systemctl start redxt
sudo systemctl status redxt
```

`--service --uninstall` and `--service --disable` reverse the install.
In `$USER` mode (no escalation), redxt binds only ports above 1024.

## Configuration

See [Configuration](configuration.md) for `server.yml`, environment
variables, and first-run behavior.
