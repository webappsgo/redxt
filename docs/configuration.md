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

Two further path variables are read on every start and override the
resolved path directly rather than being persisted:

| Variable | Effect |
|---|---|
| `CACHE_DIR` | Overrides the resolved cache directory |
| `PID_FILE` | Overrides the resolved PID file path |

Runtime variables are applied on every start and are never written back
to `server.yml`:

| Variable | Effect |
|---|---|
| `DATABASE_DRIVER` | Database driver: `sqlite`, `libsql`, `postgres`, `mysql`, `mssql`, `mongodb` |
| `DATABASE_URL` | Full database connection string, overriding the config file |
| `HOSTNAME` | Fallback hostname used by URL variable resolution when the OS hostname is unavailable |

SMTP delivery can be configured entirely from the environment, which
overrides `server.notifications.email.smtp.*` in `server.yml`:

| Variable | Effect |
|---|---|
| `SMTP_HOST` | SMTP server hostname |
| `SMTP_PORT` | SMTP server port; a non-numeric value is ignored with a warning |
| `SMTP_USERNAME` | SMTP authentication username |
| `SMTP_PASSWORD` | SMTP authentication password; redacted in all logs and in debug mode |
| `SMTP_TLS` | Enable TLS; a non-boolean value is ignored with a warning |
| `SMTP_FROM_NAME` | Display name on outgoing mail |
| `SMTP_FROM_EMAIL` | Envelope and header From address |

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
