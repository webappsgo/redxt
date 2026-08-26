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

## [ ] Implement API structure (REST endpoints, routing, middleware)
Read: AI.md PART 14

## [ ] Implement SSL/TLS & Let's Encrypt (ACME, cert storage, renewal)
Read: AI.md PART 15

## Accounts, orgs, and domains — route to go-auth-builder

## [ ] Implement multi-user (registration modes, MFA, roles, tokens per IDEA.md Roles & permissions table)
Read: AI.md PART 34
Builder: go-auth-builder

## [ ] Implement organizations (personal + shared orgs, owner/role permissions, per-zone isolation)
Read: AI.md PART 35
Builder: go-auth-builder

## [ ] Implement custom domains (white-labeling, org-scoped custom vhosts)
Read: AI.md PART 36
Builder: go-auth-builder

## Frontend and admin

## [ ] Implement web frontend
Read: AI.md PART 16

## [ ] Implement admin panel
Read: AI.md PART 17

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
