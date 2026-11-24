
## Project description

**redxt** is a fully RFC-compliant DNS platform that replaces BIND/named, Unbound, Pi-hole, Technitium, AdGuard Home, acme-dns, and hosted services like DuckDNS, No-IP, DynDNS, FreeDNS, and redirect.center with one self-hosted application. Its server provides authoritative DNS, recursive resolution, forwarding, DNS firewall, hosted dynamic DNS, HTTP redirects, and DNS-distributed data. Companion CLI and data-plane agent binaries provide operator access and distributed DNS service. The former **cvedex** project is fully absorbed into redxt as the flagship dataset of its data-zones engine; cvedex no longer exists as a separate application.

It targets self-hosters, SMBs, and enterprises who want one DNS daemon for everything, deployable by users with low assumed technical knowledge, with sane defaults and full customizability. ISC BIND formats (named.conf, master zone files) are the canonical interchange standard: a working named deployment migrates with zero hand-editing.

All DNS roles are available without requiring an external DNS service.

**Reference deployment**: the official site and flagship public instance run at **redxt.us** (serving `redxt.us` and `*.redxt.us`); **ns1.redxt.us** and **ns2.redxt.us** are cluster nodes. The public DDNS provider, redirect engine, and data zones (including cvedex) operate under this domain.

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

**Scope: DNS only.** The product provides authoritative DNS (primary/secondary), recursive resolution, forwarding, hybrid/split-horizon views, DNS firewall/blocking, hosted dynamic DNS, HTTP redirects, RDAP/WHOIS, and published data zones, plus the clustering, agents, security, and management features those roles need. A Terraform provider is a first-class deliverable from v1.

DNS protocol behavior must comply with all relevant RFCs. The supported protocol scope includes:

| Area | RFCs |
|---|---|
| Core protocol | 1034, 1035, 2181, 2308, 2317, 3597, 6891 (EDNS0), 7766 (TCP), 7873 (Cookies), 8020, 8482 (ANY), 8914 (Extended Errors) |
| Transfers & sync | 1995 (IXFR), 1996 (NOTIFY), 5936 (AXFR), 8945 (TSIG), 3645 (GSS-TSIG), 9103 (XoT), 9432 (catalog zones), 8976 (ZONEMD) |
| Dynamic update | 2136, 2845 (TSIG auth for updates), 3007 |
| DNSSEC | 4033–4035, 5155, 6605, 6840, 8080, 8198, 8624, 7344, 8078, 8901, 9276 |
| Encrypted transports | 7858 (DoT), 8484 (DoH), 9250 (DoQ); DNSCrypt (non-RFC, v2 protocol) |
| Service binding & discovery | 9460 (SVCB/HTTPS), 9461, 9462 (DDR), 9463 (DNR-ready) |
| Resolver behavior | 9156 (QNAME minimization), 6147 (DNS64), 8806 (hyperlocal root), 8767 (serve-stale), 7871 (ECS) |
| Privacy | 7830/8467 (padding), 9076 |
| Special-use | 6761, 7686 (.onion) |
| RDAP | 7480, 7481, 9082, 9083 |

BIND-style Response Rate Limiting (RRL) and per-client/per-zone throttles are also in scope.

**Non-goals (explicit):**

- **Not a DHCP server** — a separate future casapps DHCP project integrates through documented hooks (see Trust boundaries).
- **Not an mDNS/LLMNR reflector** — local discovery is out of scope.
- **Not an NTP server, registrar, CDN, reverse proxy, mail server, or packet firewall** — redxt hosts the records (MX, SPF, DKIM, DMARC, TLSA, CAA) and filters DNS answers, nothing else. HTTP serving is limited to the public service hosts listed below.
- **Not a downstream DNS load balancer (dnsdist)** — balancing across upstreams is the forwarder policy engine; balancing across redxt instances is clustering/agents; fronting third-party DNS pools is explicitly rejected. The supported pattern for legacy servers is migration (AXFR them in, retire them).
- **No plugins or installable apps** — extensions are provided through data zones, RPZ, REST APIs, webhooks, and notifications.
- **No org change-approval workflow** — versioning, audit trail, and role separation cover shared-org safety.
- **No AI/ML** — rule-based logic only (including the tunneling-detection rules).
- **No outbound telemetry, no cloud dependency, no monetization, no feature tiers** — every feature for every deployment size.
- **Not a byte-for-byte redirect.center/txtdirect clone** — redxt supports both projects' CNAME-encoded and TXT-directive DNS record schemes; txtdirect's `proxy` type is excluded because reverse-proxying is outside the product scope.

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

Instance admins, cluster nodes, and anonymous resolver clients have the distinct permissions and access boundaries described below and in AI.md PARTs 17 and 10.

### Data model & sensitivity

| Data | Sensitivity | Notes |
|---|---|---|
| DNSSEC private keys (KSK/ZSK/CSK) | **Critical** | Encrypted at rest; exportable only by org owners/instance admin; included only in encrypted backups |
| TSIG secrets, API/DDNS/agent tokens, GSS-TSIG material | **Critical** | Hashed where verification-only suffices, encrypted where the secret is needed; revocable; never logged |
| User credentials / MFA secrets | **Critical** | Passwords are one-way protected; MFA secrets are encrypted at rest. |
| Query logs | **High (PII)** | Client IP + names reveal behavior; retention policies, IP-truncation anonymization, purge on demand; org members see analytics only for their org's zones; instance-wide logs are instance-admin only |
| Git sync credentials (deploy keys/tokens) | **High** | Encrypted at rest; push-only scope recommended in docs |
| Zone data & records | Medium | Public by DNS nature; enumeration still resisted (NSEC3, ACL'd transfers) |
| Config, ACLs, policies, templates, blocklist subscriptions | Medium | Versioned with diff/rollback; append-only changelog |
| Metrics | Low | Aggregated; no per-client identifiers in Prometheus output by default |

### Trust boundaries & external services

All external dependencies are optional, fail safe, and have stated trust assumptions:

| External | Trust assumption | Failure mode |
|---|---|---|
| Upstream forwarders (built-in catalog + custom) | Untrusted for integrity unless DNSSEC-validated; trusted only to answer | Health probes demote; rollover to the user's secondary; serve-stale; if both down, fall back to full recursion |
| Root/authoritative internet DNS (recursion) | Untrusted; DNSSEC validation enforces integrity | SERVFAIL with RFC 8914 extended error on validation failure |
| Blocklist/RPZ feed URLs | **Untrusted input**: defensive parsing, size caps, checksums where offered; a feed can only block/rewrite within its declared policy scope, never add authoritative data | Keep last good list + notify; skip and report malformed entries |
| Data-zone sources (signed release feeds, e.g. cvedex) | Trusted only when release signature/checksum verifies | Keep last good dataset; alert |
| Tor daemon / network | Transport for `.onion`/`.exit` only (RFC 7686); DNSSEC validation auto-excepted for these zones | Tor down → NXDOMAIN/SERVFAIL for onion only |
| Outbound proxy (SOCKS5/HTTP; Tor is a special case) | Transport only; admin-configured | Proxy down → per-policy: fail closed (privacy) or fall back direct (availability), admin's choice |
| ACME CA | Cert issuance only | Renewal failure → notify + self-signed fallback; never blocks DNS |
| Git remotes (zone synchronization) | Push-only escrow target; remote treated as untrusted storage | Push failure → retry + notify; local history remains authoritative |
| RDAP registries (domain-expiry monitoring) | Informational only | Lookup failure → monitoring gap notification, no serving impact |
| S3-compatible backup targets | Untrusted storage; client-side encryption before upload | Backup failure → notify; serving unaffected |
| Webhooks/notification channels | Receive-only sinks | Retry with backoff, then log |
| Future casapps DHCP server | Authenticated peer via RFC 2136+TSIG and/or lease-event API with scoped token; leases become A/AAAA/PTR with expiry cleanup | DHCP down → records age out per policy |
| AD/Kerberos infrastructure (GSS-TSIG) | Trusted only for the identities it asserts on update messages | KDC unreachable → GSS-TSIG updates fail closed; everything else unaffected |
| GeoIP data (optional) | Informational | Missing → geo features disabled |
| Peer cluster nodes / agents | Authenticated members with scoped data access | Membership or quorum failures stop management writes; query serving continues where local data is available |
| Terraform provider | A privileged API client; trusts redxt's API, authenticated by scoped org tokens | API errors surface as Terraform failures; no special server-side trust |

Inbound DNS queries, DNS UPDATE messages, provider-API calls, encrypted-transport connections, and transfer requests are all **untrusted** and are subject to authorization, validation, and rate limits. The management plane trusts only authenticated sessions and tokens.

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
7. **Agents are data-plane only** — no management listener; **agent DoH is off by default**, and when enabled the listener handles DNS queries only (no management UI or API).
8. **DNSSEC validation exceptions** — `onion`/`exit` auto-excepted (RFC 7686); admin-added negative trust anchors / validate-except entries are logged and visible.
9. **Built-in ACME client makes outbound CA connections** — only for the server's own certificates; can be disabled in favor of supplied certs.
10. **Admin-configured remote fetches** (feeds, data zones, GeoIP, git, backups) are core functionality, governed by the SSRF restrictions above.
11. **Cluster writes require quorum** per spec — management-plane availability is sacrificed in minority partitions; query serving continues.
12. **Query path is stateless per node** — no node-pinned query state, so cluster nodes and agents are anycast-safe by design.
13. **DNSSEC is required, not optional** — every domain a user/org adds is signed by default (RFC 4033–4035); this is a stronger stance than "supported," reflecting that an RFC-noncompliant DNS response or an unauthenticated dynamic update is treated as a security bug, not a missing feature.
14. **BIND configuration and zone files are interoperability formats** — operators may import, export, and synchronize them with managed DNS settings and zone data; conflicting changes are surfaced for review.

---

### Server roles

Each zone and listener can use its own role and policy settings.

- **Authoritative** — primary/secondary hosting for any number of zones with full transfer, NOTIFY, and dynamic-update support.
- **Recursive resolver** — full iterative resolution from root hints with DNSSEC validation, QNAME minimization, aggressive negative caching (RFC 8198), serve-stale (RFC 8767), **cache prefetch** of popular expiring entries, **DNS64** (RFC 6147), and **hyperlocal root** (RFC 8806) serving the root zone locally.
- **Forwarder** — see "Forwarders & upstream policy".
- **Hybrid / split-horizon with views** — per-client-ACL zone sets and per-zone view answers; local zones authoritative, the rest recursed or forwarded.
- **Redirect engine** — supports redirect.center-compatible CNAME-encoded targets and txtdirect-compatible TXT directives. CNAME targets can specify a destination, protocol, path, query, and status code. TXT directives can specify a destination, status code, path behavior, Go import metadata, Docker registry redirects, custom headers, and reusable shared rules. Both modes support parking pages and DNS answer rewrites; reverse-proxying is out of scope.
  - **Shared behavior across modes**: self-redirect loop detection, a configurable path/host blocklist (redirect.center's "guardian" pattern generalized into the existing filtering/policy engine and per-org reserved-name rules), fallback-on-parse-error to a configured default rather than a hard error, and DNS-level answer rewrites (NXDOMAIN rewrites, answer overrides) as a third, non-HTTP redirect primitive.
  - **Parking**: per-hostname customizable parking pages for DDNS hosts (No-IP offline-mode equivalent), independent of which redirect mode created the record.
  - **Precedence**: CNAME-encoded mode wins when a name has both, reflecting redirect.center as the primary replacement target (BIND + redirect.center is the stated primary use case); the TXT directive is only consulted when no CNAME-encoded target resolves.
- **Data zones** — serves curated datasets as DNS zones, with DNSSEC signing, HTTP redirects, a JSON gateway, and full-text search. **cvedex is the flagship dataset**: CVE IDs use `YYYY-NNNNN.<zone>` labels (default zone `cve.`); future datasets may include MAC vendors, ASN, and TLD metadata.
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
- **Zone versioning**: full history with diff and rollback; optional git synchronization publishes zone changes in BIND format.

### ISC BIND compatibility & migration

- **Zone files**: full RFC 1035 master-file syntax — `$ORIGIN`, `$TTL`, `$INCLUDE`, `$GENERATE`, multi-line parentheses, multi-string TXT, comments preserved on round-trip, wildcards, RFC 3597 generic syntax. Import and export.
- **Root hints** (RFC 8109 priming): recursive service starts with a usable root-hints set and refreshes it monthly. A failed refresh leaves the last usable set available.
- **named.conf and zone files**: support BIND keys, ACLs, options, logging categories, controls, and zone declarations, with a migration report for anything unmappable. Operators may keep these files synchronized with managed settings and zone data; conflicting concurrent changes are shown for review instead of silently overwritten.
- **One-command migration wizard**: point redxt at a live named server — it AXFRs every zone, parses named.conf, recreates keys/ACLs/policies, and produces the report. The same wizard handles the other importers.
- **Other importers**: dnsmasq, Unbound local-zone/local-data, Pi-hole Teleporter, Technitium, PowerDNS, Knot/NSD, tinydns/djbdns, Windows DNS, hosted-provider BIND exports (Cloudflare, Route 53, ...). Exports: BIND master files, JSON, CSV.
- **rndc protocol**: existing `rndc` clients and configuration work with redxt for reload, freeze/thaw, retransfer, notify, flush, key management, and statistics. These operations are also available through redxt's management interface. Operators may import existing `rndc` credentials during migration.
- **Configuration and zone validation**: users can validate BIND-style configuration, zone syntax and delegation, and root-hints data. Reports identify errors and return a failing status when validation fails.

### TSIG, GSS-TSIG & keys

- **TSIG algorithms**: HMAC-SHA512 **(default)**, SHA384, SHA256, SHA224, SHA1, MD5 (legacy interop only). SIG(0) supported.
- **GSS-TSIG (RFC 3645)**: secure dynamic updates from AD domain-joined Windows clients and DCs, with identity-scoped update policies.
- Key generation, rotation, expiry from UI/CLI/API; keys usable in ACLs with named semantics (`allow-update { key "dhcp-key"; }`) on updates, transfers, and NOTIFY.

### Dynamic DNS

- **ISC-style (RFC 2136/3007)**: full DNS UPDATE with TSIG/GSS-TSIG auth, prerequisites, per-zone/per-key `update-policy` grants, and update forwarding from secondaries to the primary. Works out of the box with nsupdate, ISC dhcpd, Kea, and certbot `dns-rfc2136`.
- **Provider mode (DuckDNS / No-IP / DynDNS / FreeDNS replacement)**:
  - **Self-service subdomains** under operator-designated parent zones, governed by the instance registration mode (invite default) plus per-user quotas and reserved names.
  - **Update API compatibility**: DynDNS2, No-IP, DuckDNS, FreeDNS, and a native JSON endpoint — existing routers, ddclient, inadyn, and DuckDNS containers migrate by changing URL and token. The DuckDNS-compatible endpoint accepts domain, token, address, optional TXT, and clear parameters.
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

- **Built-in catalog**: profiles for **Cloudflare** (standard — *default primary* — plus Malware 1.1.1.2 and Family 1.1.1.3), **Google** (*default secondary*; unfiltered like Cloudflare so failover never changes what resolves), **Quad9** (filtered 9.9.9.9 / unfiltered 9.9.9.10), **OpenDNS** (standard/FamilyShield), **AdGuard DNS** (standard/Family), **dns0.eu**, **Mullvad**, and **Custom** (any endpoint, any transport). Profiles provide Do53, DoT, and DoH endpoints where supported.
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

Cluster nodes use authenticated membership, shared secrets, heartbeats, and primary election. A minority partition cannot accept management writes; DNS query service continues from each node's available data. redxt-specific behavior:

- **Cluster nodes**: every node serves every assigned zone authoritatively and provides the same management experience. ns1/ns2.redxt.us are cluster nodes.
- **Scheduled maintenance**: certificate renewal, GeoIP and blocklist updates, cve updates, backups, update checks, cleanup, and health reporting are coordinated across the cluster.
- **Agent** (`redxt-agent`): serves assigned zones authoritatively, recursion/forwarding, blocklists, and client-group policies over Do53, DoT, and DoQ. DoH is optional and disabled by default. Agents have no management interface and enroll with a scoped agent token; administrators assign zone scope, policies, blocklists, forwarders, and listener choices from the server.
- **Standards interop**: AXFR/IXFR/NOTIFY (+TSIG, +XoT) and **catalog zones (RFC 9432)** so third-party servers can be secondaries of redxt and vice versa — zone transfer (primary and secondary roles) is in scope for v1, not deferred.
- **CLI** (`redxt-cli`): full management parity, rndc-equivalent operations, the diagnostics suite, and token authentication.
- **Anycast-safe**: any eligible node or agent can answer a query for its assigned data.

### Public service hosts

Subdomains offered on the instance domain (redxt.us reference):

| Host | Serves |
|---|---|
| apex + `www` | Web UI, landing, login, management API, and DNS-over-HTTPS service |
| `dns` | Canonical encrypted-DNS hostname; certificate identity and DDR/SVCB target for DoT/DoQ |
| `ddns` | Provider update APIs (DuckDNS/DynDNS2/No-IP/FreeDNS-compatible + native JSON) |
| `cve` | cvedex redirector, JSON gateway, and full-text search |
| `data` | Generic data-zones gateway (cvedex is dataset #1; future datasets appear here) |
| `rdap` | RDAP service; WHOIS stays on port 43 and web WHOIS lookup is a UI tool |
| `whoami` | Diagnostics echo: client IP, transport, ECS, resolver path |
| `ns1`/`ns2` | DNS-only hosts; HTTP redirects to apex |
| `*` (wildcard) | Unclaimed → 404/landing; claimed DDNS hostnames are DNS records only (or their parking page if parked) |

Custom domains (PART 36) let orgs serve their DDNS signup, redirector, parking, and data gateways under their own verified domains with automatic certificates; branding follows the spec's white-label rules (cosmetic only).

### Reserved subdomains

- **Hard-blocked (not admin-removable)**: every service subdomain in the public service host list, all underscore-prefixed labels, `wpad`, `isatap`, `localhost`, and any hostname belonging to the instance itself.
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

- **Terraform provider — first-class deliverable from v1**: full resource coverage (zones, records, templates, keys, policies, DDNS hosts, agents' assignments) through the REST API with org-scoped tokens.
- **DHCP hook** for the future casapps DHCP server: RFC 2136+TSIG (identical to ISC dhcpd/Kea) plus a lease-event API; leases become A/AAAA/PTR with automatic expiry cleanup.
- **Webhooks out**: zone/record changes, security events, policy hits.
- **Git zone sync**: push-on-change with per-org remotes and credentials.
- **Backup/restore**: scheduled encrypted snapshots to local path or S3-compatible storage; one-command restore.
- **GeoIP** (optional): location-aware records and analytics; off by default.

### API routes

Native management routes use the versioned REST API. Organization routes require a session or token scoped to that organization; the service enforces the role permissions above. Responses contain the requested resource or a collection of resources, and errors use the shared API error format. DNS-over-HTTPS, RDAP, WHOIS, and compatibility routes retain their protocol-defined paths and authentication.

| Method | Path | Authentication; purpose; request and response |
|---|---|---|
| GET | `/api/{api_version}/orgs/{slug}/zones` | Org member; list zones; no body, returns zone summaries. |
| POST | `/api/{api_version}/orgs/{slug}/zones` | Org Admin/Owner; create a zone; accepts zone name and role settings, returns the new zone. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}` | Org member; read a zone; returns zone details. |
| PATCH | `/api/{api_version}/orgs/{slug}/zones/{zone_id}` | Org Admin/Owner; update zone settings; accepts changed fields, returns the updated zone. |
| DELETE | `/api/{api_version}/orgs/{slug}/zones/{zone_id}` | Org Admin/Owner; delete a zone; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/export` | Org member; export a zone; accepts a BIND, JSON, or CSV format, returns a downloadable file. |
| POST | `/api/{api_version}/orgs/{slug}/zones/import` | Org Admin/Owner; import zone data; accepts a BIND master file or supported provider export, returns an import report and created or updated zone. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records` | Org member; list records in a zone; returns record summaries. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records` | Org Editor/Admin/Owner; create a record; accepts record owner, type, TTL, and data, returns the record. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records/{record_id}` | Org member; read a record; returns record details. |
| PATCH | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records/{record_id}` | Org Editor/Admin/Owner; update a record; accepts changed record fields, returns the record. |
| DELETE | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/records/{record_id}` | Org Admin/Owner; delete a record; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/history` | Org member; list zone versions; returns version identifiers and change summaries. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/history/{version_id}` | Org member; read a historical zone version; returns the version's zone data and diff. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/history/{version_id}/restore` | Org Admin/Owner; restore a zone version; returns the restored current zone. |
| GET | `/api/{api_version}/orgs/{slug}/keys/tsig` | Org member with key-read scope; list TSIG keys; returns public key metadata, never secret material. |
| POST | `/api/{api_version}/orgs/{slug}/keys/tsig` | Org Admin/Owner; create a TSIG key; accepts name, algorithm, and grants, returns key metadata and the secret once. |
| GET | `/api/{api_version}/orgs/{slug}/keys/tsig/{key_id}` | Org member with key-read scope; read TSIG key metadata; returns grants and status, never secret material. |
| PATCH | `/api/{api_version}/orgs/{slug}/keys/tsig/{key_id}` | Org Admin/Owner; change TSIG grants or lifecycle; returns updated key metadata. |
| DELETE | `/api/{api_version}/orgs/{slug}/keys/tsig/{key_id}` | Org Admin/Owner; revoke a TSIG key; returns a revocation result. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec` | Org member; read DNSSEC status and public signing material; returns status and public DNSKEY/DS data. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec/keys` | Org Admin/Owner; generate or rotate signing keys; accepts key role and algorithm, returns public key metadata. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec/rollovers` | Org Admin/Owner; schedule a signing-key rollover; accepts rollover parameters, returns rollover status. |
| GET | `/api/{api_version}/orgs/{slug}/policies` | Org member; list DNS policies; returns policy summaries. |
| POST | `/api/{api_version}/orgs/{slug}/policies` | Org Admin/Owner; create a DNS policy; accepts policy rules and scope, returns the policy. |
| GET | `/api/{api_version}/orgs/{slug}/policies/{policy_id}` | Org member; read a DNS policy; returns policy details. |
| PATCH | `/api/{api_version}/orgs/{slug}/policies/{policy_id}` | Org Admin/Owner; update a DNS policy; accepts changed fields, returns the policy. |
| DELETE | `/api/{api_version}/orgs/{slug}/policies/{policy_id}` | Org Admin/Owner; delete a DNS policy; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/ddns/hosts` | Org member; list dynamic-DNS hosts; returns host metadata. |
| POST | `/api/{api_version}/orgs/{slug}/ddns/hosts` | Org Editor/Admin/Owner; create a dynamic-DNS host; accepts hostname and record options, returns host metadata and its update credential once. |
| GET | `/api/{api_version}/orgs/{slug}/ddns/hosts/{host_id}` | Org member; read host status and history; returns host metadata and update history. |
| PATCH | `/api/{api_version}/orgs/{slug}/ddns/hosts/{host_id}` | Org Editor/Admin/Owner; change host settings; accepts changed fields, returns host metadata. |
| DELETE | `/api/{api_version}/orgs/{slug}/ddns/hosts/{host_id}` | Org Admin/Owner; remove a dynamic-DNS host; returns a deletion result. |
| POST | `/api/{api_version}/ddns/update` | Host update credential; update the caller's host addresses and optional TXT data; accepts hostname, addresses, TXT and clear options, returns update status. |
| GET | `/update` on the `ddns` host | DuckDNS-compatible update credential; accepts `domains`, `token`, `ip`, optional `txt` and `clear`; returns DuckDNS-compatible status text. |
| GET | `/nic/update` on the `ddns` host | DynDNS2/No-IP-compatible credentials; accepts hostname and address parameters; returns provider-compatible status text. |
| GET | `/update.php` on the `ddns` host | FreeDNS-compatible update credential and hostname token; updates the assigned host and returns provider-compatible status text. |
| POST | `/register` on the `ddns` host | ACME-DNS registration; accepts account registration data, returns account credentials and update hostname. |
| POST | `/update` on the `ddns` host | ACME-DNS account credentials; accepts the ACME challenge TXT value, returns update status. |
| GET | `/api/{api_version}/orgs/{slug}/data-zones` | Org member; list published datasets; returns dataset summaries. |
| POST | `/api/{api_version}/orgs/{slug}/data-zones` | Org Admin/Owner; publish a dataset; accepts dataset metadata and source settings, returns dataset metadata. |
| GET | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}` | Org member; read dataset details and status; returns dataset metadata. |
| PATCH | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}` | Org Admin/Owner; update dataset settings; accepts changed fields, returns dataset metadata. |
| DELETE | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}` | Org Admin/Owner; unpublish a dataset; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/agents` | Org member; list assigned agents; returns agent status and scope. |
| POST | `/api/{api_version}/orgs/{slug}/agents` | Org Admin/Owner; enroll or assign an agent; accepts enrollment and scope settings, returns agent metadata. |
| GET | `/api/{api_version}/orgs/{slug}/agents/{agent_id}` | Org member; read agent status; returns assigned zones, policies, and health. |
| PATCH | `/api/{api_version}/orgs/{slug}/agents/{agent_id}` | Org Admin/Owner; change agent assignments; accepts changed scope and policy assignments, returns agent metadata. |
| DELETE | `/api/{api_version}/orgs/{slug}/agents/{agent_id}` | Org Admin/Owner; revoke agent enrollment; returns a revocation result. |
| GET | `/api/{api_version}/orgs/{slug}/zone-templates` | Org member; list reusable zone templates; returns template summaries. |
| POST | `/api/{api_version}/orgs/{slug}/zone-templates` | Org Admin/Owner; create a reusable zone template; accepts template fields and records, returns the template. |
| GET | `/api/{api_version}/orgs/{slug}/zone-templates/{template_id}` | Org member; read a template; returns template details. |
| PATCH | `/api/{api_version}/orgs/{slug}/zone-templates/{template_id}` | Org Admin/Owner; update a template; accepts changed fields, returns the template. |
| DELETE | `/api/{api_version}/orgs/{slug}/zone-templates/{template_id}` | Org Admin/Owner; delete a template; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/blocklists` | Org member; list blocklist feeds; returns feed status and policy scope. |
| POST | `/api/{api_version}/orgs/{slug}/blocklists` | Org Admin/Owner; subscribe to a blocklist feed; accepts feed URL, format, and policy scope, returns feed status. |
| GET | `/api/{api_version}/orgs/{slug}/blocklists/{list_id}` | Org member; read feed status and counts; returns feed metadata. |
| PATCH | `/api/{api_version}/orgs/{slug}/blocklists/{list_id}` | Org Admin/Owner; update feed settings; accepts changed fields, returns feed metadata. |
| DELETE | `/api/{api_version}/orgs/{slug}/blocklists/{list_id}` | Org Admin/Owner; unsubscribe from a feed; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/git-sync` | Org member; read zone synchronization status; returns remote and sync state without credentials. |
| PATCH | `/api/{api_version}/orgs/{slug}/git-sync` | Org Admin/Owner; configure zone synchronization; accepts remote and credential settings, returns sync state without secrets. |
| POST | `/api/{api_version}/orgs/{slug}/git-sync/push` | Org Admin/Owner; push zone changes; returns the resulting revision and sync status. |
| GET | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}/records` | Org member; list dataset entries; accepts filters and paging, returns matching entries. |
| POST | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}/records` | Org Editor/Admin/Owner; add a dataset entry; accepts entry data, returns the created entry. |
| GET | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}/records/{record_id}` | Org member; read a dataset entry; returns entry details. |
| PATCH | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}/records/{record_id}` | Org Editor/Admin/Owner; update a dataset entry; returns the updated entry. |
| DELETE | `/api/{api_version}/orgs/{slug}/data-zones/{dataset_id}/records/{record_id}` | Org Admin/Owner; delete a dataset entry; returns a deletion result. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec/keys` | Org member; list public DNSSEC key metadata; returns key roles and lifecycle status. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/dnssec/keys/{key_id}/rotate` | Org Admin/Owner; rotate a signing key; returns public key metadata and rollover status. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/transfers` | Org member; inspect transfer status and history; returns transfer summaries. |
| POST | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/transfers/notify` | Org Admin/Owner; send a zone-change notification; returns notification status. |
| GET | `/api/{api_version}/orgs/{slug}/zones/{zone_id}/query-log` | Org member; read query events for the zone; accepts filters and paging, returns org-scoped query summaries. |
| GET | `/api/{api_version}/orgs/{slug}/analytics` | Org member; read zone analytics; accepts a time range, returns aggregated counts and latency data. |
| GET | `/api/{api_version}/data/{dataset}/search` | Public; search a published dataset; accepts query and paging parameters, returns matching entries. |
| GET | `/api/{api_version}/data/{dataset}/{key}` | Public; read a published dataset entry; returns the public representation. |
| GET | `/search` on the cve host | Public; search the CVE dataset; accepts query and paging parameters, returns matching CVE summaries. |
| GET | `/{cve_id}` on the cve host | Public; resolve a CVE identifier in a browser; redirects to the authoritative reference. |
| GET | `/dns-query` on the apex or `dns` host | Public DNS query; accepts a DNS message using the DoH GET encoding, returns a DNS message. |
| POST | `/dns-query` on the apex or `dns` host | Public DNS query; accepts a DNS message body, returns a DNS message. |
| GET | `/{dataset}/{key}` on the data host | Public; read a published dataset entry; returns its public representation. |
| GET | `/` on the `whoami` host | Public; report the request's client address and DNS transport diagnostics. |
| GET | `/rdap/domain/{domain}` on the `rdap` host | Public; look up domain registration data; returns an RFC 9082 domain object. |
| GET | `/rdap/nameserver/{nameserver}` on the `rdap` host | Public; look up nameserver data; returns an RFC 9082 nameserver object. |
| GET | `/data/{dataset}/{key}` on the `data` host | Public; read a published data-zone item; returns the dataset's public representation. |
