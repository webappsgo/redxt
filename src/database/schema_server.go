package database

import "slices"

// serverTables is the full server.db object list: the core server state from
// PART 5, PART 10, PART 11, and PART 12, followed by the instance-wide DNS
// policy objects from IDEA.md.
var serverTables = slices.Concat(serverCoreTables, serverDNSTables)

// serverUpdates is the full server.db additive update list.
//
// PART 10 rules for anything added here: every statement must be idempotent,
// every added column must carry a DEFAULT or be nullable, nothing is ever
// renamed or dropped, and a comment above each entry names the version that
// introduced it.
var serverUpdates = slices.Concat(serverCoreUpdates, serverDNSUpdates)

// serverCoreTables holds server and instance state. Per the PART 5 "Full
// Database Schema Summary" rule, everything that describes the running server
// rather than a user's data lives here.
var serverCoreTables = []string{
	// app_secrets holds the project-level cryptographic secrets from PART 11
	// "Cryptographic Keys": installation_secret, cookie_signing_key, and
	// csrf_token_secret. Values are base64-encoded 32-byte secrets and are
	// never returned by any API, never logged, and never rendered in the
	// admin UI beyond a fingerprint.
	//
	// Rotation is add-only: a rotation inserts the next version and sets the
	// previous version's expires_at to the end of its grace window. Old rows
	// are never deleted inside that window, so an in-flight HMAC signed with
	// the previous version still validates.
	//
	// revoked_for_node records a node that was removed from the cluster while
	// this version was current, so an audit can see a removal that happened
	// mid-rotation (PART 10 "Removed-Node Local Cleanup").
	`CREATE TABLE IF NOT EXISTS app_secrets (
		name             TEXT NOT NULL,
		version          INTEGER NOT NULL DEFAULT 1,
		value            TEXT NOT NULL,
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at       TIMESTAMP,
		revoked_for_node TEXT,
		PRIMARY KEY (name, version)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_app_secrets_name ON app_secrets(name, version DESC)`,

	// config is the database-driven configuration store from PART 5
	// "Database Schema for Configuration". value holds JSON; type records the
	// declared value type so a reader can decode without a schema lookup;
	// updated_by is a node id or "admin".
	`CREATE TABLE IF NOT EXISTS config (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		type       TEXT NOT NULL DEFAULT 'string',
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_by TEXT NOT NULL DEFAULT 'admin'
	)`,

	// config_meta carries the per-key metadata the admin UI needs: the
	// default value, whether changing the key requires a restart, and where
	// the key belongs in the settings tree.
	`CREATE TABLE IF NOT EXISTS config_meta (
		key             TEXT PRIMARY KEY,
		default_value   TEXT NOT NULL DEFAULT '',
		value_type      TEXT NOT NULL DEFAULT 'string',
		category        TEXT NOT NULL DEFAULT '',
		description     TEXT NOT NULL DEFAULT '',
		requires_restart INTEGER NOT NULL DEFAULT 0
	)`,

	// cluster_state is the PART 5 cluster state table. node_id is NULL for
	// values that are global to the cluster and set for per-node values such
	// as tor.onion_address.
	`CREATE TABLE IF NOT EXISTS cluster_state (
		key        TEXT NOT NULL,
		node_id    TEXT NOT NULL DEFAULT '',
		value      TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (key, node_id)
	)`,

	// cluster_nodes is the heartbeat table from PART 10 "Cluster Heartbeat &
	// Failure Handling". Every node upserts its row every 30 seconds; every
	// node reads the table to derive membership and elect the primary.
	//
	// The *_version columns let the primary detect secret-version drift: a
	// node reporting a version below the cluster's current version is told to
	// re-read, and is marked stale once the drift exceeds the rotation grace
	// window.
	`CREATE TABLE IF NOT EXISTS cluster_nodes (
		node_id                                TEXT PRIMARY KEY,
		hostname                               TEXT NOT NULL DEFAULT '',
		address                                TEXT NOT NULL DEFAULT '',
		app_version                            TEXT NOT NULL DEFAULT '',
		commit_hash                            TEXT NOT NULL DEFAULT '',
		installation_secret_version            INTEGER NOT NULL DEFAULT 0,
		server_security_encryption_key_version INTEGER NOT NULL DEFAULT 0,
		cookie_signing_key_version             INTEGER NOT NULL DEFAULT 0,
		csrf_token_secret_version              INTEGER NOT NULL DEFAULT 0,
		learned_origins_version                INTEGER NOT NULL DEFAULT 0,
		state                                  TEXT NOT NULL DEFAULT 'healthy',
		last_seen                              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		joined_at                              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_cluster_nodes_last_seen ON cluster_nodes(last_seen)`,

	// cluster_locks is the sentinel table behind the PART 10 anti-split-brain
	// advisory lock. On SQLite a rotation takes a BEGIN IMMEDIATE transaction
	// and writes the sentinel row; PostgreSQL uses pg_try_advisory_xact_lock
	// against the same logical name.
	`CREATE TABLE IF NOT EXISTS cluster_locks (
		name       TEXT PRIMARY KEY,
		holder     TEXT NOT NULL DEFAULT '',
		acquired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP
	)`,

	// learned_origins records the request origins the server has observed, so
	// a cluster node can report the newest observed_at it has read in its
	// heartbeat and detect that it is behind.
	`CREATE TABLE IF NOT EXISTS learned_origins (
		origin      TEXT PRIMARY KEY,
		first_seen  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		observed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		hits        INTEGER NOT NULL DEFAULT 1
	)`,
	`CREATE INDEX IF NOT EXISTS idx_learned_origins_observed ON learned_origins(observed_at)`,

	// tokens is the PART 11 "Token Database Schema" table, reproduced
	// verbatim. Only the SHA-256 hash of a token is stored; the plaintext is
	// shown once at creation and never again. token_prefix is the first eight
	// characters, kept purely for display.
	//
	// It lives in server.db even though user and org tokens reference user
	// and org ids, because token verification happens on the request hot path
	// for admin, user, org, and agent callers alike, and admin tokens exist
	// in every deployment whether or not multi-user is enabled.
	`CREATE TABLE IF NOT EXISTS tokens (
		id            INTEGER PRIMARY KEY,
		owner_type    TEXT NOT NULL,
		owner_id      INTEGER NOT NULL,
		name          TEXT NOT NULL,
		token_hash    TEXT NOT NULL,
		token_prefix  TEXT NOT NULL,
		scope         TEXT NOT NULL DEFAULT 'global',
		expires_at    TIMESTAMP,
		last_used_at  TIMESTAMP,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(owner_type, owner_id, name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tokens_hash ON tokens(token_hash)`,
	`CREATE INDEX IF NOT EXISTS idx_tokens_owner ON tokens(owner_type, owner_id)`,

	// rate_limits holds the per-identifier sliding-window counters. The
	// identifier is an IP, a login name, or an API token fingerprint; bucket
	// names the rule class (read, write, health, login, ...). Shared through
	// the database so a cluster enforces one budget rather than one per node.
	`CREATE TABLE IF NOT EXISTS rate_limits (
		identifier   TEXT NOT NULL,
		bucket       TEXT NOT NULL,
		window_start TIMESTAMP NOT NULL,
		count        INTEGER NOT NULL DEFAULT 0,
		updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (identifier, bucket, window_start)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rate_limits_window ON rate_limits(window_start)`,

	// ip_blocks holds the temporary and permanent client blocks produced by
	// repeated rate-limit violations, failed logins, and manual admin action.
	// expires_at NULL means the block is permanent until lifted.
	`CREATE TABLE IF NOT EXISTS ip_blocks (
		id         INTEGER PRIMARY KEY,
		cidr       TEXT NOT NULL UNIQUE,
		reason     TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'auto',
		created_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ip_blocks_expires ON ip_blocks(expires_at)`,

	// audit_log is the append-only record of admin actions, configuration
	// changes, and security events. It is written, never updated and never
	// deleted by application code; retention is enforced by an explicit
	// operator-run purge, not by the schema.
	`CREATE TABLE IF NOT EXISTS audit_log (
		id          INTEGER PRIMARY KEY,
		event       TEXT NOT NULL,
		actor_type  TEXT NOT NULL DEFAULT '',
		actor_id    TEXT NOT NULL DEFAULT '',
		target_type TEXT NOT NULL DEFAULT '',
		target_id   TEXT NOT NULL DEFAULT '',
		org_id      INTEGER,
		node_id     TEXT NOT NULL DEFAULT '',
		ip_address  TEXT NOT NULL DEFAULT '',
		details     TEXT NOT NULL DEFAULT '{}',
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_audit_log_event ON audit_log(event, created_at)`,

	// scheduler_tasks holds the scheduled task definitions. Only the primary
	// node runs them, per PART 10 "What Primary Node Handles".
	`CREATE TABLE IF NOT EXISTS scheduler_tasks (
		name        TEXT PRIMARY KEY,
		schedule    TEXT NOT NULL,
		enabled     INTEGER NOT NULL DEFAULT 1,
		last_run_at TIMESTAMP,
		next_run_at TIMESTAMP,
		updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// scheduler_history records each execution: when it ran, on which node,
	// how long it took, and whether it succeeded.
	`CREATE TABLE IF NOT EXISTS scheduler_history (
		id          INTEGER PRIMARY KEY,
		task_name   TEXT NOT NULL,
		node_id     TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'ok',
		message     TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		started_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		finished_at TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_scheduler_history_task ON scheduler_history(task_name, started_at)`,

	// backups holds snapshot metadata: where the snapshot went, how large it
	// is, and its checksum. Backup contents are encrypted client-side before
	// upload, so no key material is recorded here.
	`CREATE TABLE IF NOT EXISTS backups (
		id           INTEGER PRIMARY KEY,
		filename     TEXT NOT NULL,
		destination  TEXT NOT NULL DEFAULT 'local',
		size_bytes   INTEGER NOT NULL DEFAULT 0,
		checksum     TEXT NOT NULL DEFAULT '',
		encrypted    INTEGER NOT NULL DEFAULT 1,
		status       TEXT NOT NULL DEFAULT 'complete',
		created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at)`,

	// notification_channels holds the configured outbound notification
	// transports. credentials holds the channel secret encrypted with
	// server.security.encryption_key; it is never stored in plaintext and
	// never returned by an API.
	`CREATE TABLE IF NOT EXISTS notification_channels (
		id            INTEGER PRIMARY KEY,
		name          TEXT NOT NULL UNIQUE,
		type          TEXT NOT NULL,
		target        TEXT NOT NULL DEFAULT '',
		credentials   TEXT NOT NULL DEFAULT '',
		events        TEXT NOT NULL DEFAULT '[]',
		enabled       INTEGER NOT NULL DEFAULT 1,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
}

// serverCoreUpdates holds the additive server.db schema changes. It is empty
// at the initial schema version: every column above is part of the base
// CREATE TABLE. New columns append here with a comment naming their version.
var serverCoreUpdates = []string{}
