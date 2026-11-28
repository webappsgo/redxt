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

The saved backup encryption password is also wrapped as `enc:v1:` with
AES-256-GCM under `server.security.encryption_key` before it is written to
`secrets.yml`. Plaintext values from older configuration are migrated after
the server key is available, then decrypted only in memory for scheduled runs.

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

`server.yml` stores node-local settings. Shared settings belong in the area file
for their section:

| File | Sections |
|---|---|
| `branding.yml` | `server.branding`, `server.seo`, `web.ui` |
| `web.yml` | `web.*` except `web.ui`; `server.cors`, `server.csrf`, `server.url_detection`, `server.compression`, `server.healthz`, `server.i18n` |
| `pages.yml` | `server.pages` |
| `contact.yml` | `server.contact` |
| `privacy.yml` | `server.privacy`, `server.compliance`, `server.tracking` |
| `security.yml` | `server.security`, `server.rate_limit`, `server.geoip`, `server.limits`, `server.tokens`, `data.*` |
| `logs.yml` | `server.logs` |
| `metrics.yml` | `server.metrics` |
| `notifications.yml` | `server.notifications` |

`server.pages.help.mcp` stores the Markdown expansion shown in the public MCP
help page. Server Admins can edit it in Pages → Help; saving writes the
canonical `yaml.d/pages.yml` area file.
| `tor.yml` | `server.tor` and legacy top-level `tor` |
| `i2p.yml` | `server.i2p` |
| `maintenance.yml` | `server.maintenance`, `server.scheduler`, `server.update`, `server.auto_update`, `server.backup`, `server.restore`, top-level `cli` |
| `accounts.yml` | `server.auth`, `server.users`, `server.orgs`, `server.session`, `server.admin`, `server.admin_path` |
| `app.yml` | Other project-defined sections |

On startup, legacy `server.yml` and drop-in settings in the wrong owner file
are moved to their canonical area file. Existing canonical owner values win
collisions. The original `server.yml` is copied to `server.yml.bak` (mode
`0600`) before area files are atomically written; then `server.yml` is
rewritten with its fixed node-local header. Migration does not change or
revalidate values. If the configuration directory cannot be written, the
migration is skipped and the merged settings remain active.

`server.restore` limits untrusted backup extraction:

| Setting | Default | Purpose |
|---|---:|---|
| `max_archive_size` | `1G` | Maximum compressed archive size |
| `max_expanded_size` | `4G` | Maximum expanded bytes, including an incremental base |
| `max_files` | `10000` | Maximum files, including the manifest and incremental base |
| `max_file_size` | `512M` | Maximum expanded size of one file |
| `max_ratio` | `100` | Maximum expanded-to-compressed ratio |

Invalid non-positive values fall back to these defaults with a configuration
warning. Each archive is checked before it is read and each entry is bounded
while streaming from the tar reader.

`server.compliance` lists enabled regulatory standards. All are disabled by
default. Enabling HIPAA, SOC 2, or PCI-DSS raises the effective shared password
policy to at least 12 characters, uppercase, number, special character, 90 day
maximum age, and 12 remembered passwords. Account creation and password changes
currently enforce the length and composition requirements; password expiry and
history are not enforced yet.

`server.security.password` configures the shared password length and composition
requirements. Its default is 8 characters with no extra composition classes.

## Maintenance Recovery

`server.maintenance` is stored in `yaml.d/maintenance.yml`.

| Setting | Default | Purpose |
|---|---:|---|
| `self_healing.retry_interval` | `30s` | Delay between background recovery attempts |
| `self_healing.max_attempts` | `0` | Retry limit; zero retries without a limit |
| `cleanup.disk_threshold` | `90` | Disk usage percentage that triggers cleanup |
| `cleanup.log_retention_days` | `7` | Age after which maintenance cleanup may remove logs |
| `notify.on_enter` | `true` | Send a notification when maintenance begins |
| `notify.on_exit` | `true` | Send a notification when maintenance ends |

Invalid retry intervals, limits, and cleanup thresholds are replaced by their
defaults and recorded as configuration warnings.

CLI and agent releases are operator-provisioned in `{data_dir}/cli/`. The
directory contains the platform binaries, `version.txt`, and `sha256.txt`;
the server publishes and streams only binaries whose checksum matches. The
`cli.binary_download.dir` setting in `yaml.d/maintenance.yml` overrides this
directory, and `cli.binary_download.require_auth` defaults to `false` for CLI
downloads. Agent binary downloads always require an agent-scoped Bearer token.

When the server cannot open or validate either database during startup, it
enters read-only maintenance mode and keeps the HTTP listener and health
endpoint available. The dashboard at `/server/{admin_path}` shows the retry
state and general database recovery checks without exposing configured
credentials or internal connection errors. Health reports HTTP 503 and
`status: maintenance` until both database schemas are available; successful
recovery resumes database-backed services and removes the dashboard. A
positive `self_healing.max_attempts` stops retries at that limit; correcting
the database and restarting the server begins a new recovery cycle.

## Browser CSRF

The double-submit CSRF cookie is controlled by `server.csrf` in the merged
configuration. `enabled` defaults to `true`; `secure` accepts `auto`,
`true`, or `false` and defaults to `auto`, which sets the cookie's `Secure`
attribute when the request scheme resolves to HTTPS. `exempt_paths` lists
frontend paths that cannot use browser form tokens; backend API and other
protocol routes are outside CSRF validation.

## Web Policy

`web.cors` is stored in `yaml.d/web.yml`. Its default `"*"` allows the CORS
resolution order to use `DOMAIN` and trusted-proxy-learned origins; an explicit
comma-separated origin list takes precedence, while an empty value disables
CORS. Credentials are not allowed with the wildcard fallback.

`web.hsts` controls Strict-Transport-Security and is emitted only on TLS
responses. The default enables a two-year max age with `includeSubDomains` and
`preload`; tune it with `web.hsts.max_age_seconds`. Legacy `server.web.*`
settings migrate into root `web.*` settings.

## API Token Lifetimes

`server.security.tokens` sets the default finite lifetime, maximum finite
lifetime, and rotation reminder interval for API tokens. The default lifetime
is `0` (no expiry), the maximum is `365d`, and the reminder interval is
`90d`. The admin token form starts with the configured default and still lets
an administrator explicitly choose Never, a supported preset, or a custom
expiration. Finite expirations beyond `max_expiry` are rejected. Invalid
values fall back to the documented defaults; a reminder later than the maximum
lifetime is reduced to the maximum. The admin token list marks active tokens
for rotation once their age reaches `rotation_reminder`. Regular User token
defaults remain controlled by `server.users.tokens.expiration_days`, and are
also bounded by `max_expiry`.

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
served under `/server/{admin_path}`. It must be 2–32 lowercase letters,
numbers, or interior hyphens and cannot use a reserved server route such as
`api`, `health`, `about`, `privacy`, `security`, or `preferences`. An invalid
configured value falls back to `administration`. See [Admin Panel](admin.md).

## Update Channel

`server.update.branch` selects `stable` (default), `beta`, or `devel`.
Channels are cumulative: beta includes stable releases, and devel includes
stable, beta, and the rolling `devel` release. The legacy values `daily`,
`rolling`, `dev`, and `development` are accepted and normalized to `devel`.
`server.update.auto_install` defaults to `false`; scheduled checks notify about
eligible releases without replacing the running binary. `server.update.defer_days`
delays scheduled eligibility by 0–365 days, while a manual update check ignores
the delay. The CLI can change the canonical setting with
`redxt --update branch {stable|beta|devel}`.
