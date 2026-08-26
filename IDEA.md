
# redxt — The Complete DNS Server

## Project description

**redxt** is a single-binary, fully RFC-compliant DNS platform that replaces BIND/named, Unbound, Pi-hole, Technitium, AdGuard Home, acme-dns, and hosted services like DuckDNS, No-IP, DynDNS, FreeDNS, and redirect.center with one self-hosted application. It is authoritative server, recursive resolver, forwarder, DNS firewall, hosted dynamic-DNS provider, HTTP redirect engine, and DNS-distributed data index in a single process — clustered, encrypted, and observable by default. The former **cvedex** project is fully absorbed into redxt as the flagship dataset of its data-zones engine; cvedex no longer exists as a separate application.

It targets self-hosters, SMBs, and enterprises who want one DNS daemon for everything, deployable by users with low assumed technical knowledge, with sane defaults and full customizability. ISC BIND formats (named.conf, master zone files) are the canonical interchange standard: a working named deployment migrates with zero hand-editing.

One binary. Every DNS role. Zero external dependencies.

**Reference deployment**: the official site and flagship public instance run at **redxt.us** (serving `redxt.us` and `*.redxt.us`); **ns1.redxt.us** and **ns2.redxt.us** are cluster nodes. The public DDNS provider, redirect engine, and data zones (including cvedex) operate under this domain.

**Origin note**: this project was formed by merging the `casns` project's goals/features/spec into `redxt` (this project keeps `redxt`'s name, org, and `official_site`; `casns`'s business logic became this spec). No code was carried over — redxt is implemented from scratch under this AI.md. The `casns` directory and its GitHub repository are retired once this merge is verified.

## Project variables

project_name:    redxt
project_org:     webappsgo
# FROZEN — equals project_name on first install, never changes
internal_name:   redxt
# FROZEN — equals project_org
internal_org:    webappsgo
app_name:        redxt
official_site:   redxt.us
license:         MIT
language:        go
maintainer_name: webappsgo
maintainer_email: git-admin@casjaysdev.pro

## Business logic

### Product scope & non-goals

**Scope: DNS only.** Every DNS role in one concurrent process — authoritative (primary/secondary), recursive resolver, forwarder, hybrid/split-horizon with views, DNS firewall/blocker, hosted dynamic-DNS provider, HTTP redirect engine, RDAP/WHOIS responder, and data-zone publisher — plus the clustering, agents, security, and management surfaces those roles need. A Terraform provider is a first-class deliverable from v1.

**Non-goals (explicit):**

- **Not a DHCP server** — a separate future casapps Go DHCP project integrates through documented hooks (see Trust boundaries).
- **Not an mDNS/LLMNR reflector** — local discovery is out of scope.
- **Not an NTP server, registrar, CDN, reverse proxy, mail server, or packet firewall** — redxt hosts the records (MX, SPF, DKIM, DMARC, TLSA, CAA) and filters DNS answers, nothing else. HTTP serving is limited to the surfaces defined in "HTTP surface map".
- **Not a downstream DNS load balancer (dnsdist)** — balancing across upstreams is the forwarder policy engine; balancing across redxt instances is clustering/agents; fronting third-party DNS pools is explicitly rejected. The supported pattern for legacy servers is migration (AXFR them in, retire them).
- **No plugin/app runtime** — extensibility is built in: data zones, RPZ, REST API, webhooks, multi-channel notifications.
- **No org change-approval workflow** — versioning, audit trail, and role separation cover shared-org safety.
- **No AI/ML** — rule-based logic only (including the tunneling-detection rules).
- **No telemetry, no cloud dependency, no monetization, no feature tiers** — every feature for every deployment size.
- **Not a byte-for-byte redirect.center/txtdirect clone** — redxt's redirect engine (see "Server roles") is a from-scratch, RFC-compliant design that natively supports both projects' DNS record schemes (CNAME-encoded and TXT-directive, researched from their public source); txtdirect's `proxy` record type (real reverse-proxying) is excluded per the reverse-proxy non-goal above.

### Roles & permissions

redxt enables **PART 34 (Multi-User)** and **PART 35 (Organizations)** as defined in AI.md. **Registration mode default: `invite`**; instance admins may switch to `open`, `admin_only`, or `disabled` per spec. **PART 36 (Custom Domains)** and spec white-labeling (cosmetic-only branding) are enabled.

Every zone, policy, key, and DDNS host belongs to an organization; every user gets a personal org and may belong to shared orgs. Isolation is enforced server-side. Account mechanics (registration, invites, MFA, admin powers and limits, token formats) follow PARTs 17/34/35 — IDEA.md defines only the DNS-specific semantics:

| Org role | DNS-specific permissions |
|---|---|
| **Owner** | Everything in the org: zones, records, TSIG keys, tokens, DDNS hosts, policies, templates, git sync settings, custom domains; transfer/delete org |
| **Admin** | Manage zones, records, keys, tokens, DDNS hosts, org policies; manage members below admin; no org deletion/transfer |
| **Editor** | Create/edit records, DDNS hosts, and scheduled changes in assigned zones; no key export, no member or zone-lifecycle management |
| **Viewer** | Read-only zones, records, and org-scoped analytics |

| Credential | Scope |
|---|---|
| **API token** (spec PART 11/14 formats) | Scoped to one org or narrower (one zone, one capability — `records:write`, `acme-challenge`, `metrics:read`); never exceeds the issuer's role; consumed by UI, CLI, agents, and the Terraform provider |
| **TSIG key** | Belongs to an org; per-key per-zone grants (update, transfer, notify) with BIND `update-policy` grant semantics; ACME-scoped keys restricted to `_acme-challenge` names |
| **GSS-TSIG identity** (RFC 3645) | Maps an AD machine/service identity to scoped update grants, for secure dynamic updates from domain-joined Windows clients |
| **DDNS host token** | Belongs to a user within an org; updates only its own host(s) |
| **Agent token** (`adm_agt_`/`usr_agt_`/`org_agt_` per spec) | Data-plane enrollment only; no management authority |

Instance admin, cluster nodes, and anonymous resolver clients behave per spec PARTs 17/10 and the ACL/rate-limit rules below.

### Data model & sensitivity

| Data | Sensitivity | Notes |
|---|---|---|
| DNSSEC private keys (KSK/ZSK/CSK) | **Critical** | Encrypted at rest; exportable only by org owners/instance admin; included only in encrypted backups |
| TSIG secrets, API/DDNS/agent tokens, GSS-TSIG material | **Critical** | Hashed where verification-only suffices, encrypted where the secret is needed; revocable; never logged |
| User credentials / MFA secrets | **Critical** | Per spec (Argon2id, encrypted TOTP) |
| Query logs | **High (PII)** | Client IP + names reveal behavior; retention policies, IP-truncation anonymization, purge on demand; org members see analytics only for their org's zones; instance-wide logs are instance-admin only |
| Git sync credentials (deploy keys/tokens) | **High** | Encrypted at rest; push-only scope recommended in docs |
| Zone data & records | Medium | Public by DNS nature; enumeration still resisted (NSEC3, ACL'd transfers) |
| Config, ACLs, policies, templates, blocklist subscriptions | Medium | Versioned with diff/rollback; append-only changelog |
| Metrics | Low | Aggregated; no per-client identifiers in Prometheus output by default |

Storage per spec PART 10: SQLite default, optional PostgreSQL/MariaDB, idempotent self-creating schema, database-driven configuration, no config files.

### Trust boundaries & external services

All external dependencies are optional, fail safe, and have stated trust assumptions:

| External | Trust assumption | Failure mode |
|---|---|---|
| Upstream forwarders (built-in catalog + custom) | Untrusted for integrity unless DNSSEC-validated; trusted only to answer | Health probes demote; rollover to the user's secondary; serve-stale; if both down, fall back to full recursion |
| Root/authoritative internet DNS (recursion) | Untrusted; DNSSEC validation enforces integrity | SERVFAIL with RFC 8914 extended error on validation failure |
| Blocklist/RPZ feed URLs | **Untrusted input**: defensive parsing, size caps, checksums where offered; a feed can only block/rewrite within its declared policy scope, never add authoritative data | Keep last good list + notify; skip and report malformed entries |
| Data-zone sources (CI-processed GitHub Releases, e.g. cvedex) | Trusted only when release signature/checksum verifies | Keep last good dataset; alert |
| Tor daemon / network | Transport for `.onion`/`.exit` only (RFC 7686); DNSSEC validation auto-excepted for these zones | Tor down → NXDOMAIN/SERVFAIL for onion only |
| Outbound proxy (SOCKS5/HTTP; Tor is a special case) | Transport only; admin-configured | Proxy down → per-policy: fail closed (privacy) or fall back direct (availability), admin's choice |
| ACME CA | Cert issuance only | Renewal failure → notify + self-signed fallback; never blocks DNS |
| Git remotes (zone sync via embedded pure-Go git — **no git binary dependency ever**) | Push-only escrow target; remote treated as untrusted storage | Push failure → retry + notify; local history remains authoritative |
| RDAP registries (domain-expiry monitoring) | Informational only | Lookup failure → monitoring gap notification, no serving impact |
| S3-compatible backup targets | Untrusted storage; client-side encryption before upload | Backup failure → notify; serving unaffected |
| Webhooks/notification channels | Receive-only sinks | Retry with backoff, then log |
| Future casapps DHCP server | Authenticated peer via RFC 2136+TSIG and/or lease-event API with scoped token; leases become A/AAAA/PTR with expiry cleanup | DHCP down → records age out per policy |
| AD/Kerberos infrastructure (GSS-TSIG) | Trusted only for the identities it asserts on update messages | KDC unreachable → GSS-TSIG updates fail closed; everything else unaffected |
| GeoIP database (optional, local file) | Informational | Missing → geo features disabled |
| Peer cluster nodes / agents | Per spec PART 10/33/34 join, secret distribution, and heartbeat rules | Per spec split-brain/majority rules; query serving continues |
| Terraform provider | A privileged API client; trusts redxt's API, authenticated by scoped org tokens | API errors surface as Terraform failures; no special server-side trust |

Inbound DNS queries, DNS UPDATE messages, provider-API calls, encrypted-transport connections, and transfer requests are all **untrusted** and pass ACL, auth (TSIG/GSS-TSIG/token), validation, and rate limiting. The management plane trusts only authenticated sessions/tokens per spec.

### Threat model & abuse cases

**Primary assets**: DNSSEC private keys; TSIG/API/DDNS/agent secrets; zone integrity (the answers users receive); query-log privacy; service availability; and the host's reputation (not becoming abuse infrastructure on a public instance like redxt.us).

**Attacker goals**: poison or forge answers, use the server for DDoS reflection, enumerate or steal zones, hijack dynamic hosts, cross org boundaries, exfiltrate query logs, take over the management plane, tunnel data through the resolver, or abuse self-service DDNS/redirects for phishing and C2.

| Threat / abuse case | Defense |
|---|---|
| Cache poisoning / off-path spoofing | Source-port + QID randomization, DNS cookies (RFC 7873), 0x20 encoding, DNSSEC validation on by default, QNAME minimization |
| Open-resolver amplification/reflection | **Recursion restricted to trusted ACLs (RFC 1918 + configured) by default** — opening it is an explicit admin act past a warning; RRL on authoritative answers; minimal ANY (RFC 8482); UDP size limits; per-IP/subnet throttles |
| Zone enumeration / theft | NSEC3, transfers refused without TSIG/ACL grant, transfer attempts logged + alertable |
| Forged dynamic updates | RFC 2136 requires TSIG (HMAC-SHA512 default) or GSS-TSIG; per-key/per-identity update-policy scoping; update forwarding only to the primary over authenticated channels |
| DDNS provider abuse (phishing hosts, C2, squatting) | Registration mode (invite default), per-user quotas, two-tier reserved-name lists, token revocation, stale-host expiry, per-host audit history, suspend host/user admin workflow |
| Redirect/parking abuse (open-redirect phishing) | Redirects/parking only for names the deployment is authoritative for; per-rule audit; configurable deny-listed target patterns |
| Cross-tenant access / privilege escalation between orgs | Server-side org isolation on every path (zones, records, keys, logs, analytics, git sync, custom domains); tokens capped at issuer's role; IDOR-resistant addressing; membership changes audited |
| Zone squatting between orgs | Zone uniqueness instance-wide; **TXT-challenge domain-ownership verification before activation, on by default**; instance-admin arbitration tooling |
| DNS tunneling / exfiltration through the resolver | Rule-based detection (label entropy, query length, NXDOMAIN floods, unique-subdomain rate) with alerting and optional per-client throttle actions; no ML |
| Malicious blocklist/RPZ feed | Scope-capped feeds, size limits, parse hardening (see Trust boundaries) |
| Credential stuffing / brute force | Per spec: Argon2id, rate-limited login, lockout, optional MFA, auth audit events |
| Token/key theft | Least-privilege scopes, rotation/revocation everywhere, secrets never logged, `_acme-challenge`-only ACME keys |
| Query-log exfiltration / privacy | Local-only logs, retention + anonymization, role-gated org scoping, encrypted backups; outbound ECS off by default |
| Rogue cluster join / agent impersonation | Spec PART 10/33/34 join flow, token types, heartbeat revocation, removed-node cleanup rules |
| Cache snooping | Cache answered only for clients allowed recursion |
| Resource exhaustion | Per-zone/global limits, streaming zone parsing, bounded cache, transfer backpressure, registration rate limits per spec |
| SSRF via admin-supplied URLs (feeds, webhooks, git remotes, backups) | Outbound restricted to admin-entered destinations, no redirects into link-local/metadata ranges, scheme allowlist |

**Explicit non-goal threats**: a fully compromised host OS, malicious instance admins, and registrar/registry-level hijacks (CDS/CDNSKEY tooling and expiry monitoring reduce exposure; the registrar relationship belongs to the operator).

### Security decisions & exceptions

1. **Plaintext Do53 stays enabled** — protocol requirement; DoT/DoH/DoQ (and DNSCrypt) are on by default and advertised via DDR/SVCB so capable clients upgrade.
2. **HMAC-MD5 TSIG accepted** — legacy interop for imported BIND deployments; flagged deprecated, excluded from key generation; default HMAC-SHA512.
3. **GSS-TSIG supported** — accepts Kerberos-authenticated updates from AD environments; fails closed when the KDC is unreachable.
4. **DNSCrypt supported** — legacy encrypted transport kept for dnscrypt-proxy ecosystems, as listener and upstream transport; DoT/DoH/DoQ remain the recommended path.
5. **Operator may open recursion to `any`** — supported (intentional public resolvers) behind an explicit warning; RRL and throttles remain active.
6. **Registration default is `invite`** — instance admin may open or close it per spec PART 34; compensating controls (quotas, reserved names, verification, abuse tooling) apply in all modes.
7. **Agents are data-plane only** — no management listener; **agent DoH is off by default**, and when enabled it is a stripped listener serving only `/dns-query` (no routes, UI, or API).
8. **DNSSEC validation exceptions** — `onion`/`exit` auto-excepted (RFC 7686); admin-added negative trust anchors / validate-except entries are logged and visible.
9. **Built-in ACME client makes outbound CA connections** — only for the server's own certificates; can be disabled in favor of supplied certs.
10. **Admin-configured remote fetches** (feeds, data zones, GeoIP, git, backups) are core functionality, governed by the SSRF restrictions above.
11. **Cluster writes require quorum** per spec — management-plane availability is sacrificed in minority partitions; query serving continues.
12. **Query path is stateless per node** — no node-pinned query state, so cluster nodes and agents are anycast-safe by design.
13. **DNSSEC is required, not optional** — every domain a user/org adds is signed by default (RFC 4033–4035); this is a stronger stance than "supported," reflecting that an RFC-noncompliant DNS response or an unauthenticated dynamic update is treated as a security bug, not a missing feature.
14. **named.conf/zone files are a live, bidirectionally-synced exception to "database-driven config, no config files"** — the on-disk files are real, watched, writable config surfaces (not a one-shot import), because BIND-config-file parity is a stated primary use case; conflicting concurrent disk/DB edits are surfaced for review rather than silently resolved either direction.

---

### Server roles

All roles run concurrently in one process; every zone and listener chooses its behavior independently.

- **Authoritative** — primary/secondary hosting for any number of zones with full transfer, NOTIFY, and dynamic-update support.
- **Recursive resolver** — full iterative resolution from root hints with DNSSEC validation, QNAME minimization, aggressive negative caching (RFC 8198), serve-stale (RFC 8767), **cache prefetch** of popular expiring entries, **DNS64** (RFC 6147), and **hyperlocal root** (RFC 8806) serving the root zone locally.
- **Forwarder** — see "Forwarders & upstream policy".
- **Hybrid / split-horizon with views** — per-client-ACL zone sets and per-zone view answers; local zones authoritative, the rest recursed or forwarded.
- **Redirect engine** — replaces both redirect.center and txtdirect by supporting each project's DNS record scheme natively, researched from their public source (`github.com/udleinati/redirect.center`, `github.com/txtdirect/txtdirect`), served by the built-in HTTP listener:
  - **CNAME-encoded mode (redirect.center-compatible)**: the destination is DNS-only — the client CNAMEs a hostname to `<encoded-labels>.<redxt redirect domain>`. redxt resolves that CNAME target's own label chain to recover the destination: plain labels concatenate into the destination host; reserved option labels (`_https`/`opts-https`, `_path-<base32>`/`opts-path-<base32>`, `_query-<base32>`/`opts-query-<base32>`, `_statuscode-301|302|307|308`, `_port-<n>`, `_uri`/`opts-uri` to pass through the original request path+query, `_slash`/`opts-slash`) are stripped and decoded (base32 for arbitrary path/query bytes) to build protocol, path, query, port, and status code. No TXT record involved.
  - **TXT-directive mode (txtdirect-compatible)**: a semicolon-separated `key=value` TXT record at the apex or a `_`-prefixed wildcard subzone (txtdirect's `_redirect` basezone convention), e.g. `v=txtv0; to=https://example.com; code=302`. Recognized keys: `v=` (version tag, required), `to=` (target URL, placeholder-expanded), `code=` (HTTP status, default 302), `type=` (`host` default; `path` for regex/path-segment routing via `re=`/`from=`; `gometa` to serve a `<meta name="go-import">` vanity-import-path response for `go get`; `dockerv2` to answer Docker Registry v2 API requests with a redirect to the real registry — both `gometa` and `dockerv2` are lookup-and-answer, not proxying, so they reuse the same record-resolution path as `host`/`path`), `ref=` (inject `Referer` header), `root=`, `vcs=` (VCS hint for `gometa`), `website=`, `use=` (chain to an upstream `_redirect.` zone for shared/reusable rule sets), and `>Header=value` for custom response headers. 255-byte TXT value cap enforced per spec.
  - **Excluded**: txtdirect's `proxy` type (real reverse-proxying — persistent upstream connections, bandwidth passthrough, SSRF-style upstream trust) is a categorically different feature from DNS-driven redirects/answers and is left to dedicated reverse-proxy tools (Caddy, nginx, Traefik); revisit only if a concrete need arises.
  - **Shared behavior across modes**: self-redirect loop detection, a configurable path/host blocklist (redirect.center's "guardian" pattern generalized into the existing filtering/policy engine and per-org reserved-name rules), fallback-on-parse-error to a configured default rather than a hard error, and DNS-level answer rewrites (NXDOMAIN rewrites, answer overrides) as a third, non-HTTP redirect primitive.
  - **Parking**: per-hostname customizable parking pages for DDNS hosts (No-IP offline-mode equivalent), independent of which redirect mode created the record.
  - **Precedence**: CNAME-encoded mode wins when a name has both, reflecting redirect.center as the primary replacement target (BIND + redirect.center is the stated primary use case); the TXT directive is only consulted when no CNAME-encoded target resolves.
- **Data zones** — engine for serving curated datasets as DNS zones from CI-processed releases, DNSSEC-signed, with HTTP 302 redirector and JSON gateway for reverse lookup and full-text search. **cvedex is the flagship dataset**: CVE IDs as `YYYY-NNNNN.<zone>` labels (default zone `cve.`); future datasets (MAC vendors, ASN, TLD metadata) ride the same engine with no new code paths.
- **Tor** — `.onion` resolution (and `.exit` handling) per RFC 7686, optional resolution egress through Tor, optional exposure of the server (DNS + UI) as a hidden service; a special case of the generalized outbound proxy support (SOCKS5/HTTP).

### Records & zone features

- **Record types**: all standard types plus **ALIAS/ANAME** (apex CNAME flattening as a first-class type), generic RFC 3597 unknown-type handling, SVCB/HTTPS (RFC 9460), TLSA/SSHFP/CAA/CERT/SMIMEA.
- **Wildcard records**: standard DNS wildcard hostnames (e.g. `*.example.com`) are a required, first-class capability — not an add-on.
- **Health-checked failover records**: answer sets resolve only to monitored-healthy targets (TCP/HTTP/ICMP probes), with **weighted round-robin** and **geo-aware** answer selection (GeoIP optional).
- **Zone templates**: create zones from blueprints with variables (SOA conventions, NS/mail/SPF/DKIM/DMARC/CAA blocks, etc.); templates are org-scoped with instance-level defaults.
- **Automatic PTR management**: reverse zones and PTR records created/maintained from forward records, including **classless reverse delegation (RFC 2317)** for sub-/24 allocations.
- **Local quick records**: Pi-hole-style host→IP and local-CNAME entries without creating a full zone.
- **Scheduled record changes**: queue changes for a maintenance window with optional auto-revert after a set duration.
- **SOA serial policy** per zone: date-based `YYYYMMDDnn` (default), unixtime, or plain increment — always auto-managed.
- **IDN**: Unicode names throughout UI/API, punycode on the wire.
- **Zone versioning**: full history with diff and rollback; **git-backed zone sync** optionally commits/pushes rendered BIND-format zone files to a remote on every change, implemented with an embedded pure-Go git library — never requires a git binary on the host.

### ISC BIND compatibility & migration

- **Zone files**: full RFC 1035 master-file syntax — `$ORIGIN`, `$TTL`, `$INCLUDE`, `$GENERATE`, multi-line parentheses, multi-string TXT, comments preserved on round-trip, wildcards, RFC 3597 generic syntax. Import and export.
- **Root hints file** (`named.root`/`root.hints`, RFC 8109 priming): redxt bootstraps recursion from a local root-hints file exactly like BIND's `hints` zone type; it downloads the file from IANA on first run if missing, and refreshes it **monthly** on the scheduler (root server IP/name changes are rare — IANA's list has changed only a handful of times in the file's history — so monthly comfortably catches a renumbering without meaningful staleness risk; no external dependency at query time, a cached copy is never a startup blocker).
- **named.conf — full support, not one-time-only**: keys, ACLs, options (notify/also-notify/notify-delay, allow-update, allow-update-forwarding, allow-transfer, allow-query, allow-recursion, forwarders, dnssec-validation, validate-except, transfer-format, max-cache-size, ...), logging categories, controls, and zone declarations — with a migration report for anything unmappable. redxt's own runtime config stays database-driven (per the "Standard casapps patterns" convention below), but named.conf/zone files are kept in **live bidirectional sync**, not treated as a one-shot import: an on-disk edit is detected (watch + reload, same mechanism as the git-backed zone sync) and merged into the DB; a DB-side change is rendered back out to the on-disk named.conf/zone files. Conflicting concurrent edits (disk vs. DB) are surfaced for review, never silently overwritten, following the existing zone-versioning diff/rollback model.
- **One-command migration wizard**: point redxt at a live named server — it AXFRs every zone, parses named.conf, recreates keys/ACLs/policies, and produces the report. The same wizard handles the other importers.
- **Other importers**: dnsmasq, Unbound local-zone/local-data, Pi-hole Teleporter, Technitium, PowerDNS, Knot/NSD, tinydns/djbdns, Windows DNS, hosted-provider BIND exports (Cloudflare, Route 53, ...). Exports: BIND master files, JSON, CSV.
- **rndc protocol**: redxt implements BIND's actual `rndc` wire protocol (not just an equivalent), so the stock `rndc` binary and existing `rndc.conf`/keys work unmodified against redxt as a drop-in — reload, freeze/thaw, retransfer, notify, flush (global/per-domain), key management, statistics dump. The same operations are also exposed natively via `redxt-cli` and the API for non-BIND workflows. **Auto-generated on first run**: redxt generates its own rndc key and a ready-to-use `rndc.conf` (config dir, per AI.md PART 8) so `rndc` works immediately with zero manual setup, same as the rest of first-run zero-config; an operator migrating an existing BIND deployment can instead import their current `rndc.conf`/key so the existing `rndc` invocations keep working unchanged.
- **Config/zone validation** (`named-checkconf`/`named-checkzone`-equivalent): the server binary's flag set is fixed per AI.md PART 8 ("THESE SERVER COMMANDS CANNOT BE CHANGED"), so this lives on the multi-command client instead — `redxt-cli check config` (validates the current named.conf-equivalent config, named-checkconf-style report and exit code), `redxt-cli check zone {zone}` (syntax, SOA/NS sanity, delegation, lame-delegation lint), `redxt-cli check root` (validates the cached root-hints file: freshness, syntax, priming-response sanity) — same auth/exit-code conventions as the rest of `redxt-cli` per PART 33.

### TSIG, GSS-TSIG & keys

- **TSIG algorithms**: HMAC-SHA512 **(default)**, SHA384, SHA256, SHA224, SHA1, MD5 (legacy interop only). SIG(0) supported.
- **GSS-TSIG (RFC 3645)**: secure dynamic updates from AD domain-joined Windows clients and DCs, with identity-scoped update policies.
- Key generation, rotation, expiry from UI/CLI/API; keys usable in ACLs with named semantics (`allow-update { key "dhcp-key"; }`) on updates, transfers, and NOTIFY.

### Dynamic DNS

- **ISC-style (RFC 2136/3007)**: full DNS UPDATE with TSIG/GSS-TSIG auth, prerequisites, per-zone/per-key `update-policy` grants, and update forwarding from secondaries to the primary. Works out of the box with nsupdate, ISC dhcpd, Kea, and certbot `dns-rfc2136`.
- **Provider mode (DuckDNS / No-IP / DynDNS / FreeDNS replacement)**:
  - **Self-service subdomains** under operator-designated parent zones, governed by the instance registration mode (invite default) plus per-user quotas and reserved names.
  - **Update API compatibility**: DynDNS2, No-IP, DuckDNS (`/update?domains=&token=&ip=` with `txt=` and `clear=`), FreeDNS, and a native JSON endpoint — existing routers, ddclient, inadyn, and DuckDNS containers migrate by changing URL and token.
  - **Records**: A/AAAA with auto-detected client IP (dual v4+v6), TXT for ACME DNS-01 on dynamic hosts, optional MX/CNAME per host.
  - **Parking**: per-hostname customizable offline/parking page via the redirect engine.
  - **Lifecycle**: per-token scoping, update history, stale-host expiry with warnings, rotation/revocation.

### ACME / certificates

- **DNS-01 for others**: acme-dns-compatible REST API plus RFC 2136, with credentials restricted to `_acme-challenge` names.
- **Itself**: built-in ACME client for DoT/DoH/DoQ/UI/redirector certificates, including automatic DNS-01 issuance/renewal for hosted custom domains (self-signed fallback, auto-renewal); can be disabled.
- **DANE/TLSA** generation and maintenance from certificates; CAA hosting with validation.

### Transports & modern DNS

- **Listeners**: Do53 UDP/TCP, DoT (RFC 7858), DoH on HTTP/2 + HTTP/3 (RFC 8484), DoQ (RFC 9250), **DNSCrypt**. Encrypted listeners on by default with automatic certificates.
- **Discovery**: DDR (RFC 9462) and SVCB/HTTPS (RFC 9460/9461) published automatically; DNR-ready (RFC 9463) for the DHCP companion.
- **Upstream**: forwarders and transfers over TLS (XoT, RFC 9103); upstream transports include Do53, DoT, DoH, DoQ, DNSCrypt; egress optionally via SOCKS5/HTTP proxy or Tor.
- **ECS (RFC 7871)**: inbound honored for geo-aware authoritative answers (only when GeoIP enabled); **outbound to forwarders off by default** (scope-0/stripped), admin-enableable with prefix anonymization (/24 v4, /56 v6).

### Forwarders & upstream policy

- **Built-in catalog** (each profile ships Do53 v4+v6 IPs, DoT hostname, DoH URL): **Cloudflare** (standard — *default primary* — plus Malware 1.1.1.2 and Family 1.1.1.3), **Google** (*default secondary*; unfiltered like Cloudflare so failover never changes what resolves), **Quad9** (filtered 9.9.9.9 / unfiltered 9.9.9.10), **OpenDNS** (standard/FamilyShield), **AdGuard DNS** (standard/Family), **dns0.eu**, **Mullvad**, and **Custom** (any endpoint, any transport).
- **Primary + secondary pair required** (must differ; chosen in setup wizard). Rollover on timeout, SERVFAIL streaks, or sustained latency past threshold; continuous health probes; automatic fail-back to primary after a cooldown (no flapping).
- **Conditional forwarding** by subnet, domain, TLD, or regex; per-client-group pair overrides; local cache layer in front of forwarding.
- **Forwarding scope**: forwarding completes answers redxt itself doesn't hold — either no record at all, or a record type it isn't authoritative for on a zone it does hold — it is not general-purpose open recursion for arbitrary third-party domains (see "Open-resolver amplification/reflection" in Threat model).

### DNSSEC

- **Required by default**: every zone redxt is authoritative for is signed automatically — see Security decisions & exceptions #13.
- **Validation**: on by default, RFC 6840-compliant, algorithms per RFC 8624 (ECDSA RFC 6605, Ed25519/Ed448 RFC 8080); negative trust anchors; validate-except list (auto-seeded for `onion`/`exit`).
- **Signing**: one-click and automatic, KSK/ZSK or CSK, automated rollovers, NSEC/NSEC3 (RFC 5155, params per RFC 9276), CDS/CDNSKEY publication (RFC 7344/8078), DS display/export for manual registrars.
- **Integrity & monitoring**: ZONEMD (RFC 8976); **RRSIG-expiry monitoring with alerts**, DS/DNSKEY parent-mismatch detection, and **domain-expiry monitoring via RDAP** for org domains — all wired to the notification channels.

### Filtering & policy (Pi-hole replacement)

- **Blocklists**: hosts-file, Adblock-style, domain, wildcard, regex formats; scheduled auto-update with checksums; per-list stats and block attribution.
- **RPZ**: consume standard feeds (full triggers/actions) and publish redxt policy as an RPZ zone for downstream resolvers.
- **Client groups**: per-client/per-subnet policy groups (blocklists, forwarder pairs, schedules); identification by IP, subnet, or DoT/DoH credential.
- **Actions**: NXDOMAIN, NODATA, REFUSED, null IP, custom answer, redirect; allowlists; temporary "disable blocking for N minutes" globally or per client.
- **Safe modes**: safe-search enforcement and family-filter upstream profiles as toggles.
- **Tunneling detection**: rule-based (entropy, length, NXDOMAIN floods, unique-subdomain rate) with alerts and optional throttle actions.

### Clustering, agents & CLI

Cluster mechanics (join flow, secret distribution, primary election, heartbeats, split-brain/majority, removal cleanup) follow AI.md **PARTs 10/33/34** exactly. redxt-specific behavior:

- **Cluster nodes**: every node serves every zone authoritatively and runs the full HTTP surface; zone data rides the spec's sync. "Secondary" is not something users configure — ns1/ns2.redxt.us are simply cluster nodes.
- **Agent** (`redxt-agent`): **pure DNS data plane** — serves synced zones authoritatively, recursion/forwarding, blocklists, and client-group policies over **Do53/DoT/DoQ**; **DoH supported but disabled by default** (stripped `/dns-query`-only listener when enabled); zero management listener. Enrolls with an agent token; configured entirely from the server admin UI (spec agent pages) with redxt-specific per-agent settings: zone scope (all or selected), assigned policies/blocklists, forwarder pair (inherit or override), listener toggles. Drop a binary, paste a token, instant secondary.
- **Standards interop**: AXFR/IXFR/NOTIFY (+TSIG, +XoT) and **catalog zones (RFC 9432)** so third-party servers can be secondaries of redxt and vice versa — zone transfer (primary and secondary roles) is in scope for v1, not deferred.
- **CLI** (`redxt-cli`, required per PART 33): full management parity, rndc-equivalent ops, the diagnostics suite, token auth per spec priority chain.
- **Anycast-safe**: stateless query path on every node and agent.

### HTTP surface map

Subdomains the built-in HTTP server answers on the instance domain (redxt.us reference):

| Host | Serves |
|---|---|
| apex + `www` | Web UI, landing, login; spec path routes (`/api/{api_version}`, `/server/{admin_path}`); `/dns-query` DoH alias |
| `dns` | Canonical encrypted-DNS hostname: DoH `/dns-query`; cert/SAN + DDR/SVCB target for DoT/DoQ |
| `ddns` | Provider update APIs (DuckDNS/DynDNS2/No-IP/FreeDNS-compatible + native JSON) |
| `cve` | cvedex surface: `cve.redxt.us/2024-12345` → 302 redirector, JSON gateway, full-text search |
| `data` | Generic data-zones gateway (cvedex is dataset #1; future datasets appear here) |
| `rdap` | RDAP base URL (WHOIS stays on port 43; web WHOIS lookup is a UI tool) |
| `whoami` | Diagnostics echo: client IP, transport, ECS, resolver path |
| `ns1`/`ns2` | DNS-only hosts; HTTP redirects to apex |
| `*` (wildcard) | Unclaimed → 404/landing; claimed DDNS hostnames are DNS records only (or their parking page if parked) |

Custom domains (PART 36) let orgs serve their DDNS signup, redirector, parking, and data gateways under their own verified domains with automatic certificates; branding follows the spec's white-label rules (cosmetic only).

### Reserved subdomains

- **Hard-blocked (not admin-removable)**: every service subdomain in the HTTP surface map, all underscore-prefixed labels, `wpad`, `isatap`, `localhost`, and any hostname belonging to the instance itself.
- **Seeded defaults (admin-editable)**: mail/mx/smtp/imap/pop/webmail/email; ftp/ssh/vpn/wg/git; api/admin/root/auth/login/sso/oauth/id/account/accounts; billing/pay/secure/ssl; support/help/docs/blog/status/metrics; proxy/gateway/cdn/static/assets/files/backup; db/mysql/postgres/redis/registry/docker/jenkins/ci; test/dev/staging/demo/beta/internal/corp/portal/app/web/m; abuse/security/postmaster/hostmaster/webmaster/noreply; autoconfig/autodiscover; ntp/time/irc/xmpp/sip/stun/turn/ldap.

### RDAP & WHOIS

- **RDAP server** (RFC 7480/7481/9082/9083) for hosted zones; classic **WHOIS** responder on port 43 with templated, privacy-aware output.
- RDAP/WHOIS **client** in CLI and UI for external lookups (also powers domain-expiry monitoring).

### Diagnostics & tooling

In both `redxt-cli` and the web UI: dig/doggo-inspired query tool, deliberately inverted from dig's default — with no type given it queries and displays **every** record type held for the name (not just A), and an optional type argument **filters** the display down to that type; the type argument is **case-insensitive** (`A`/`a`, `AAAA`/`aaaa`, `ANY`/`any`, `TXT`/`txt`, etc. all accepted, normalized before lookup). Output follows the standard `--output {table|json|plain}` convention (default `table`): box-drawn, column-aligned, Dracula-themed colors, NO_COLOR-compliant — one row per record (type/name/TTL/data/DNSSEC status), not dig's raw wire-format dump; `json` gives the same fields machine-readable, `plain` gives bare values for piping. Any transport, `+trace`, DNSSEC chain display; DNSSEC chain-of-trust visualizer with failure pinpointing; propagation checker; zone linting (syntax, delegation, SOA/NS sanity, lame-delegation detection) on import and on demand; dnsperf-style benchmark mode; and a live query stream (WebSocket in UI, `redxt tail` in CLI) with filters.

### Observability

- **Query logging**: full audit (client, transport, question, answer, rcode, latency, policy hit) with retention policies and org-scoped visibility; per-category channels mirroring BIND's model (queries, xfer-in/out, update, notify, security, dnssec, resolver) with rotation; **dnstap export** for SIEM pipelines.
- **Metrics**: Prometheus — qps, rcodes, latency histograms, cache hit ratio, per-forwarder health, per-list block counts, cluster/agent state.
- **Dashboard**: Dracula-themed analytics — top clients/domains/blocked, transport mix, DNSSEC outcomes, upstream performance, agent fleet health.
- **Notifications**: multi-channel (email, Slack, Discord, Telegram, webhook, ...) for security events, transfer failures, cert/RRSIG/domain expiry, cluster/agent membership changes, blocklist failures, tunneling alerts.

### Integration & IaC

- **Terraform provider — first-class deliverable from v1**: full resource coverage (zones, records, templates, keys, policies, DDNS hosts, agents' assignments) against the REST API with org-scoped tokens; the API is designed with the provider as a primary consumer.
- **DHCP hook** for the future casapps DHCP server: RFC 2136+TSIG (identical to ISC dhcpd/Kea) plus a lease-event API; leases become A/AAAA/PTR with automatic expiry cleanup.
- **Webhooks out**: zone/record changes, security events, policy hits.
- **Git zone sync**: embedded pure-Go git, push-on-change, per-org remotes and credentials.
- **Backup/restore**: scheduled encrypted snapshots to local path or S3-compatible storage; one-command restore.
- **GeoIP** (optional local database): geo-aware records and analytics; off by default.

### RFC compliance

redxt implements the DNS server role itself, so per AI.md's "RFC-Based Applications" rule it must be fully RFC-compliant, not merely compatible-enough. Which RFCs are implemented, and to what depth, must stay documented here as features land — do not silently narrow or widen RFC scope without updating this section.

| Area | RFCs |
|---|---|
| Core protocol | 1034, 1035, 2181, 2308, 2317, 3597, 6891 (EDNS0), 7766 (TCP), 7873 (Cookies), 8020, 8482 (ANY), 8914 (Extended Errors) |
| Transfers & sync | 1995 (IXFR), 1996 (NOTIFY), 5936 (AXFR), 8945 (TSIG), 3645 (GSS-TSIG), 9103 (XoT), 9432 (catalog zones), 8976 (ZONEMD) |
| Dynamic update | 2136, 2845 (TSIG auth for updates), 3007 |
| DNSSEC | 4033–4035, 5155, 6605, 6840, 8080, 8198, 8624, 7344, 8078, 9276 |
| Encrypted transports | 7858 (DoT), 8484 (DoH), 9250 (DoQ); DNSCrypt (non-RFC, v2 protocol) |
| Service binding & discovery | 9460 (SVCB/HTTPS), 9461, 9462 (DDR), 9463 (DNR-ready) |
| Resolver behavior | 9156 (QNAME minimization), 6147 (DNS64), 8806 (hyperlocal root), 8767 (serve-stale), 7871 (ECS) |
| Privacy | 7830/8467 (padding), 9076 |
| Special-use | 6761, 7686 (.onion) |
| RDAP | 7480, 7481, 9082, 9083 |

BIND-style Response Rate Limiting (RRL) and per-client/per-zone throttles included though RRL is not an RFC. RFC 8901 multi-signer is roadmap.

### Standard casapps patterns (governed by AI.md)

Static single Go binary set (`redxt`, `redxt-agent`, `redxt-cli`, AMD64+ARM64), zero external runtime dependencies (including git — embedded pure-Go implementation), SQLite default with optional PostgreSQL/MariaDB, database-driven config, sane defaults with full customizability, security-first without blocking usability, first-user setup wizard, Dracula default theme, white-label per spec, Jenkins CI/CD, targeting self-hosted/SMB/enterprise users with low assumed technical knowledge.
