package database

// This file holds the DNS-specific schema described by IDEA.md. AI.md PART 10
// supplies the storage and access rules; IDEA.md supplies the data model, so
// where the two differ on WHAT data exists, IDEA.md governs and these tables
// follow its vocabulary.
//
// Placement follows the PART 5 "Full Database Schema Summary" rule: instance
// state goes to server.db, user-owned data goes to users.db. IDEA.md makes
// every zone, policy, key, and DDNS host org-owned, so the bulk of the DNS
// model lives in users.db. The instance-wide policy objects an operator
// configures once for the whole server stay in server.db.

// serverDNSTables holds the instance-wide DNS objects: filtering feeds,
// firewall and client-group policy, the upstream forwarder catalog, published
// data zones, and the reserved-name lists.
var serverDNSTables = []string{
	// blocklists holds the subscribed filtering feeds. format is one of the
	// IDEA.md formats: hosts, adblock, domain, wildcard, regex. checksum is
	// the digest of the last successfully fetched copy, so an unchanged feed
	// is not reparsed and a corrupted fetch is rejected. last_good_at records
	// when the currently loaded list was accepted, because a failed refresh
	// keeps the last good list rather than emptying the filter.
	//
	// org_id is nullable and holds an organizations.id from users.db. It is
	// not a foreign key: the two SQLite files cannot reference each other.
	// NULL means the feed is instance-wide.
	`CREATE TABLE IF NOT EXISTS blocklists (
		id            INTEGER PRIMARY KEY,
		org_id        INTEGER,
		name          TEXT NOT NULL,
		url           TEXT NOT NULL DEFAULT '',
		format        TEXT NOT NULL DEFAULT 'hosts',
		action        TEXT NOT NULL DEFAULT 'nxdomain',
		enabled       INTEGER NOT NULL DEFAULT 1,
		update_period TEXT NOT NULL DEFAULT '24h',
		checksum      TEXT NOT NULL DEFAULT '',
		entry_count   INTEGER NOT NULL DEFAULT 0,
		last_status   TEXT NOT NULL DEFAULT '',
		last_good_at  TIMESTAMP,
		updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, name)
	)`,

	// blocklist_entries holds the parsed rules of each feed plus the manually
	// added allow and deny entries. kind separates an exact domain from a
	// wildcard or a regular expression; allow marks an allowlist entry, which
	// wins over any block.
	`CREATE TABLE IF NOT EXISTS blocklist_entries (
		id           INTEGER PRIMARY KEY,
		blocklist_id INTEGER NOT NULL REFERENCES blocklists(id) ON DELETE CASCADE,
		pattern      TEXT NOT NULL,
		kind         TEXT NOT NULL DEFAULT 'domain',
		allow        INTEGER NOT NULL DEFAULT 0,
		answer       TEXT NOT NULL DEFAULT '',
		hits         INTEGER NOT NULL DEFAULT 0,
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(blocklist_id, pattern)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_blocklist_entries_pattern ON blocklist_entries(pattern)`,

	// client_groups holds the per-client and per-subnet policy groups from
	// IDEA.md "Filtering & policy": identification by IP, subnet, or an
	// encrypted-transport credential, and the blocklists, forwarder pair, and
	// schedule that apply to matching clients.
	//
	// disable_until implements the temporary "disable blocking for N minutes"
	// action without deleting the group's policy.
	`CREATE TABLE IF NOT EXISTS client_groups (
		id              INTEGER PRIMARY KEY,
		name            TEXT NOT NULL UNIQUE,
		match_type      TEXT NOT NULL DEFAULT 'subnet',
		match_value     TEXT NOT NULL DEFAULT '',
		blocklist_ids   TEXT NOT NULL DEFAULT '[]',
		forwarder_pair  TEXT NOT NULL DEFAULT '',
		safe_search     INTEGER NOT NULL DEFAULT 0,
		schedule        TEXT NOT NULL DEFAULT '',
		disable_until   TIMESTAMP,
		enabled         INTEGER NOT NULL DEFAULT 1,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// firewall_rules holds the DNS firewall policy: which clients, matched by
	// IP or CIDR, get which action for which names. It also carries the RPZ
	// triggers consumed from feeds and the redxt policy published back out as
	// an RPZ zone. priority orders evaluation, lowest first.
	`CREATE TABLE IF NOT EXISTS firewall_rules (
		id              INTEGER PRIMARY KEY,
		org_id          INTEGER,
		client_group_id INTEGER REFERENCES client_groups(id) ON DELETE SET NULL,
		name            TEXT NOT NULL DEFAULT '',
		match_type      TEXT NOT NULL DEFAULT 'domain',
		match_value     TEXT NOT NULL,
		qtype           TEXT NOT NULL DEFAULT '',
		action          TEXT NOT NULL DEFAULT 'nxdomain',
		answer          TEXT NOT NULL DEFAULT '',
		priority        INTEGER NOT NULL DEFAULT 100,
		enabled         INTEGER NOT NULL DEFAULT 1,
		schedule        TEXT NOT NULL DEFAULT '',
		expires_at      TIMESTAMP,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_firewall_rules_priority ON firewall_rules(enabled, priority)`,

	// forwarders holds the upstream catalog and the operator's custom
	// entries. role marks a member of the required primary and secondary
	// pair. transport is do53, dot, doh, doq, or dnscrypt. The health columns
	// are maintained by the probe loop and drive rollover and fail-back.
	`CREATE TABLE IF NOT EXISTS forwarders (
		id            INTEGER PRIMARY KEY,
		name          TEXT NOT NULL UNIQUE,
		profile       TEXT NOT NULL DEFAULT 'custom',
		role          TEXT NOT NULL DEFAULT 'none',
		transport     TEXT NOT NULL DEFAULT 'do53',
		addresses     TEXT NOT NULL DEFAULT '[]',
		hostname      TEXT NOT NULL DEFAULT '',
		doh_url       TEXT NOT NULL DEFAULT '',
		enabled       INTEGER NOT NULL DEFAULT 1,
		health        TEXT NOT NULL DEFAULT 'unknown',
		latency_ms    INTEGER NOT NULL DEFAULT 0,
		last_probe_at TIMESTAMP,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// conditional_forwards routes selected queries to a specific forwarder
	// pair by subnet, domain, TLD, or regular expression, per IDEA.md
	// "Conditional forwarding".
	`CREATE TABLE IF NOT EXISTS conditional_forwards (
		id            INTEGER PRIMARY KEY,
		match_type    TEXT NOT NULL DEFAULT 'domain',
		match_value   TEXT NOT NULL,
		primary_id    INTEGER REFERENCES forwarders(id) ON DELETE SET NULL,
		secondary_id  INTEGER REFERENCES forwarders(id) ON DELETE SET NULL,
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(match_type, match_value)
	)`,

	// data_zones holds the published datasets served by the data-zones
	// engine, cvedex being dataset one. release_tag and checksum record the
	// verified CI release currently loaded; a release whose signature or
	// checksum fails to verify is rejected and the last good dataset stays.
	`CREATE TABLE IF NOT EXISTS data_zones (
		id            INTEGER PRIMARY KEY,
		dataset       TEXT NOT NULL UNIQUE,
		zone          TEXT NOT NULL,
		source_url    TEXT NOT NULL DEFAULT '',
		release_tag   TEXT NOT NULL DEFAULT '',
		checksum      TEXT NOT NULL DEFAULT '',
		signature_ok  INTEGER NOT NULL DEFAULT 0,
		record_count  INTEGER NOT NULL DEFAULT 0,
		enabled       INTEGER NOT NULL DEFAULT 1,
		last_good_at  TIMESTAMP,
		updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// reserved_names holds the two-tier reserved-subdomain lists from
	// IDEA.md: hard-blocked entries that no administrator can remove, and
	// seeded defaults that an administrator may edit. hard_blocked separates
	// the tiers.
	`CREATE TABLE IF NOT EXISTS reserved_names (
		name         TEXT PRIMARY KEY,
		hard_blocked INTEGER NOT NULL DEFAULT 0,
		reason       TEXT NOT NULL DEFAULT '',
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// agents holds the enrolled redxt-agent data-plane nodes. Agents are not
	// cluster nodes: they never receive app_secrets and never join the
	// management plane, so they are tracked separately from cluster_nodes.
	// The enrollment token itself lives in the tokens table, hashed; only its
	// row id is referenced here.
	`CREATE TABLE IF NOT EXISTS agents (
		id            INTEGER PRIMARY KEY,
		agent_id      TEXT NOT NULL UNIQUE,
		name          TEXT NOT NULL DEFAULT '',
		owner_type    TEXT NOT NULL DEFAULT 'admin',
		owner_id      INTEGER NOT NULL DEFAULT 0,
		token_id      INTEGER REFERENCES tokens(id) ON DELETE SET NULL,
		address       TEXT NOT NULL DEFAULT '',
		app_version   TEXT NOT NULL DEFAULT '',
		zone_scope    TEXT NOT NULL DEFAULT 'all',
		zone_ids      TEXT NOT NULL DEFAULT '[]',
		policy_ids    TEXT NOT NULL DEFAULT '[]',
		listeners     TEXT NOT NULL DEFAULT '{}',
		state         TEXT NOT NULL DEFAULT 'healthy',
		last_seen     TIMESTAMP,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
}

// serverDNSUpdates holds the additive server.db DNS schema changes. Empty at
// the initial schema version.
var serverDNSUpdates = []string{}

// usersDNSTables holds the org-owned DNS data. Every table here carries an
// org_id, which is what enforces the IDEA.md rule that isolation between
// organizations is checked server-side on every path.
var usersDNSTables = []string{
	// zones holds every zone the instance is authoritative for, plus the
	// forward, stub, and local variants. kind is primary, secondary,
	// forward, or stub. serial_policy is date, unixtime, or increment, per
	// IDEA.md "SOA serial policy". dnssec_enabled defaults to 1 because
	// IDEA.md makes DNSSEC required rather than optional.
	//
	// verified_at records the TXT-challenge domain-ownership check that
	// IDEA.md requires before activation, which is what stops one
	// organization squatting another's zone. Zone names are unique
	// instance-wide for the same reason.
	`CREATE TABLE IF NOT EXISTS zones (
		id             INTEGER PRIMARY KEY,
		org_id         INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		name           TEXT NOT NULL UNIQUE,
		kind           TEXT NOT NULL DEFAULT 'primary',
		view           TEXT NOT NULL DEFAULT 'default',
		serial         INTEGER NOT NULL DEFAULT 0,
		serial_policy  TEXT NOT NULL DEFAULT 'date',
		soa_mname      TEXT NOT NULL DEFAULT '',
		soa_rname      TEXT NOT NULL DEFAULT '',
		refresh        INTEGER NOT NULL DEFAULT 86400,
		retry          INTEGER NOT NULL DEFAULT 7200,
		expire         INTEGER NOT NULL DEFAULT 3600000,
		minimum_ttl    INTEGER NOT NULL DEFAULT 3600,
		default_ttl    INTEGER NOT NULL DEFAULT 3600,
		primaries      TEXT NOT NULL DEFAULT '[]',
		also_notify    TEXT NOT NULL DEFAULT '[]',
		allow_transfer TEXT NOT NULL DEFAULT '[]',
		allow_update   TEXT NOT NULL DEFAULT '[]',
		allow_query    TEXT NOT NULL DEFAULT '[]',
		dnssec_enabled INTEGER NOT NULL DEFAULT 1,
		nsec3          INTEGER NOT NULL DEFAULT 1,
		catalog_member TEXT NOT NULL DEFAULT '',
		status         TEXT NOT NULL DEFAULT 'pending',
		verified_at    TIMESTAMP,
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_zones_org ON zones(org_id)`,

	// health_checks holds the probes behind health-checked failover records:
	// TCP, HTTP, or ICMP against a target, with the thresholds that flip a
	// record between healthy and unhealthy.
	`CREATE TABLE IF NOT EXISTS health_checks (
		id                  INTEGER PRIMARY KEY,
		org_id              INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		name                TEXT NOT NULL,
		protocol            TEXT NOT NULL DEFAULT 'tcp',
		target              TEXT NOT NULL,
		port                INTEGER NOT NULL DEFAULT 0,
		path                TEXT NOT NULL DEFAULT '',
		interval_seconds    INTEGER NOT NULL DEFAULT 30,
		timeout_seconds     INTEGER NOT NULL DEFAULT 5,
		healthy_threshold   INTEGER NOT NULL DEFAULT 2,
		unhealthy_threshold INTEGER NOT NULL DEFAULT 3,
		enabled             INTEGER NOT NULL DEFAULT 1,
		state               TEXT NOT NULL DEFAULT 'unknown',
		last_probe_at       TIMESTAMP,
		created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, name)
	)`,

	// records holds the resource records of every zone. rdata is the
	// presentation-format record data, which keeps RFC 3597 unknown types and
	// multi-string TXT records representable without a per-type column set.
	//
	// The health and geo columns support IDEA.md health-checked failover
	// records: weight drives weighted round-robin, geo_scope restricts an
	// answer to a region, and healthy is maintained by the probe loop.
	`CREATE TABLE IF NOT EXISTS records (
		id           INTEGER PRIMARY KEY,
		zone_id      INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		name         TEXT NOT NULL,
		type         TEXT NOT NULL,
		class        TEXT NOT NULL DEFAULT 'IN',
		ttl          INTEGER NOT NULL DEFAULT 3600,
		rdata        TEXT NOT NULL,
		priority     INTEGER NOT NULL DEFAULT 0,
		weight       INTEGER NOT NULL DEFAULT 0,
		geo_scope    TEXT NOT NULL DEFAULT '',
		health_check_id INTEGER REFERENCES health_checks(id) ON DELETE SET NULL,
		healthy      INTEGER NOT NULL DEFAULT 1,
		disabled     INTEGER NOT NULL DEFAULT 0,
		comment      TEXT NOT NULL DEFAULT '',
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_records_zone_name ON records(zone_id, name, type)`,
	`CREATE INDEX IF NOT EXISTS idx_records_name ON records(name, type)`,

	// zone_versions is the append-only zone history behind IDEA.md's diff and
	// rollback. content holds the rendered BIND master file for that version,
	// which is also what the git-backed sync pushes.
	`CREATE TABLE IF NOT EXISTS zone_versions (
		id         INTEGER PRIMARY KEY,
		zone_id    INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		serial     INTEGER NOT NULL DEFAULT 0,
		content    TEXT NOT NULL,
		diff       TEXT NOT NULL DEFAULT '',
		author     TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'ui',
		commit_ref TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_zone_versions_zone ON zone_versions(zone_id, created_at)`,

	// zone_templates holds the org-scoped blueprints new zones are created
	// from. variables declares the template's parameters; body holds the
	// record set with placeholders. org_id NULL marks an instance-level
	// default template.
	`CREATE TABLE IF NOT EXISTS zone_templates (
		id         INTEGER PRIMARY KEY,
		org_id     INTEGER REFERENCES organizations(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		variables  TEXT NOT NULL DEFAULT '{}',
		body       TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, name)
	)`,

	// scheduled_changes queues record changes for a maintenance window, with
	// the optional auto-revert IDEA.md calls for. payload holds the change to
	// apply; revert_payload holds what to restore when revert_at arrives.
	`CREATE TABLE IF NOT EXISTS scheduled_changes (
		id             INTEGER PRIMARY KEY,
		zone_id        INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		operation      TEXT NOT NULL,
		payload        TEXT NOT NULL DEFAULT '{}',
		revert_payload TEXT NOT NULL DEFAULT '{}',
		apply_at       TIMESTAMP NOT NULL,
		revert_at      TIMESTAMP,
		status         TEXT NOT NULL DEFAULT 'pending',
		created_by     INTEGER REFERENCES users(id),
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_scheduled_changes_apply ON scheduled_changes(status, apply_at)`,

	// dnssec_keys holds the signing keys. role is KSK, ZSK, or CSK.
	//
	// SECURITY: private_key_encrypted is the AES-256-GCM ciphertext of the
	// private key under server.security.encryption_key, never plaintext. A
	// hash is impossible here: signing requires the key material back.
	// IDEA.md rates this data Critical, exportable only by an org owner or
	// the instance admin, and included only in encrypted backups.
	// key_version records the encryption key generation used, so a key
	// rotation can still decrypt during its grace window.
	//
	// The rollover timestamps implement the RFC 6781 key lifecycle:
	// published, active, retired, revoked.
	`CREATE TABLE IF NOT EXISTS dnssec_keys (
		id                    INTEGER PRIMARY KEY,
		zone_id               INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		role                  TEXT NOT NULL DEFAULT 'CSK',
		algorithm             INTEGER NOT NULL,
		key_tag               INTEGER NOT NULL DEFAULT 0,
		flags                 INTEGER NOT NULL DEFAULT 256,
		public_key            TEXT NOT NULL,
		private_key_encrypted TEXT NOT NULL,
		key_version           INTEGER NOT NULL DEFAULT 1,
		state                 TEXT NOT NULL DEFAULT 'published',
		published_at          TIMESTAMP,
		activated_at          TIMESTAMP,
		retired_at            TIMESTAMP,
		revoked_at            TIMESTAMP,
		created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_dnssec_keys_zone ON dnssec_keys(zone_id, state)`,

	// tsig_keys holds the shared secrets used to authenticate dynamic
	// updates, transfers, and NOTIFY.
	//
	// SECURITY: secret_encrypted is the AES-256-GCM ciphertext under
	// server.security.encryption_key, not a hash. IDEA.md says TSIG secrets
	// are "hashed where verification-only suffices, encrypted where the
	// secret is needed" — TSIG is a symmetric HMAC over the message, so the
	// server must recompute the MAC from the raw secret to verify an inbound
	// update and must sign its own outbound transfers and NOTIFYs with it.
	// Encryption is therefore the only workable choice, and it is also what
	// lets an operator re-export a key into a peer's configuration. The
	// hashed alternative applies to the bearer credentials in the tokens
	// table, which are verification-only.
	//
	// algorithm defaults to HMAC-SHA512 per IDEA.md; HMAC-MD5 is accepted for
	// legacy interop but excluded from key generation. grants holds the BIND
	// update-policy grant set. acme_only restricts a key to _acme-challenge
	// names.
	`CREATE TABLE IF NOT EXISTS tsig_keys (
		id               INTEGER PRIMARY KEY,
		org_id           INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		name             TEXT NOT NULL,
		algorithm        TEXT NOT NULL DEFAULT 'hmac-sha512',
		secret_encrypted TEXT NOT NULL,
		key_version      INTEGER NOT NULL DEFAULT 1,
		grants           TEXT NOT NULL DEFAULT '[]',
		acme_only        INTEGER NOT NULL DEFAULT 0,
		deprecated       INTEGER NOT NULL DEFAULT 0,
		expires_at       TIMESTAMP,
		last_used_at     TIMESTAMP,
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, name)
	)`,

	// gss_tsig_identities maps an Active Directory machine or service
	// principal to a scoped update grant, per RFC 3645. No secret is stored:
	// the KDC authenticates the principal, so this table holds only the
	// identity and what it is allowed to change.
	`CREATE TABLE IF NOT EXISTS gss_tsig_identities (
		id         INTEGER PRIMARY KEY,
		org_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		principal  TEXT NOT NULL,
		realm      TEXT NOT NULL DEFAULT '',
		grants     TEXT NOT NULL DEFAULT '[]',
		enabled    INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, principal)
	)`,

	// ddns_hostnames holds the self-service dynamic hostnames of the provider
	// mode that replaces DuckDNS, No-IP, DynDNS, and FreeDNS. Each host is
	// owned by a user inside an organization and updates only itself.
	//
	// The update token itself is not stored here: it lives hashed in the
	// tokens table, which is the single place bearer credentials are kept.
	//
	// expires_at drives the stale-host expiry with warnings; parked and
	// parking_page back the offline parking page; suspended is the abuse
	// workflow's off switch.
	`CREATE TABLE IF NOT EXISTS ddns_hostnames (
		id            INTEGER PRIMARY KEY,
		org_id        INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		zone_id       INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
		hostname      TEXT NOT NULL UNIQUE,
		token_id      INTEGER,
		last_ipv4     TEXT NOT NULL DEFAULT '',
		last_ipv6     TEXT NOT NULL DEFAULT '',
		last_txt      TEXT NOT NULL DEFAULT '',
		ttl           INTEGER NOT NULL DEFAULT 60,
		parked        INTEGER NOT NULL DEFAULT 0,
		parking_page  TEXT NOT NULL DEFAULT '',
		suspended     INTEGER NOT NULL DEFAULT 0,
		last_update_at TIMESTAMP,
		expires_at    TIMESTAMP,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ddns_hostnames_user ON ddns_hostnames(user_id)`,

	// ddns_updates is the per-host update history IDEA.md requires for audit.
	//
	// PRIVACY: client_ip is stored truncated to the anonymized prefix (/24
	// for IPv4, /56 for IPv6), matching the query_logs treatment, because an
	// update history is as identifying as a query log.
	`CREATE TABLE IF NOT EXISTS ddns_updates (
		id          INTEGER PRIMARY KEY,
		hostname_id INTEGER NOT NULL REFERENCES ddns_hostnames(id) ON DELETE CASCADE,
		protocol    TEXT NOT NULL DEFAULT 'native',
		client_ip   TEXT NOT NULL DEFAULT '',
		result      TEXT NOT NULL DEFAULT 'good',
		detail      TEXT NOT NULL DEFAULT '',
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ddns_updates_host ON ddns_updates(hostname_id, created_at)`,

	// redirect_rules holds the HTTP redirect engine's rules. mode is cname
	// for the redirect.center-compatible CNAME-encoded scheme, txt for the
	// txtdirect-compatible TXT directive, or answer for a DNS-level answer
	// rewrite. directive_type carries the txtdirect type: host, path, gometa,
	// or dockerv2. txtdirect's proxy type is deliberately absent, per the
	// IDEA.md exclusion.
	`CREATE TABLE IF NOT EXISTS redirect_rules (
		id             INTEGER PRIMARY KEY,
		org_id         INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		zone_id        INTEGER REFERENCES zones(id) ON DELETE CASCADE,
		hostname       TEXT NOT NULL,
		mode           TEXT NOT NULL DEFAULT 'cname',
		directive_type TEXT NOT NULL DEFAULT 'host',
		target         TEXT NOT NULL DEFAULT '',
		status_code    INTEGER NOT NULL DEFAULT 302,
		pass_uri       INTEGER NOT NULL DEFAULT 0,
		match_regex    TEXT NOT NULL DEFAULT '',
		headers        TEXT NOT NULL DEFAULT '{}',
		fallback       TEXT NOT NULL DEFAULT '',
		enabled        INTEGER NOT NULL DEFAULT 1,
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(hostname, mode)
	)`,

	// query_logs is the PII-bearing query audit trail. IDEA.md rates it High
	// (PII) and requires retention policies, IP-truncation anonymization, and
	// purge on demand.
	//
	// PRIVACY: client_ip holds the ALREADY-TRUNCATED client address — the
	// /24 prefix for IPv4 and the /56 prefix for IPv6, matching the outbound
	// ECS anonymization. The full address is never written here.
	// retention_until is the row's own expiry, set from the applicable
	// retention policy at insert time, so a purge is a single indexed delete
	// and a row can never outlive the policy that admitted it.
	//
	// org_id scopes visibility: an organization member sees only rows for
	// that organization's zones, and instance-wide rows are visible to the
	// instance admin alone.
	`CREATE TABLE IF NOT EXISTS query_logs (
		id              INTEGER PRIMARY KEY,
		org_id          INTEGER,
		zone_id         INTEGER,
		client_ip       TEXT NOT NULL DEFAULT '',
		client_group_id INTEGER,
		transport       TEXT NOT NULL DEFAULT 'do53',
		qname           TEXT NOT NULL,
		qtype           TEXT NOT NULL,
		qclass          TEXT NOT NULL DEFAULT 'IN',
		rcode           TEXT NOT NULL DEFAULT 'NOERROR',
		answer_count    INTEGER NOT NULL DEFAULT 0,
		policy_hit      TEXT NOT NULL DEFAULT '',
		blocklist_id    INTEGER,
		dnssec_status   TEXT NOT NULL DEFAULT '',
		upstream        TEXT NOT NULL DEFAULT '',
		cache_hit       INTEGER NOT NULL DEFAULT 0,
		latency_us      INTEGER NOT NULL DEFAULT 0,
		node_id         TEXT NOT NULL DEFAULT '',
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		retention_until TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_query_logs_created ON query_logs(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_query_logs_retention ON query_logs(retention_until)`,
	`CREATE INDEX IF NOT EXISTS idx_query_logs_org ON query_logs(org_id, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_query_logs_qname ON query_logs(qname)`,

	// git_sync_settings holds the per-organization git remote that rendered
	// zone files are pushed to by the embedded pure-Go git implementation.
	//
	// SECURITY: credential_encrypted is the AES-256-GCM ciphertext of the
	// deploy key or token under server.security.encryption_key. IDEA.md rates
	// git sync credentials High and requires encryption at rest; the secret
	// must be recoverable to authenticate a push, so hashing is not an
	// option.
	`CREATE TABLE IF NOT EXISTS git_sync_settings (
		id                   INTEGER PRIMARY KEY,
		org_id               INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		remote_url           TEXT NOT NULL,
		branch               TEXT NOT NULL DEFAULT 'main',
		auth_type            TEXT NOT NULL DEFAULT 'ssh',
		credential_encrypted TEXT NOT NULL DEFAULT '',
		key_version          INTEGER NOT NULL DEFAULT 1,
		push_on_change       INTEGER NOT NULL DEFAULT 1,
		enabled              INTEGER NOT NULL DEFAULT 1,
		last_push_at         TIMESTAMP,
		last_error           TEXT NOT NULL DEFAULT '',
		created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, remote_url)
	)`,

	// domain_monitors backs the RDAP-driven expiry monitoring and the
	// DNSSEC integrity alerts: registrar expiry, RRSIG expiry, and a DS or
	// DNSKEY mismatch against the parent zone.
	`CREATE TABLE IF NOT EXISTS domain_monitors (
		id               INTEGER PRIMARY KEY,
		org_id           INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
		domain           TEXT NOT NULL,
		rdap_expires_at  TIMESTAMP,
		rrsig_expires_at TIMESTAMP,
		ds_match         INTEGER NOT NULL DEFAULT 1,
		last_checked_at  TIMESTAMP,
		last_error       TEXT NOT NULL DEFAULT '',
		enabled          INTEGER NOT NULL DEFAULT 1,
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(org_id, domain)
	)`,
}

// usersDNSUpdates holds the additive users.db DNS schema changes. Empty at the
// initial schema version.
var usersDNSUpdates = []string{}
