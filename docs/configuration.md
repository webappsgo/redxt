# Configuration

## Config File

redxt reads `server.yml` (never `.yaml` — a legacy `server.yaml` is
auto-migrated on startup). Default locations:

| Environment | Path |
|---|---|
| Linux (privileged) | `/etc/webappsgo/redxt/server.yml` |
| Linux (user) | `~/.config/webappsgo/redxt/server.yml` |
| Docker | `/config/redxt/server.yml` |

Comments in `server.yml` always go above the setting they document,
never inline.

Settings resolve in priority order: CLI flag > environment variable >
config file > built-in default.

## Environment Variables

Init-only variables apply only on first run and are then persisted into
`server.yml`:

`CONFIG_DIR`, `DATA_DIR`, `LOG_DIR`, `DATABASE_DIR`, `BACKUP_DIR`,
`PORT`, `LISTEN`, `APPLICATION_NAME`, `APPLICATION_TAGLINE`

Mode and debug behavior:

| Variable | Effect |
|---|---|
| `MODE` | `production` (default), `development`, or `debug` |
| `DEBUG` | Enables `/debug/*`, pprof, expvar — independent of `MODE` |

`--mode` and `--debug` CLI flags take precedence over their environment
variable equivalents. Debug mode never bypasses authentication, and
credentials (keys, tokens, passwords, secrets) are redacted in every
mode, including debug.

On first run, redxt selects a random port in the 64000-64999 range if
none is configured, and persists the chosen port to `server.yml`.

## Admin Panel

The admin panel path is configurable (default `administration`) and is
served under `/server/{admin_path}`. See [Admin Panel](admin.md).
