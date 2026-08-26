package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

// Cluster heartbeat timing from PART 10 "Cluster Heartbeat & Failure
// Handling".
const (
	// HeartbeatInterval is how often a node writes its own row.
	HeartbeatInterval = 30 * time.Second
	// HeartbeatTimeout is how long a node may go unheard from before it is
	// no longer considered healthy. It is three missed intervals, so a single
	// slow write or one dropped tick never demotes a working node.
	HeartbeatTimeout = 90 * time.Second
	// HeartbeatOffline is how long a node may go unheard from before it is
	// considered offline and excluded from quorum entirely.
	HeartbeatOffline = 5 * time.Minute
)

// Node states from PART 10. A node's state is derived from its last heartbeat
// on read rather than written by the node itself, because a node that has
// crashed cannot update its own row to say so.
const (
	// NodeHealthy means the node's last heartbeat is within HeartbeatTimeout.
	NodeHealthy = "healthy"
	// NodeDegraded means the node has missed heartbeats past HeartbeatTimeout
	// but not yet past HeartbeatOffline. It still counts toward quorum but is
	// not eligible to become primary.
	NodeDegraded = "degraded"
	// NodeOffline means the node has been silent past HeartbeatOffline. It is
	// excluded from quorum and from primary election.
	NodeOffline = "offline"
	// NodeRemoved means an operator has retired the node deliberately. It is
	// sticky: it is stored in the row and survives later heartbeats, so a
	// decommissioned node that is accidentally restarted does not silently
	// rejoin.
	NodeRemoved = "removed"
	// NodeStale means the node is reachable but is running against an
	// out-of-date secret version. It is sticky for the same reason: only the
	// primary clears it, after confirming the node has re-read.
	NodeStale = "stale"
)

// NodeIDSecret is the app_secrets row holding this installation's persisted
// node-id suffix. It is stored alongside the cryptographic secrets because it
// needs the same guarantee they do — written once, then stable forever — but
// it is not itself secret.
const NodeIDSecret = "node_id_suffix"

// nodeIDSuffixBytes is the length of the random half of a node id. Eight bytes
// is far more than enough to keep two nodes on identically named hosts apart.
const nodeIDSuffixBytes = 8

// Node is one row of the cluster_nodes heartbeat table.
//
// The five version fields are what PART 10 uses to detect secret drift: a node
// reporting a version below the cluster's current version is behind and must
// re-read before it can be trusted to validate a cookie or a CSRF token.
type Node struct {
	// ID is the stable node identifier, hostname plus persisted suffix.
	ID string
	// Hostname is the node's OS hostname, for display.
	Hostname string
	// Address is the address other nodes reach this one on.
	Address string
	// AppVersion is the running redxt version.
	AppVersion string
	// CommitHash is the build's commit, for spotting a mixed-build cluster.
	CommitHash string
	// InstallationSecretVersion is the app_secrets version this node last read.
	InstallationSecretVersion int
	// EncryptionKeyVersion is the server.security.encryption_key generation
	// this node is using.
	EncryptionKeyVersion int
	// CookieSigningKeyVersion is the cookie signing key version in use.
	CookieSigningKeyVersion int
	// CSRFTokenSecretVersion is the CSRF secret version in use.
	CSRFTokenSecretVersion int
	// LearnedOriginsVersion is the newest learned-origin generation this node
	// has loaded.
	LearnedOriginsVersion int
	// State is the stored state. It is authoritative only for the sticky
	// values NodeRemoved and NodeStale; otherwise the derived NodeState wins.
	State string
	// LastSeen is when the node last wrote its heartbeat, in UTC.
	LastSeen time.Time
	// JoinedAt is when the node first appeared, in UTC.
	JoinedAt time.Time
}

// NodeState derives a node's liveness from its last heartbeat.
//
// This is pure and takes now explicitly so the caller controls the clock and
// the thresholds are testable without waiting on real time.
//
//	within HeartbeatTimeout  -> healthy
//	within HeartbeatOffline  -> degraded
//	beyond HeartbeatOffline  -> offline
//
// A zero lastSeen is offline: a row with no heartbeat has never checked in.
func NodeState(lastSeen, now time.Time) string {
	if lastSeen.IsZero() {
		return NodeOffline
	}
	age := now.Sub(lastSeen)
	switch {
	case age < HeartbeatTimeout:
		return NodeHealthy
	case age < HeartbeatOffline:
		return NodeDegraded
	default:
		return NodeOffline
	}
}

// EffectiveState combines the stored state with the derived one.
//
// The sticky operator-set states win over liveness: a removed node that is
// still heartbeating is still removed, and a stale node is still stale until
// the primary clears it. Everything else is derived from LastSeen, because a
// crashed node cannot write "offline" into its own row.
func (n Node) EffectiveState(now time.Time) string {
	switch n.State {
	case NodeRemoved, NodeStale:
		return n.State
	default:
		return NodeState(n.LastSeen, now)
	}
}

// Heartbeat writes this node's row, inserting it on first call and refreshing
// last_seen and the reported versions on every call after.
//
// The stored state is deliberately NOT overwritten by an ordinary heartbeat.
// If it were, a node marked removed or stale would clear its own mark simply
// by continuing to run, which is exactly what those two states exist to
// prevent. Clearing them is an explicit administrative action.
//
// joined_at is set only by the insert, so it keeps recording when the node
// first appeared rather than when it last restarted.
func Heartbeat(ctx context.Context, db *DB, n Node) error {
	if n.ID == "" {
		return fmt.Errorf("database: heartbeat: empty node id")
	}

	const q = `INSERT INTO cluster_nodes (
	               node_id, hostname, address, app_version, commit_hash,
	               installation_secret_version, server_security_encryption_key_version,
	               cookie_signing_key_version, csrf_token_secret_version,
	               learned_origins_version, state, last_seen, joined_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	           ON CONFLICT (node_id) DO UPDATE SET
	               hostname = excluded.hostname,
	               address = excluded.address,
	               app_version = excluded.app_version,
	               commit_hash = excluded.commit_hash,
	               installation_secret_version = excluded.installation_secret_version,
	               server_security_encryption_key_version = excluded.server_security_encryption_key_version,
	               cookie_signing_key_version = excluded.cookie_signing_key_version,
	               csrf_token_secret_version = excluded.csrf_token_secret_version,
	               learned_origins_version = excluded.learned_origins_version,
	               last_seen = excluded.last_seen`

	now := FormatTime(time.Now())
	state := n.State
	if state == "" {
		state = NodeHealthy
	}

	if _, err := ExecContext(ctx, db, TimeoutWrite, q,
		n.ID, n.Hostname, n.Address, n.AppVersion, n.CommitHash,
		n.InstallationSecretVersion, n.EncryptionKeyVersion,
		n.CookieSigningKeyVersion, n.CSRFTokenSecretVersion,
		n.LearnedOriginsVersion, state, now, now); err != nil {
		return fmt.Errorf("database: heartbeat %s: %w", n.ID, err)
	}
	return nil
}

// ListNodes returns every row of cluster_nodes ordered by node id.
//
// The order is what makes primary election deterministic: PrimaryNode picks
// the lowest healthy id, and every node reading the same table in the same
// order reaches the same answer without any coordination.
//
// Rows are returned as stored. Deriving liveness is the caller's job, via
// EffectiveState, so that one consistent "now" applies across the whole set
// rather than a slightly different one per row.
func ListNodes(ctx context.Context, db *DB) ([]Node, error) {
	const q = `SELECT node_id, hostname, address, app_version, commit_hash,
	                  installation_secret_version, server_security_encryption_key_version,
	                  cookie_signing_key_version, csrf_token_secret_version,
	                  learned_origins_version, state, last_seen, joined_at
	           FROM cluster_nodes
	           ORDER BY node_id`

	rows, cancel, err := QueryContext(ctx, db, TimeoutSimple, q)
	if err != nil {
		return nil, fmt.Errorf("database: list nodes: %w", err)
	}
	defer cancel()
	defer func() { _ = rows.Close() }()

	var nodes []Node
	for rows.Next() {
		var n Node
		var lastSeen, joinedAt any
		if err := rows.Scan(&n.ID, &n.Hostname, &n.Address, &n.AppVersion, &n.CommitHash,
			&n.InstallationSecretVersion, &n.EncryptionKeyVersion,
			&n.CookieSigningKeyVersion, &n.CSRFTokenSecretVersion,
			&n.LearnedOriginsVersion, &n.State, &lastSeen, &joinedAt); err != nil {
			return nil, fmt.Errorf("database: list nodes: %w", err)
		}
		n.LastSeen = toTime(lastSeen)
		n.JoinedAt = toTime(joinedAt)
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: list nodes: %w", err)
	}
	return nodes, nil
}

// PrimaryNode elects the cluster primary: the healthy node with the lowest
// node id.
//
// It is pure so every node computes the same result from the same table with
// no election protocol and no messages. PART 10 specifies no preemption, which
// falls out of this rule naturally — while the current primary stays healthy
// it keeps the lowest healthy id, so the answer does not change. Only its
// failure moves the role, and it moves to exactly one node because the id
// order is total.
//
// Liveness is derived against now rather than trusted from the stored state,
// so a crashed primary that never got to update its own row is still correctly
// skipped. Degraded, offline, removed, and stale nodes are all ineligible: a
// stale node is running on the wrong secret version and must not be handed a
// rotation.
//
// The second return is false when no node qualifies, which the caller must
// treat as "no primary" and not as "I am the primary".
func PrimaryNode(nodes []Node) (Node, bool) {
	return primaryNodeAt(nodes, time.Now())
}

// PrimaryNodeAt is PrimaryNode with an explicit clock, for callers that must
// evaluate the cluster as of a specific instant.
func PrimaryNodeAt(nodes []Node, now time.Time) (Node, bool) {
	return primaryNodeAt(nodes, now)
}

// primaryNodeAt holds the shared election logic. It scans rather than assuming
// the slice is sorted, so a caller that filtered or reordered ListNodes output
// still gets the correct answer.
func primaryNodeAt(nodes []Node, now time.Time) (Node, bool) {
	var best Node
	found := false
	for _, n := range nodes {
		if n.EffectiveState(now) != NodeHealthy {
			continue
		}
		if !found || n.ID < best.ID {
			best = n
			found = true
		}
	}
	return best, found
}

// HealthyNodes returns the subset of nodes that are healthy as of now.
func HealthyNodes(nodes []Node, now time.Time) []Node {
	var healthy []Node
	for _, n := range nodes {
		if n.EffectiveState(now) == NodeHealthy {
			healthy = append(healthy, n)
		}
	}
	return healthy
}

// HasQuorum reports whether a strict majority of the known, non-removed nodes
// are healthy as of now.
//
// This is the PART 10 anti-split-brain guard. A rotation or any other
// cluster-wide write must not proceed without it, because a partitioned
// minority that believed it was the whole cluster would elect its own primary
// and rotate a secret the majority never sees.
//
// Removed nodes are excluded from the denominator: a decommissioned node must
// not be able to hold the surviving cluster below quorum forever.
func HasQuorum(nodes []Node, now time.Time) bool {
	total := 0
	healthy := 0
	for _, n := range nodes {
		state := n.EffectiveState(now)
		if state == NodeRemoved {
			continue
		}
		total++
		if state == NodeHealthy {
			healthy++
		}
	}
	if total == 0 {
		return false
	}
	return healthy*2 > total
}

// MarkNodeState sets the stored state of a node, which is how the sticky
// NodeRemoved and NodeStale marks are applied and cleared.
//
// Setting NodeHealthy here clears a sticky mark and hands liveness back to the
// derived state; it does not claim the node is up.
func MarkNodeState(ctx context.Context, db *DB, nodeID, state string) error {
	switch state {
	case NodeHealthy, NodeDegraded, NodeOffline, NodeRemoved, NodeStale:
	default:
		return fmt.Errorf("database: mark node %s: unknown state %q", nodeID, state)
	}
	const q = `UPDATE cluster_nodes SET state = ? WHERE node_id = ?`
	if _, err := ExecContext(ctx, db, TimeoutWrite, q, state, nodeID); err != nil {
		return fmt.Errorf("database: mark node %s: %w", nodeID, err)
	}
	return nil
}

// PruneNodes deletes rows for nodes that have not been heard from since
// before cutoff, and reports how many were removed.
//
// Pruning is separate from the offline threshold on purpose. Going offline is
// recoverable and a returning node should find its row and its joined_at
// intact, so the cutoff a caller passes here is expected to be far longer than
// HeartbeatOffline — long enough that the node is genuinely gone rather than
// rebooting.
//
// The comparison works as a plain string comparison because every timestamp,
// whether written by FormatTime or by CURRENT_TIMESTAMP, uses the same
// zero-padded UTC layout, which sorts lexically in the same order as it does
// chronologically.
func PruneNodes(ctx context.Context, db *DB, cutoff time.Time) (int64, error) {
	const q = `DELETE FROM cluster_nodes WHERE last_seen < ?`
	res, err := ExecContext(ctx, db, TimeoutWrite, q, FormatTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("database: prune nodes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("database: prune nodes: %w", err)
	}
	return n, nil
}

// NodeID returns this node's stable cluster identifier, creating and
// persisting it on first call.
//
// The id is the OS hostname joined to a random suffix persisted in
// app_secrets. Both halves are needed. The hostname alone is not unique — two
// containers from the same image routinely share one — and would make two
// nodes fight over a single heartbeat row. A random value alone would be
// unique but unreadable, and an operator reading the node list needs to know
// which machine a row belongs to.
//
// The suffix is stored, not regenerated, so a restart rejoins as the same node
// instead of leaving an orphan row behind on every boot. It is created through
// EnsureSecret, which means two nodes starting simultaneously on a fresh
// database converge on the same suffix — and that is correct, because the
// suffix distinguishes installations, while the hostname distinguishes nodes
// within one.
//
// A hostname lookup failure is not fatal: the id falls back to the suffix
// alone, which is still unique and still stable.
func NodeID(ctx context.Context, db *DB) (string, error) {
	suffix, _, err := EnsureSecret(ctx, db, NodeIDSecret, newNodeIDSuffix)
	if err != nil {
		return "", err
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		return suffix, nil
	}
	return sanitizeHostname(host) + "-" + suffix, nil
}

// newNodeIDSuffix generates the random half of a node id.
func newNodeIDSuffix() (string, error) {
	buf := make([]byte, nodeIDSuffixBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("database: generate node id suffix: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// sanitizeHostname reduces a hostname to the characters a node id may contain,
// so an unusual hostname cannot produce an id that is awkward to display, log,
// or use in a lock name.
func sanitizeHostname(host string) string {
	host = strings.ToLower(host)
	var b strings.Builder
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// toTime converts a scanned TIMESTAMP column to a time.Time.
//
// The column is scanned into an any rather than a concrete type because a SQL
// driver is free to hand back either the stored text or an already-parsed
// time.Time for a column declared TIMESTAMP, and the choice varies by driver
// and by build. Scanning straight into a *string or a *sql.NullTime would work
// against one of those and fail against the other.
func toTime(v any) time.Time {
	switch t := v.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return t.UTC()
	case string:
		return parseTime(t)
	case []byte:
		return parseTime(string(t))
	default:
		return time.Time{}
	}
}

// nullTimeFrom converts a scanned TIMESTAMP column to a sql.NullTime, treating
// SQL NULL and an unreadable value alike as invalid.
func nullTimeFrom(v any) sql.NullTime {
	t := toTime(v)
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// parseTime converts a stored timestamp back to a time.Time.
//
// An unparseable or empty value yields the zero time, which NodeState reads as
// offline. That is the safe direction: a row whose timestamp cannot be read is
// treated as not heartbeating rather than as healthy.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(TimeLayout, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// ScanTime converts a TIMESTAMP column scanned into an any to a time.Time,
// mapping SQL NULL and any unreadable value to the zero time.
//
// It is exported so packages built on this one can read timestamps the same
// way, without each of them having to guess which concrete type the driver
// returns.
func ScanTime(v any) time.Time {
	return toTime(v)
}
