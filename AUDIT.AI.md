# Project Audit

Started: 2026-08-26

Scope: full six-pass health audit of redxt — security, code quality, logic,
documentation, line-by-line AI.md PART 0-36 compliance, and code flow trace.
Every PART was walked against the actual code, not from memory.

Verification baseline for every fix below: `gofmt -l ./src` clean, `go vet
./...` clean, `go build ./...` exit 0, `go test ./...` all pass, `go test
-race` clean on scheduler/metrics/overlay/backup/daemon/cache — all run in
`casjaysdev/go:latest` with `CGO_ENABLED=0` and `GOFLAGS=-buildvcs=false`,
never on the host.

**Headline:** the premise that PARTs 0-36 are complete does not hold. PARTs
0-15, 26, 27, 28 are in good-to-excellent shape. PARTs 16, 17, and large
parts of 34, 35, 36 have subsystems that do not exist, and several
TODO.AI.md items are marked `[x]` for work that is demonstrably unbuilt.

---

## Pass 1: Security

- [ ] `src/server/service/org.go` `GrantZone`: never validates that the
      granted `zoneID` belongs to the granting org, and `zone_grants.zone_id`
      carries no `REFERENCES zones(id)` FK (unlike its sibling columns in
      `src/database/schema_users.go`). `CanEditZone` reads the grant back
      unvalidated and `src/server/service/token.go:126` mints a token
      carrying that `zone_id`. Not yet reachable end-to-end (no record-write
      path consumes `zone_grants`), but becomes cross-tenant record write the
      moment DNS record handlers honor it. FIX: reject when
      `zone.OrgID != orgID`; add the FK. — HIGH
- [ ] `src/server/handler/api.go:267-274` / `org.go:230-252`: `memberView`
      carries an unconditional `Email` field and `apiMembers` populates it
      for every row after only a `PermRead` check, so any org Viewer harvests
      every member's address. AI.md 62170-62232 marks `email` as NOT visible
      within an org and requires `profile_visibility` masking per the
      target's `visibility` + `org_visibility`. `src/server/store/member.go:38`
      selects `u.email` unconditionally. — HIGH
- [ ] Client/agent credential files are read with no permission gate. AI.md
      52838/52847 require `os.Stat` and refusal when
      `info.Mode().Perm()&0o077 != 0`. `src/client/config.go` `LoadConfig`
      and `ResolveTokenFile`, and `src/agent/config.go:137+`, all read
      without the check (`SaveConfig` does write 0600, so only the read-side
      refusal is missing). FIX: shared `checkCredentialPerms(path)` helper
      called from every credential read, non-Windows. — HIGH
- [ ] `src/client/http.go`: no `401 TOKEN_REVOKED` handling. AI.md 52856
      requires exit code 4 and 52859 requires deleting the cached token from
      `cli.yml`/`token`. Neither exists; `TOKEN_REVOKED` appears nowhere
      under `src/client` or `src/agent`. — HIGH
- [ ] `src/overlay/i2p.go:199-211`: the `i2pd` child is sent `os.Interrupt`
      and immediately nil'd, with no `cmd.Wait()` anywhere in `src/overlay`.
      An i2pd that ignores SIGINT survives; one that exits becomes a zombie
      for the parent's lifetime, and the health-check restart loop at
      `i2p.go:690-700` accumulates them. FIX: `Wait()` with a timeout, then
      `Process.Kill()`. — MEDIUM
- [ ] `src/overlay/tor.go:92-145`: generated torrc omits
      `PublishServerDescriptor 0` and `SocksPolicy accept 127.0.0.1`, both
      required by AI.md 50175/50177. Defense-in-depth only — `ORPort 0`
      already prevents descriptor publication and SocksPort binds loopback.
      Note AI.md's own reference torrc at 50868-50921 also omits them while
      the normative list at 50172-50177 requires them. — MEDIUM
- [ ] `src/server/service/domain.go:165`: `require_ssl` is used only as
      `SSLEnabled: policy.requireSSL` at creation. A domain is marked
      `active` on TXT verification and a later `issueCertificate` failure
      does not roll that back, so a `require_ssl: true` deployment serves an
      active, certificate-less domain. FIX: hold at `pending` until issuance
      succeeds. — MEDIUM
- [ ] `verification_ttl` (AI.md 62554) is defaulted in
      `src/config/defaults.go:327-338` but `domainConfig()` at
      `src/server/service/domain.go:79-91` never carries it into
      `domainPolicy`, so a pending verification token never expires. — MEDIUM

## Pass 2: Code Quality

- [x] `tests/incus.sh`: shellcheck SC2064 — trap body expanded
      `$CONTAINER_NAME` at definition time — FIXED (single-quoted trap body)
- [x] `tests/{docker,incus,e2e}.sh`: unused `PROJECT_ORG` assignment
      (SC2034) — FIXED (removed; `PROJECT_NAME` is used and kept)
- [x] `src/server/middleware/geoip.go`: dead `GeoResult.Anonymous` field,
      declared and documented but never populated anywhere — FIXED (removed)
- [x] `src/config/validate.go`: dead `_ = def` assignment in
      `validateGeoIP` — FIXED (removed)
- [x] `src/backup/archive.go:98`: unbounded `io.ReadAll` on tar extraction —
      a crafted gzip "tar bomb" in a restore upload could exhaust process
      memory — FIXED (new `readEntry` reads through an
      `io.LimitReader(tr, maxEntryBytes+1)` with a 512 MiB cap and errors
      rather than truncating)
- [ ] `src/client/http.go`: `do(method, path, body, out)` accepts `body` and
      never marshals or sends it — every POST/PUT the CLI eventually needs
      silently sends nothing. Violates the project's no-partial-implementation
      rule. FIX: encode it, or drop the parameter until needed. — LOW
- [ ] 19 uses of AI.md-banned generic type names (`type Config struct`,
      `type Options struct`, `type Result struct`) — AI.md 4730-4793 requires
      intent-revealing names. `src/config/config.go:29 type Config struct` is
      the exact case the spec names. Full list: server/server.go:54,
      server/admin/handler.go:34, server/service/service.go:61,
      client/flags.go:17, server/handler/handler.go:34, graphql/graphql.go:38,
      server/middleware/chain.go:83, client/config.go:15, agent/config.go:17,
      backup/backup.go:101, cli/flags.go:21, daemon/daemon.go:23,
      logging/logger.go:57, cache/cache.go:48, agent/flags.go:20,
      urlvars/resolve.go:112, config/config.go:29, geoip/geoip.go:90,
      swagger/swagger.go:155. — MEDIUM
- [ ] `src/signals/` should be `src/signal/` (AI.md 4906-4912, and the Go
      singular-package-dir convention). — MEDIUM
- [ ] `src/agent/help.go`: help columns mix 39 and 40 chars; PART 8 fixes a
      38-char left-aligned item column. — LOW

## Pass 3: Logic and Correctness

- [x] `src/metrics/text.go`: **counter names were doubled** —
      `writeFamily` appended a `_total` suffix to names AI.md PART 21 already
      requires to be registered as `..._total`, emitting
      `redxt_requests_total_total` while HELP/TYPE announced
      `redxt_requests_total`, orphaning every counter for any scraper —
      FIXED (sample name is now the registered name verbatim)
- [x] `src/metrics/text.go`: **concurrent map read/write crash on the
      exposition path** — the renderer took `r.mu` per family and released it
      before ranging `f.series`/`f.labelValues`, while Counter/Gauge/Histogram
      insert new series keys into those same maps. A scrape overlapping a
      request that carried an unseen label combination tripped Go's
      unrecoverable concurrent-map fatal error, killing the process — FIXED
      (single snapshot of all three family maps taken under one `r.mu`
      acquisition via new `snapshotFamilies`/`snapshotHistograms`; values stay
      as pointers so rendering still reads live numbers, each guarded by its
      own mutex). The existing tests never scraped concurrently with writes,
      which is why `-race` had never surfaced it.
- [x] `src/metrics/registry.go`: label values escaped with Go's `%q`, which
      also escapes non-ASCII as `\xNN`/`\uNNNN` — unparseable by Prometheus
      and silently corrupting UTF-8 values such as an ASN organization name —
      FIXED (`labelValueEscaper` implements only the three escapes the text
      exposition parser accepts: `\\`, `\n`, `\"`)
- [x] `src/metrics/http.go`: `req.ContentLength` observed verbatim into the
      request-size histogram; Go reports `-1` for unknown length (chunked
      encoding and most GETs), driving `_sum` negative and making average-size
      queries wrong — FIXED (new `requestSize` maps unknown to 0)
- [x] `src/scheduler/cron.go`: `@weekly` and `@monthly` fell through to
      `parseCron`, which rejects a 1-field expression. Because `Register`
      propagates the error and `startScheduler` returns it, **an admin
      setting `@weekly` in config failed server startup** — FIXED (both
      descriptors resolve to cron expressions). `@hourly` also drifted from
      process start rather than firing on the hour — FIXED (`0 * * * *`);
      test expectation corrected and `@weekly`/`@monthly` cases added.
- [x] `src/mode/mode.go`: used `strconv.ParseBool`, an explicit project NEVER
      rule (AI.md 55119), so `DEBUG=yes`/`on`/`enabled` silently evaluated
      false; and an empty `DEBUG=""` counted as an explicit source,
      suppressing the `MODE=debug` default-on — FIXED (new `debugEnv` returns
      `*bool` through `config.ParseBool`; nil means "not an explicit source")
- [ ] `src/client/flags.go` and `src/agent/flags.go` register `--debug` via
      `flag.BoolVar`, which is stdlib `strconv.ParseBool` — the same banned
      path just removed from `src/mode`. `ParseBool`/`IsTruthy` appear
      nowhere under `src/client` or `src/agent` despite existing at
      `src/config/boolean.go:23,36`. — MEDIUM
- [ ] `src/server/service/account.go:468-481`: recovery codes are
      `security.RandomString(recoveryCodeLength)`, not the AI.md 59279 format
      `a1b2c3d4-e5f6`; and `consumeRecoveryCode` trims but never lowercases,
      so a correctly-typed uppercase key fails. — MEDIUM
- [ ] `src/client/config.go:61`: `--config` profile resolution is
      `filepath.Join(configDir, name)` verbatim, so `--config test` resolves
      to an extensionless `{config_dir}/test` and `~`/absolute paths are
      silently joined under the config dir. AI.md 54740-54754 specifies
      `test` → `{config_dir}/test.yml` with `.yml`-then-`.yaml` autodetect
      and pass-through for `~`/absolute. — MEDIUM
- [ ] `src/client/run.go`: enters the blocking TUI with no TTY check, so
      `redxt-cli | tee` hangs on stdin instead of printing plain output.
      AI.md 53294/53343 require the plain-output fallback, and
      `src/common/display/detect.go` already implements the detection. — MEDIUM
- [ ] Client and agent both resolve paths through the server's privileged-aware
      `paths.Resolve()`, so `sudo redxt-cli` writes `cli.yml` to
      `/etc/webappsgo/redxt/` and a non-root `redxt-agent` reads from
      `~/.config/...`. AI.md 57074-57084 fixes the client to user scope and
      the agent to system scope regardless of privilege. — MEDIUM
- [ ] `src/client/run.go`: exit-code taxonomy unimplemented. AI.md
      56260-56270 requires 2 configuration, 3 connection, 4 authentication,
      5 not found, 64 usage error; only 0/1/2 are ever returned. — MEDIUM

## Pass 4: Documentation Completeness

- [x] `docs/configuration.md`: 12 env vars read by the binary but documented
      nowhere (`CACHE_DIR`, `PID_FILE`, `DATABASE_DRIVER`, `DATABASE_URL`,
      `HOSTNAME`, and the seven `SMTP_*`) — FIXED (all documented)
- [x] `docs/cli.md`: listed 5 of 11 real client flags, documented a
      `status` command that does not exist while omitting the real `health`
      command, and gave shell-completion syntax backwards — FIXED (all 11
      flags, env-var table, token priority chain, correct
      `redxt-cli --shell completions bash`, expanded agent section)
- [x] `TODO.AI.md`: sub-60%-coverage list omitted `src/startup` (46.1%) —
      FIXED
- [ ] `LICENSE.md` third-party attribution lists 2 of 13 direct deps.
      Missing: chromedp/cdproto, chromedp/chromedp, cretz/bine,
      dustin/go-humanize, google/uuid, pires/go-proxyproto, golang.org/x/crypto,
      golang.org/x/net, golang.org/x/sys, modernc.org/sqlite, plus all 11
      indirects. AI.md 5776-5778/5894-5899. No GPL/AGPL/LGPL dep exists —
      that half is clean. — HIGH
- [ ] `README.md`: zero badges (AI.md 4967 requires every badge linked,
      5026-5032 requires the dynamic GitHub license badge) and 5 of the 12
      AI.md 4950-4963 sections are missing. The Disclaimer block is present
      and correct. — HIGH
- [ ] `Jenkinsfile` absent — AI.md 6295 required root tree, full template at
      AI.md 44202. — HIGH
- [ ] `.github/CODEOWNERS`, `.github/SECURITY.md`, `.github/ISSUE_TEMPLATE/`,
      and the PR template are all absent (AI.md 3235-3251, 4350-4357);
      `.github/` contains only `workflows/`. — HIGH
- [ ] `renovate.json` absent (AI.md 4354). — HIGH
- [ ] `.gitea/workflows/` absent (AI.md 3235-3251, templates at 42259-43312).
      — HIGH
- [ ] `scripts/` is empty — `scripts/verify-licenses.sh` (AI.md 5935-5970)
      and `.github/workflows/licenses.yml` (AI.md 5905-5929) both missing.
      — MEDIUM
- [ ] `tests/docker.sh:81` and `tests/incus.sh:105` use `curl -q -LSs`
      without `-f`, against the AI.md 5505-5578 project standard. Both
      deliberately capture `%{http_code}`, where `-f` would suppress it —
      a justified exception, but undocumented. FIX: add an above-line comment
      naming the exception. — LOW

## Pass 5: Spec and Rules Compliance

### TODO.AI.md reconciliation — items marked `[x]` that are not done

- [ ] Web frontend (~line 119) — `src/server/templates` and static assets do
      not exist; PART 16 is unbuilt.
- [ ] Privilege escalation (line 280) — PART 24/25 unimplemented on all three
      binaries.
- [ ] Metrics (line 243) — was marked done while carrying two CRITICAL
      defects (now fixed above); the grafana dashboard and loki stream
      surfaces still need verification against PART 21.
- [ ] GeoIP (line 224) — see the enforcement gap below.
- [ ] Database layer & clustering (line 54) — marked `[x]`, but only
      `modernc.org/sqlite` of the six drivers AI.md PART 10 requires is
      present in `go.mod`.
- [ ] Line 157 claims token resolution is "flag > env > config file > token
      file"; that ordering matches neither AI.md 52785 nor 55487 and omits
      source #5 entirely. The TODO text is itself wrong.
- [ ] Stale entries to prune: `ssl_renewal` is wired; metrics is mounted;
      `server.metrics.root.enabled` already exists.

### Deferral verification (TODO.AI.md 150-195)

All six PART 33 deferrals were re-checked against code. **Five hold** — the
`--admin` blocker holds (`src/admin/` is an empty directory, `routes.Admin`
is a placeholder), agent register/enrollment holds (no server-side handler
exists), agent foreground holds, `--service`/`--update` holds. **One is
borderline:** cluster failover's stated blocker ("depends on
`/api/autodiscover` client-side consumption") is self-referential — the
server side exists at `src/server/router.go:141` + `src/server/autodiscover.go`,
so nothing external blocks it. Not stale, but schedulable now.

### GeoIP country blocking is configured but inert — needs a decision

- [ ] `geoip.Service.Blocked()` (`src/geoip/geoip.go:300`) is fully
      implemented and tested — allow-wins-over-deny, fail-open,
      private-IP-exempt — and has **zero production callers**, so
      `server.geoip.deny_countries` / `allow_countries` are silently no-ops.
      AI.md PART 20 lines 34467-34472 give an explicit behavior table
      requiring them to block. IDEA.md:308 legitimately overrides PART 20's
      enabled-by-default posture but is silent on, and therefore does not
      override, the enforcement requirement.

      **This is deliberately not fixed inline.** `src/server/middleware/geoip.go`
      documents at length that the stage annotates and never gates, citing
      PART 11's "GeoIP is a risk signal, never a sole access gate" — a
      genuine spec-internal tension between PART 11 and PART 20 that
      `Blocked()`'s fail-open, private-exempt design already half-reconciles.
      Wiring it in is a user-visible behavior change (requests that pass
      today would be refused), which is a stop-and-ask category. Both lists
      default to empty, so the change is a no-op under default config and
      only affects an operator who explicitly configured the lists and is
      currently getting silence. RECOMMENDED FIX: add an optional
      `Blocked func(ip string) bool` seam to `GeoIPOptions` and bind
      `geoip.Service.Blocked` to it at startup.

### PART 34 — Multi-User

- [ ] `admins` and `admin_sessions` are in **users.db**, not server.db.
      AI.md 60604-60618 and 61482-61517 state the two-database split twice,
      with an explicit security rationale ("Compromised user DB doesn't
      expose admin"). `src/database/schema_users.go:20,37` registers both in
      `usersCoreTables`. FIX: move to `serverCoreTables` and repoint
      `src/server/admin`. — HIGH
- [ ] Server Admin moderation surface absent entirely — AI.md 60451-60472
      and 60202-60209 (`/moderation/users`, `/users/disable`, `/enable`,
      `/impersonate`, `/moderation/orgs`, …). Zero matches for "moderation"
      or "impersonate" under `src/`. With registration enabled there is no
      admin path to disable or delete an abusive user or org. — HIGH
- [ ] Recovery-key routes absent: `/server/auth/recovery/use` (60089/60308),
      `/users/security/recovery` and `/recovery/regenerate` (60356-60357).
      — MEDIUM
- [ ] Avatar model absent — `avatar_type` is a Required enum per AI.md
      59292-59293 and the profile response must carry the 256/128/64/32
      gravatar URL object (59314-59322, 59340-59342). The `users` DDL has
      only `avatar_url`; "gravatar" appears nowhere. — MEDIUM
- [ ] `src/server/handler/web.go:46-47` mounts only `/users/settings`; AI.md
      59357-59363 defines separate `/privacy`, `/notifications`,
      `/appearance` routes. — MEDIUM
- [ ] `user_preferences` is missing `email_mentions`, `email_updates`,
      `email_digest`, `push_enabled`, `push_mentions` (AI.md 59476-59482).
      `email_digest` has no storage at all, so digest frequency cannot be
      honored. — MEDIUM
- [ ] Absent auth routes: `/server/auth/username/forgot` (60088/60307),
      `POST /auth/refresh` (60310/59805), `/server/auth/invite/server/{token}`
      (60092/60313-60314). The user-invite half exists; the Server Admin half
      does not, so a second Server Admin cannot be onboarded by invite as
      AI.md 61503 requires. — MEDIUM
- [ ] `/config/roles` CRUD (59685-59703, 60418-60437) and
      `/config/users/invites` CRUD absent. Invite codes are the enforcement
      point for this project's default `invite` registration mode
      (`src/config/defaults.go:39`), which is therefore unusable from the
      admin panel. — MEDIUM
- [ ] Session/2FA paths diverge: code mounts `/users/sessions` and
      `/users/security/2fa` (+ a non-spec `/2fa/confirm`); AI.md 60351-60355
      specifies `/users/security/sessions` and `/2fa/enable` / `/2fa/disable`.
      — LOW
- [ ] `src/server/service/profile.go:73-80` omits `verified` (59301, 59346)
      and `created_at` (61645) from the public profile. — LOW
- [ ] Email templates `user_invite` and `user_account_disabled` missing from
      `src/notify/template.go:134-157` (AI.md 60599-60600). — LOW
- [ ] Cluster shared-DB `srv_*`/`usr_*` table prefixes (60743-60748),
      SQLite↔remote migration on driver change (60894-60911), and the
      `/config/nodes` tree with `node_{32}` join tokens (61441-61457) are all
      absent. Belongs with the PART 10 clustering work. — MEDIUM

### PART 35 — Organizations

- [ ] `org_preferences` table absent entirely (AI.md 62121-62123). — MEDIUM
- [ ] `GET /orgs/{slug}/roles` absent (60389-60392, 60133). Custom role
      creation is arguably out of scope since IDEA.md fixes the role set at
      owner/admin/editor/viewer, but the read-only listing has no such
      excuse. — MEDIUM
- [ ] Org security tree: only `/orgs/{slug}/audit` exists — wrong path, and
      missing `/security/audit/export` POST, `/export/{id}` GET,
      `/retention` GET/PATCH, `/security/sessions` GET (60393-60398). — MEDIUM
- [ ] No username-keyed per-member org profile route returning the
      visibility-filtered payload with `profile_visibility` (62201-62231).
      — MEDIUM

### PART 36 — Custom Domains

- [ ] `src/server/service/domain.go:470` `ResolveServableDomain` has **zero
      call sites** and no host-matching middleware is registered in
      `src/server/router.go`, so a verified custom domain never actually
      serves the org's content (AI.md 62672-62775). — HIGH
- [ ] `src/server/service/domain.go:381` `RenewCertificates` has **zero call
      sites**, and none of the three AI.md 63263-63285 scheduler tasks
      (`custom_domain_verification` `*/15 * * * *`,
      `custom_domain_ssl_renewal` `0 4 * * *` renew_before 7d,
      `custom_domain_cleanup` `0 5 * * *`) are registered. An issued
      custom-domain certificate silently expires. — HIGH
- [ ] The 13 domain error codes at AI.md 63309-63323 do not exist;
      `src/server/handler/handler.go:334-363` collapses every domain failure
      into the generic validation/not-found/forbidden envelope, so a client
      cannot distinguish TXT_RECORD_MISSING from DOMAIN_RESERVED. — MEDIUM
- [ ] `custom_domains` is missing `ssl_provider`, `ssl_credentials`,
      `ssl_cert_pem`, `ssl_key_pem` (62590-62651); wildcard domains are
      storable but not issuable without a DNS-01 path (63157-63172). — MEDIUM
- [ ] `custom_domain_audit` column is `event`; AI.md 62660 says `action`.
      — LOW

### PART 33 — Client & Agent

- [ ] Token source #5 (`{config_dir}/token`, AI.md 52785-52790) is not read
      by either binary. — HIGH
- [ ] `cli.yml` schema does not match AI.md 54830-54915, **including a wrong
      key name**: the code uses `server.url`, the spec key is
      `server.primary` — and `src/agent/config.go` already uses
      `Server.Primary` correctly, so the two config files disagree with each
      other. Also missing `server.cluster`, `api_version`, `admin_path`,
      `timeout`, `retry`, `retry_delay`, `auth.token_file`, `tui.*`,
      `output.*`, `logging.*`, `cache.*`. — HIGH
- [ ] `cli.yml` is never auto-created on first run (AI.md 55470);
      `SaveConfig` exists and is never called from `Run`. — MEDIUM
- [ ] Server-address fallback stops at `cfg.Server.URL`; AI.md 54967-54972
      requires `--server` → `server.primary` → `server.cluster` →
      compiled `{official_site}` → error. `OfficialSite` is stamped into
      `src/client/main.go` and never read for this. — MEDIUM
- [ ] `--color` is inert and `NO_COLOR`/`TERM=dumb` are honored in neither
      binary — zero `NO_COLOR`/`IsTerminal` references under `src/client` or
      `src/agent`, though `src/common/color` and `src/common/display` both
      implement it. AI.md 55076, 56682-56683, 56626. The TODO deferral covers
      only config-file precedence for an unset flag, not the flag doing
      nothing. — MEDIUM
- [ ] `--lang` is parsed in both binaries and never consumed. — MEDIUM
- [ ] `--user` smart detection (`@`/`+` prefix scope routing) parsed and
      never read. — MEDIUM
- [ ] Agent has no startup banner (AI.md 56587-56634, 56579), though
      `src/common/banner` exists and already handles NO_COLOR. — MEDIUM
- [ ] Documented agent env-var mapping unimplemented — AI.md 56997-57005
      requires `REDXT_AGENT_SERVER_PRIMARY`, `_HOSTNAME`,
      `_COLLECTION_INTERVAL`, `REDXT_DEBUG`; only `REDXT_AGENT_TOKEN` and
      `REDXT_TOKEN` are read. — MEDIUM
- [ ] Extended `--version` (server URL/version, Go/OS/arch, and the version-
      compatibility warning at AI.md 56308) absent. — LOW

### Docker compose template drift

- [ ] `docker/docker-compose.test.yml` deviates from AI.md 40844-40908 on six
      points: `restart: always` (spec `"no"`), `MODE: debug` (spec
      `development`), `container_name: redxt-test-app` (spec `redxt-test`),
      cache named `redxt-test-cache` with a persistent volume (spec
      `redxt-cache-test` with `tmpfs: - /data`), port `64582` (spec `64581`),
      network block missing `driver: bridge`. — HIGH
- [ ] `docker/docker-compose.dev.yml` deviates from AI.md 40702-40743 on four
      points: `container_name: redxt-dev-app` (spec `redxt-dev`),
      `hostname: redxt-dev` (spec `{project_name}`), `172.17.0.1:64581:80`
      (spec plain `"64580:80"`, explicitly no `172.17.0.1:` bind), and it
      includes a valkey service the spec says the dev file must not have.
      — HIGH

## Pass 6: Code Flow Trace

Covered inline above. The dead-call-target findings are the two PART 36
zero-call-site functions (`ResolveServableDomain`, `RenewCertificates`), the
zero-call-site `geoip.Service.Blocked`, and the ignored-parameter case in
`src/client/http.go`. Env-var completeness was closed by the
`docs/configuration.md` fix. No wrong-call-target or swapped-argument defects
were found.

---

## Verified clean — checked and NOT findings

Recorded so these are not re-audited from scratch.

- **Injection:** all SQL parameterized; zero `fmt.Sprintf` into a query.
- **XSS:** zero `template.HTML/JS/URL/CSS`, no `text/template` on the web
  path, zero `<script>` tags under `src/`.
- **Secrets:** no committed tokens, keys, or `.env` files anywhere; git
  history clean.
- **CSRF:** wired into the chain, enabled by default, HttpOnly double-submit,
  `SameSite=Strict`. No CORS wildcard-with-credentials.
- **Crypto:** Argon2id parameters correct (m=65536, t=3, p=4, keylen=32,
  saltlen=16, PHC v19); SHA-256 for tokens; `crypto/rand` for every security
  value; `crypto/sha1` appears only in the RFC 6238 TOTP implementation.
- **Archive safety:** Zip-Slip and tar-symlink both defended in
  `src/backup/archive.go` `safeJoin`.
- **Panics:** every `panic()` is an init-time `MustRegister` idiom.
- **Org authorization:** no IDOR anywhere in the org tree — all 19 handlers
  in `src/server/handler/org.go` route through `orgScope` →
  `AccessBySlug(ctx, slug, c.UserID)` → service `require(...)`; non-members
  get `ErrNotFound`, never `ErrForbidden`. `TransferOrg` correctly refuses
  non-owners, personal orgs, and self-transfer.
- **Domain authorization:** an unverified domain cannot get a certificate —
  `verifyDomain` compares the TXT record with `crypto/subtle` before writing
  `verified_at`, and `issueCertificate` independently re-asserts
  `if !domain.Verified()`. `domainForOrg` returns `ErrNotFound` for another
  org's domain id.
- **Account security:** password reset consumes the challenge before writing
  and then calls `DeleteUserSessions`; `currentUser` refuses a Server Admin
  credential on end-user routes.
- **Username validation:** `src/user/validate.go` + `blocklist.go` match AI.md
  61651-61702 exactly (2-39 chars, lowercase regex, no consecutive hyphens,
  reserved-name blocklist, shared user/org namespace).
- **Tokens:** all six prefixes at `src/security/token.go:38-50` match AI.md
  60034-60041; `api_tokens.last_used_at` present per 59640.
- **PART 28 CI/CD:** essentially perfect. All five workflows present; zero
  unpinned actions (every `uses:` a 40-char SHA); zero `make`; zero
  `setup-go`; zero `pull_request_target`, with a `workflow-policy` job that
  programmatically fails the build on either violation; least-privilege
  permissions; all three blocking security jobs plus trivy and a real 60%
  coverage gate; correct concurrency groups; 8-platform release matrix with
  SBOM, checksums, and provenance attestation.
- **PART 26 Makefile:** byte-for-byte identical to the AI.md 38825-39091
  template.
- **`docker/Dockerfile`, `docker/rootfs/…/entrypoint.sh`,
  `docker/docker-compose.yml` (prod), `.dockerignore`, `.gitignore`:** all
  exact template matches.
- **No TODO/FIXME/HACK/stub markers in committed code** — all 13 grep hits
  are prose references to `TODO.AI.md` in doc comments. Zero
  `panic("not implemented")`.
- **`SELECT *`:** zero occurrences. **`server.yaml`:** only in the
  auto-migration path, which is implemented.
- **PART 32 Tor/I2P:** Tor auto-enables with no toggle, I2P is opt-in only;
  torrc emits `ExitRelay 0`, `ExitPolicy reject *:*`, `ORPort 0`,
  `DirPort 0`, `VanguardsLiteEnabled 1`; 0700/0600 permissions enforced;
  supervisors mutex-guarded with no `Close()` deadlock.
- **`src/main.go:21` `OfficialSite = "redxt.us"`** was raised as a
  "guessed value" violation. It is **not** — IDEA.md:25 declares
  `official_site: redxt.us`, which is the sanctioned source for project
  variables. No change needed.
- **No production `.go` file outside `src/paths/` hardcodes an org/name
  path** — the grep hits are expected-value literals inside `_test.go`
  assertions, which is legitimate.
