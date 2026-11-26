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

`server.yml` is a private file and is secured to mode `0600` before it is
read. Config saves use a temporary file in the same directory, sync the
file, atomically replace `server.yml`, and sync the directory where the
operating system supports directory syncing.

Settings resolve in priority order: CLI flag > environment variable >
`secrets.yml` > `yaml.d/` area files in filename order > `server.yml` >
built-in default. Maps merge by key; a later scalar or list replaces the
earlier value.

`secrets.yml` is an optional private `0600` file merged last. It stores the
operator token, encryption key, generated metrics credentials, SMTP/cache/
database passwords and tokens, credential-bearing cache/database URLs, backup
encryption password, CAPTCHA and CVE API secrets, contact webhook URLs and
signing secrets, LDAP bind passwords, and OIDC client secrets. LDAP and OIDC
provider credentials merge with providers by `name`. Supported credentials found in `server.yml` or a
`yaml.d/` drop-in are written to `secrets.yml` before their source file is
rewritten without those values. DNS-01 credentials use the flat
`server.ssl.letsencrypt.dns_credentials` map keyed by lego environment variable
name; plaintext values are encrypted into `secrets.yml` after the server
encryption key is available and decrypted only in memory. DNS provider
construction is checked at startup and before certificate requests when
DNS-01 is selected, and issuance uses the configured lego provider. The admin
panel can edit provider settings without returning credential values.
Issuance status tracking and key-rotation support remain in progress.

On first start, redxt creates `server.token` (`adm_` plus 32 random base62
characters) and the three `mtr_` metrics tokens, stores them in `secrets.yml`,
and audits their key names without logging their values. The operator token
is printed once to the startup console. Metrics tokens remain in the private
file and authorize only their metrics service.

Drop-in files are regular `*.yml` and `*.yaml` files directly under
`yaml.d/`; hidden files and nested directories are ignored. Symlinks are
accepted only when their target remains inside `yaml.d/`. The directory is
restricted to mode `0700` and loaded files to `0600`. An unparseable file
is ignored with a warning; malformed `server.yml` uses built-in defaults
and is left in place for repair. Invalid individual values, explicit nulls,
duplicate keys, and unknown keys are handled independently with a warning
that names the source file, line, and key, so valid neighboring settings
remain active.

Privacy and consent settings belong in `yaml.d/privacy.yml`. On startup,
legacy `server.privacy` values in `server.yml` are migrated there and removed
from `server.yml`; the area file is created atomically with mode `0600` inside
the `yaml.d/` directory (mode `0700`).

## Browser CSRF

The double-submit CSRF cookie is controlled by `server.csrf` in the merged
configuration. `enabled` defaults to `true`; `secure` accepts `auto`,
`true`, or `false` and defaults to `auto`, which sets the cookie's `Secure`
attribute when the request scheme resolves to HTTPS. `exempt_paths` lists
frontend paths that cannot use browser form tokens; backend API and other
protocol routes are outside CSRF validation.

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
| `DOT_PORT` | DNS-over-TLS listener port; overrides `dns.dot.port` at runtime |
| `DOQ_PORT` | DNS-over-QUIC listener port; overrides `dns.doq.port` at runtime |
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
TLS uses the shared certificate manager on port 853 by default when a usable
certificate is configured. Set `dns.dot.address` to bind it to a dedicated
interface, or leave it empty to inherit the DNS listener address. `DOT_PORT`
overrides `dns.dot.port` at runtime and is never persisted. DNS-over-QUIC
uses the shared certificate manager and a dedicated QUIC listener on port 853
by default when a usable certificate is configured. Set `dns.doq.address` to
bind it to a dedicated interface, or leave it empty to inherit the DNS
listener address. `DOQ_PORT` overrides `dns.doq.port` at runtime and is never
persisted. DNS-over-HTTPS is available at `/dns-query` on the HTTPS listener
(GET and POST using
`application/dns-message`); requests must arrive over TLS or through a trusted
proxy that reports HTTPS. DNSCrypt is not available yet.

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

## Regular User Names

`server.users.username_blocklist_extra` adds exact names to the built-in
Regular User username blocklist. Values are trimmed, lowercased, and
Unicode NFKC-normalized at load time. The list only adds restrictions; it
does not replace built-in entries or affect organization slugs, Server
Admin usernames, or the shared reserved-name rules.
It can be set in `server.yml` or `yaml.d/accounts.yml`; the later file
replaces the whole configured list.

```yaml
server:
  users:
    username_blocklist_extra:
      - brandname
      - product-team
```

## Admin Panel

The admin panel path is configurable (default `administration`) and is
served under `/server/{admin_path}`. The `preferences` segment is reserved
for guest preferences and falls back to the default admin path. See
[Admin Panel](admin.md).
