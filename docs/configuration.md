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
| `DNS_PORT` | DNS UDP/TCP listener port; overrides `dns.port` at runtime |
| `DATABASE_DRIVER` | Driver identifier: `sqlite`, `libsql`, `postgres`, `mysql`, `mssql`, or `mongodb`. SQLite is the only implemented driver; selecting another value fails at startup. |
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
| `I2P_ENABLED` | Overrides `server.i2p.enabled` on every start |

The built-in DNS listener uses UDP and TCP on port 53 by default. It follows
`server.listen` unless `dns.address` names a dedicated interface. Set
`dns.enabled: false` to disable it or `dns.port` to override the standard
port. `DNS_PORT` overrides `dns.port` at runtime and is never persisted.
The current listeners serve verified primary and secondary zones. DNS-over-
HTTPS is available at `/dns-query` on the HTTPS listener (GET and POST using
`application/dns-message`); requests must arrive over TLS or through a trusted
proxy that reports HTTPS. Recursive resolution and other encrypted transports
(DNS-over-TLS, DNS-over-QUIC, and DNSCrypt) are not available yet.

Tor settings live under `server.tor`; the active onion address is runtime
state and is never written to `server.yml`. The I2P eepsite settings live
under `server.i2p`. Both overlays default to hidden from `/api/autodiscover`;
set the respective `expose` setting to `true` to publish an active address.

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
served under `/server/{admin_path}`. The `preferences` segment is reserved
for guest preferences and falls back to the default admin path. See
[Admin Panel](admin.md).
