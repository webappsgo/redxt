# TODO.AI.md

Implementation backlog for redxt, generated from AI.md PART 7-37 (PART 0-6
scaffolding and PART 26-28 Makefile/Docker/CI-CD are already complete and
verified — no items below for those). PART 37 is a read-only IDEA.md
reference, not an implementation task, and is excluded.

## Resolved dependency order

Foundation first, then layered by what each PART actually needs at runtime:

1. **PART 7-13 — binary requirements, server CLI, error handling/caching,
   database & cluster, security & logging, server config, health &
   versioning.** Nothing else can start before this: every later PART reads
   config through PART 12, persists through PART 10, and logs/authenticates
   through PART 11.
2. **PART 14 — API structure.** Needs 7-13 (config, db, security). Everything
   HTTP-facing (web, admin, client/agent, custom domains) is built on it.
3. **PART 15 — SSL/TLS & Let's Encrypt.** Needs 14 (serves the API/web over
   TLS) and 12 (cert paths/config).
4. **PART 34 — Multi-user.** Needs 10 (db), 11 (security/password hashing),
   14 (API). Must land before anything that gates behavior by user identity:
   35, 36, 16, 17, 18, 33.
5. **PART 35 — Organizations.** Needs 34 (users belong to orgs).
6. **PART 36 — Custom domains.** Needs 35 (domains are org-scoped) and 15
   (TLS for custom-domain vhosts).
7. **PART 16 — Web frontend.** Needs 14, 15, 34 (session/login UI).
8. **PART 17 — Admin panel.** Needs 16 (shares frontend framework), 34, 35
   (admin manages users/orgs).
9. **PART 18 — Email & notifications.** Needs 34 (user accounts to notify)
   and 12 (SMTP config). Routed to `notifications-builder`.
10. **PART 33 — Client & agent.** Needs 14 (API it talks to), 11 (agent
    token security model), 34/35 (agent tokens are user/org scoped per
    IDEA.md's `adm_agt_`/`usr_agt_`/`org_agt_` token table).
11. **PART 19-25, 29-32** — scheduler, GeoIP, metrics, backup/restore,
    update command, privilege escalation/service, service support, testing,
    docs, i18n/a11y, overlay networks (Tor/I2P). Each depends only on the
    7-13 foundation plus its own named PART; ordered below by PART number
    as the tiebreaker since none blocks another in this group.

---

## Foundation (PART 7-13)

## [x] Implement binary requirements (single self-contained binary, build/version embedding)
Read: AI.md PART 7

## [x] Implement server binary CLI (flags, subcommands — including --address/--port already wired in entrypoint.sh but not yet in src/main.go)
Read: AI.md PART 8

## [x] Implement error handling & caching
Read: AI.md PART 9

## [x] Implement database layer & clustering
Read: AI.md PART 10

## [x] Implement security & logging (authN primitives, structured logging, secret handling)
Read: AI.md PART 11

## [x] Implement server configuration (config file, env vars, precedence)
Read: AI.md PART 12

## [x] Implement health checks & versioning
Read: AI.md PART 13

## API and transport layer

## [x] Implement API structure (REST endpoints, routing, middleware)
Read: AI.md PART 14

## [x] Implement SSL/TLS & Let's Encrypt (ACME, cert storage, renewal)
Read: AI.md PART 15

## [ ] Schedule the daily 03:00 certificate renewal check
PART 15 requires app-managed certificates to renew 7 days before expiry on a
daily 03:00 check. `ssl.Manager.RenewAll`, `ssl.RenewalWindow`, `ssl.RenewalHour`
and `ssl.NextRenewalCheck` exist and are tested; nothing calls them yet because
the scheduler is PART 19. Wire the job when PART 19 lands.
Read: AI.md PART 15, PART 19

## [ ] Mount the metrics, swagger, and GraphQL handlers on the router
`server.Routes` has a field per surface and the router mounts every documented
path (including the unversioned aliases) the moment a field is non-nil. The
metrics/swagger/GraphQL fields are left nil until PART 21 supplies those
handlers, so those routes answer 404 rather than an empty success. The admin
field is now supplied (`startup/admin.go` builds `admin.Handler`, wired via
`mergeAdminRoutes` in `startup/http.go`) — see "Implement admin panel" below
for what that handler does and does not yet cover.
Read: AI.md PART 14, PART 21

## [ ] Add the `server.metrics.root.enabled` config key
The router mounts the root `/metrics` alias unconditionally when a metrics
handler is supplied, but PART 21 describes the root alias as opt-in the way
`server.healthz.root.enabled` is. Add the matching key and gate the alias on it
when PART 21 is implemented.
Read: AI.md PART 21

## [ ] Publish the client version floor in the autodiscovery document
PART 33 describes `cli_versions` and `cli_min_version` in the autodiscovery
payload so a client can tell whether it must upgrade. `server.Autodiscovery`
omits both because no client release channel exists yet.
Read: AI.md PART 23, PART 33

## Accounts, orgs, and domains — route to go-auth-builder

## [x] Implement multi-user (registration modes, MFA, roles, tokens per IDEA.md Roles & permissions table)
Read: AI.md PART 34
Builder: go-auth-builder

## [x] Implement organizations (personal + shared orgs, owner/role permissions, per-zone isolation)
Read: AI.md PART 35
Builder: go-auth-builder

## [x] Implement custom domains (white-labeling, org-scoped custom vhosts)
Read: AI.md PART 36
Builder: go-auth-builder

## Frontend and admin

## [x] Implement web frontend
Read: AI.md PART 16

## [ ] Implement admin panel
Read: AI.md PART 17
`src/server/admin` (service, store, model from an earlier pass; handler.go
added this pass) plus `src/server/handler/web.go`'s shared-login fallback now
cover the panel's entry points: the first-run setup wizard
(`GET/POST {admin_path}/config/setup`), the isolation-rule landing dashboard
(`GET {admin_path}/`), and logout (`POST {admin_path}/logout`). Sign-in itself
deliberately has no separate admin route — PART 17 requires the same shared
`/server/auth/login` form Regular Users use, with no admin-specific hint, so
`webLogin` tries the Regular User service first and falls back to
`admin.Service.Login` on failure.
Still not implemented — tracked here rather than stubbed, per PART 1:
- The entire `{admin_path}/config/*` server-management surface: settings,
  ssl, email, scheduler, logs (+ logs/audit), backup, updates, info, metrics,
  network (tor, geoip), security (auth incl. oidc/ldap/saml, tokens,
  firewall), users, orgs, cluster, agents.
- The `{admin_path}/{admin_username}/*` own-account tree: profile,
  preferences, notifications.

## Notifications — route to notifications-builder

## [ ] Implement email & notifications
Read: AI.md PART 18
Builder: notifications-builder

## Client and agent

## [ ] Implement client & agent (adm_agt_/usr_agt_/org_agt_ token enrollment and data-plane sync)
Read: AI.md PART 33

## Operational and platform PARTs (no cross-dependencies beyond the foundation)

## [ ] Implement scheduler
Read: AI.md PART 19

## [ ] Implement GeoIP
Read: AI.md PART 20

## [ ] Implement metrics
Read: AI.md PART 21

## [ ] Implement backup & restore
Read: AI.md PART 22

## [ ] Implement update command
Read: AI.md PART 23

## [ ] Implement privilege escalation & service integration
Read: AI.md PART 24

## [ ] Implement service support
Read: AI.md PART 25

## [ ] Implement testing & development tooling
Read: AI.md PART 29

## [ ] Implement ReadTheDocs documentation
Read: AI.md PART 30

## [ ] Implement i18n & a11y
Read: AI.md PART 31

## [ ] Implement overlay networks (Tor & I2P)
Read: AI.md PART 32

## [ ] Reconcile PART 36 custom-domain routes with the spec tables
Read: AI.md PART 36 (route tables, approx. lines 62775-62849)
The implemented org-scoped domain tree differs from the spec's tables in
five ways. Each is a deliberate, recorded decision, not an oversight:
- Domains are addressed by `{domain_id}`; the spec addresses them by
  `{domain}` name. An opaque id keeps a tenant-owned name out of the path.
- The ownership-challenge route is `/verification`; the spec names it
  `/dns`. Decide which wins before the route is public.
- `GET /ssl`, `POST /ssl`, and `POST /ssl/renew` are not implemented.
  Issuance currently happens as a consequence of successful verification
  through the PART 15 ACME manager, with no separate certificate routes.
- The spec's parallel `/users/domains/...` user-scope tree is absent. A
  domain always belongs to an organization; a single user reaches theirs
  through their personal organization.
- `/suspend` lives in the org tree and there is no `/unsuspend`; the spec
  places both in the admin tree. Unsuspend has no route at all today.

## [ ] Implement the GDPR data routes
Read: AI.md lines 17491-17495
`/users/data/export`, `/users/data/delete`, and `/users/consents` are
specified but not built. They are absent rather than stubbed, so nothing
answers on those paths today.

## [ ] Implement external identity providers
Read: AI.md PART 34
OIDC, LDAP, SAML, and passkey/WebAuthn sign-in are not built. Local
Argon2id credentials and TOTP are the only authentication methods.

## [ ] Implement DNS credential types on the token scope column
Read: AI.md PART 34, PART 35
The token table already carries `scope`, `zone_id`, and `capability`
columns so TSIG, GSS-TSIG, DDNS, and agent credentials can attach to a
zone later without a migration. No such credential type exists yet, and
no code path issues or accepts one.

## [ ] Raise coverage on packages below the 60% gate
Read: AI.md PART 29
As of the PART 16/17 admin-panel pass, `go test ./... -cover` shows several
pre-existing packages under the required 60% gate:
`src/server/handler` 30.7%, `src/server/service` 42.9%, `src/server/store`
40.4%, `src/daemon` 19.1%. None of these regressed this pass — `handler`
gained two new tests covering the admin-login fallback added this pass
(`web_admin_test.go`) without materially moving its percentage, since the
package is large and mostly covers pre-existing PART 34-36 surface. Add
table-driven tests until each package clears 60%.

## [ ] Add /server/about and /server/help pages sourced from IDEA.md
Read: AI.md PART 1 ("Create /server/about or /server/help with placeholder
text" is forbidden — content must come from IDEA.md), AI.md PART 16
No `about`/`help` route exists yet (`grep` of `src/server/handler` and
`src/server/router.go` finds neither). Out of scope for the PART 16/17
admin-panel pass just completed; needs real copy sourced from IDEA.md, not
placeholder text.

## [ ] Add a workflow_dispatch trigger to ci.yml
Read: .github/workflows/ci.yml, ci.yml's `on:` block currently only has
push/pull_request/schedule. After pushing commit c9f910d5fb52 (PART
34-36), CI never got a run created at all and Docker Build got stuck
`queued` with zero job records for over an hour — GitHub refused both
cancel (409 "not queued yet") and delete (403) on the orphaned run.
Daily Build (which does have workflow_dispatch) was manually dispatched
as a substitute signal and returned success for the same commit. Adding
workflow_dispatch to ci.yml would let a missed/stuck push-triggered run
be manually retried in future without relying on an unrelated workflow.

## [ ] PART 18: admin_notifications / admin_notification_prefs schema
Read: AI.md PART 18 ("WebUI Notification System"), src/database/schema_server.go
The PART 18 config layer and `src/notify` (SMTP autodetect/send, embedded
email templates, `{variable}` template engine, validation) are implemented
and tested. The server.db side of the WebUI notification system is not:
add `admin_notifications` (id, type, title, message, link, read, created_at)
and `admin_notification_prefs` tables to `schema_server.go`, following the
existing table-creation pattern; do not touch the existing
`notification_channels` table.

## [ ] PART 18: user_notifications schema + notification_prefs column
Read: AI.md PART 18, src/database/schema_users.go
Add a `user_notifications` table (same shape as `admin_notifications`, but
in users.db — never merged with the admin table) to `schema_users.go`, and
add a `notification_prefs` column to `user_preferences` via an idempotent
`ALTER TABLE ... ADD COLUMN` in `usersCoreUpdates`, matching the existing
additive-migration pattern in that file.

## [ ] PART 18: store/service layer for WebUI notifications
Read: AI.md PART 18, src/server/store/preferences.go, src/server/service/service.go
Once the schema above exists, add store methods (create, list paginated
with `?unread=true` filter, mark-one-read, mark-all-read, unread count) and
a `Service` wrapper following the existing `Service`/`Options`/`New`/
`mapStoreErr`/`fieldError` pattern. Apply retention (30 days / max 100
rows) via purge-on-write, since no scheduler exists yet — do not add a fake
cron job to do this; PART 19 (Scheduler) is the correct home for a
periodic purge once it is built.

## [ ] PART 18: Notify() decision engine
Read: AI.md PART 18 ("Notification Preferences")
Add a single entry point, e.g. `func (s *Service) Notify(ctx, event string,
recipient Recipient, vars map[string]string) error`, that always writes a
WebUI row (respecting the recipient's webui preference and per-category
defaults) and additionally sends email via `notify.Send` when SMTP is
enabled and that event's email preference is true. The four
Security-category events (`login_alert`, `security_alert`,
`password_changed`, `token_regenerated`) must never be user-disableable —
enforce this in code, not only by omitting a UI toggle.

## [ ] PART 18: wire real notification trigger call sites
Read: AI.md PART 18, src/server/service/*.go, src/server/admin/
Once `Notify()` exists, call it from the real actions that already exist:
login (login_alert), password change (password_changed), 2FA
enable/disable, API token regeneration, admin `CompleteSetup` (welcome
email — see `src/server/admin/service.go`). For events with no existing
call site yet (`backup_complete`/`backup_failed`, `ssl_expiring`/
`ssl_renewed`/`ssl_renewal_failed`, `scheduler_error`, `startup`/
`shutdown`, `update_available`/`update_installed`), do not fake a call
site — wire each one when its owning feature (PART 19 scheduler, PART 22
backup, PART 23 update, PART 15 SSL renewal) is actually built.

## [ ] PART 18: admin panel routes (template editor, notification center)
Read: AI.md PART 18 ("Admin Panel"), .claude/rules/frontend-rules.md
Add `/server/{admin_path}/...` routes for the email-template editor
(list/edit/reset, using `notify.Load`/`SaveOverride`/`ResetOverride`/
`Validate`) and an admin notification center (bell icon, polling — not
WebSocket; this is a deliberate simplification and should stay documented
here, not silently upgraded without discussion). Audit-log template edits,
resets, and test-email sends via the existing `audit_log` table/
`src/server/store/audit.go` pattern.

## [ ] PART 18: user panel routes (notification preferences + center)
Read: AI.md PART 18, .claude/rules/optional-rules.md (PART 34)
Add `/users/settings/notifications` (preferences) and a user notification
center, reusing the same store/service pattern as the admin side but
against the separate `user_notifications` table/users.db connection —
never merge with the admin notification tables.
