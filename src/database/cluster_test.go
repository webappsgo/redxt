package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

// openServerDB opens a temp database with the server schema already applied,
// which is what the cluster and secret functions need.
func openServerDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	if err := EnsureServerSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureServerSchema: %v", err)
	}
	return db
}

func TestNodeState(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		lastSeen time.Time
		want     string
	}{
		{name: "just now", lastSeen: now, want: NodeHealthy},
		{name: "one interval ago", lastSeen: now.Add(-HeartbeatInterval), want: NodeHealthy},
		{name: "just inside timeout", lastSeen: now.Add(-HeartbeatTimeout + time.Second), want: NodeHealthy},
		{name: "exactly at timeout", lastSeen: now.Add(-HeartbeatTimeout), want: NodeDegraded},
		{name: "past timeout", lastSeen: now.Add(-2 * time.Minute), want: NodeDegraded},
		{name: "just inside offline", lastSeen: now.Add(-HeartbeatOffline + time.Second), want: NodeDegraded},
		{name: "exactly at offline", lastSeen: now.Add(-HeartbeatOffline), want: NodeOffline},
		{name: "long gone", lastSeen: now.Add(-24 * time.Hour), want: NodeOffline},
		{name: "zero time", lastSeen: time.Time{}, want: NodeOffline},
		{name: "clock skew into the future", lastSeen: now.Add(time.Minute), want: NodeHealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeState(tt.lastSeen, now); got != tt.want {
				t.Errorf("NodeState = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEffectiveState(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			name: "live node with no stored state",
			node: Node{LastSeen: now},
			want: NodeHealthy,
		},
		{
			name: "stored healthy but silent",
			node: Node{State: NodeHealthy, LastSeen: now.Add(-time.Hour)},
			want: NodeOffline,
		},
		{
			name: "removed node still heartbeating stays removed",
			node: Node{State: NodeRemoved, LastSeen: now},
			want: NodeRemoved,
		},
		{
			name: "stale node still heartbeating stays stale",
			node: Node{State: NodeStale, LastSeen: now},
			want: NodeStale,
		},
		{
			name: "stored offline but live is healthy again",
			node: Node{State: NodeOffline, LastSeen: now},
			want: NodeHealthy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.EffectiveState(now); got != tt.want {
				t.Errorf("EffectiveState = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrimaryNodeAt(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	live := now.Add(-10 * time.Second)
	dead := now.Add(-time.Hour)

	tests := []struct {
		name  string
		nodes []Node
		want  string
		found bool
	}{
		{name: "empty cluster", nodes: nil, found: false},
		{
			name:  "single healthy node",
			nodes: []Node{{ID: "a-1", LastSeen: live}},
			want:  "a-1",
			found: true,
		},
		{
			name: "lowest id wins",
			nodes: []Node{
				{ID: "c-3", LastSeen: live},
				{ID: "a-1", LastSeen: live},
				{ID: "b-2", LastSeen: live},
			},
			want:  "a-1",
			found: true,
		},
		{
			name: "dead lowest id is skipped",
			nodes: []Node{
				{ID: "a-1", LastSeen: dead},
				{ID: "b-2", LastSeen: live},
				{ID: "c-3", LastSeen: live},
			},
			want:  "b-2",
			found: true,
		},
		{
			name: "removed node is never primary",
			nodes: []Node{
				{ID: "a-1", State: NodeRemoved, LastSeen: live},
				{ID: "b-2", LastSeen: live},
			},
			want:  "b-2",
			found: true,
		},
		{
			name: "stale node is never primary",
			nodes: []Node{
				{ID: "a-1", State: NodeStale, LastSeen: live},
				{ID: "b-2", LastSeen: live},
			},
			want:  "b-2",
			found: true,
		},
		{
			name: "degraded node is never primary",
			nodes: []Node{
				{ID: "a-1", LastSeen: now.Add(-2 * time.Minute)},
				{ID: "b-2", LastSeen: live},
			},
			want:  "b-2",
			found: true,
		},
		{
			name: "no healthy node yields no primary",
			nodes: []Node{
				{ID: "a-1", LastSeen: dead},
				{ID: "b-2", LastSeen: dead},
			},
			found: false,
		},
		{
			name: "unsorted input still deterministic",
			nodes: []Node{
				{ID: "z-9", LastSeen: live},
				{ID: "a-1", LastSeen: live},
			},
			want:  "a-1",
			found: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PrimaryNodeAt(tt.nodes, now)
			if ok != tt.found {
				t.Fatalf("found = %v, want %v", ok, tt.found)
			}
			if ok && got.ID != tt.want {
				t.Errorf("primary = %q, want %q", got.ID, tt.want)
			}
		})
	}
}

// TestPrimaryNodeNoPreemption is the PART 10 stability property: while the
// current primary stays healthy, adding or losing other nodes never moves the
// role.
func TestPrimaryNodeNoPreemption(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	live := now.Add(-time.Second)

	nodes := []Node{{ID: "b-2", LastSeen: live}}
	first, ok := PrimaryNodeAt(nodes, now)
	if !ok || first.ID != "b-2" {
		t.Fatalf("initial primary = %v/%v, want b-2", first.ID, ok)
	}

	// A node with a HIGHER id joins: the primary must not move.
	nodes = append(nodes, Node{ID: "c-3", LastSeen: live})
	again, ok := PrimaryNodeAt(nodes, now)
	if !ok || again.ID != "b-2" {
		t.Errorf("primary after higher-id join = %v, want b-2", again.ID)
	}
}

func TestHasQuorum(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	live := now.Add(-time.Second)
	dead := now.Add(-time.Hour)

	tests := []struct {
		name  string
		nodes []Node
		want  bool
	}{
		{name: "no nodes", nodes: nil, want: false},
		{name: "single healthy node", nodes: []Node{{ID: "a", LastSeen: live}}, want: true},
		{name: "single dead node", nodes: []Node{{ID: "a", LastSeen: dead}}, want: false},
		{
			name: "two of three healthy",
			nodes: []Node{
				{ID: "a", LastSeen: live},
				{ID: "b", LastSeen: live},
				{ID: "c", LastSeen: dead},
			},
			want: true,
		},
		{
			name: "one of three healthy is a minority partition",
			nodes: []Node{
				{ID: "a", LastSeen: live},
				{ID: "b", LastSeen: dead},
				{ID: "c", LastSeen: dead},
			},
			want: false,
		},
		{
			name: "even split has no majority",
			nodes: []Node{
				{ID: "a", LastSeen: live},
				{ID: "b", LastSeen: dead},
			},
			want: false,
		},
		{
			name: "removed node is excluded from the denominator",
			nodes: []Node{
				{ID: "a", LastSeen: live},
				{ID: "b", State: NodeRemoved, LastSeen: dead},
			},
			want: true,
		},
		{
			name: "stale node counts against quorum",
			nodes: []Node{
				{ID: "a", LastSeen: live},
				{ID: "b", State: NodeStale, LastSeen: live},
				{ID: "c", State: NodeStale, LastSeen: live},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasQuorum(tt.nodes, now); got != tt.want {
				t.Errorf("HasQuorum = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHealthyNodes(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	nodes := []Node{
		{ID: "a", LastSeen: now},
		{ID: "b", LastSeen: now.Add(-time.Hour)},
		{ID: "c", State: NodeRemoved, LastSeen: now},
		{ID: "d", LastSeen: now},
	}
	got := HealthyNodes(nodes, now)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "d" {
		t.Errorf("HealthyNodes = %v, want [a d]", got)
	}
}

func TestHeartbeatInsertsAndUpdates(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	node := Node{
		ID:                        "host-abc",
		Hostname:                  "host",
		Address:                   "10.0.0.1:8080",
		AppVersion:                "1.0.0",
		CommitHash:                "deadbeef",
		InstallationSecretVersion: 1,
		EncryptionKeyVersion:      1,
		CookieSigningKeyVersion:   1,
		CSRFTokenSecretVersion:    1,
		LearnedOriginsVersion:     7,
	}
	if err := Heartbeat(ctx, db, node); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	nodes, err := ListNodes(ctx, db)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(nodes))
	}
	got := nodes[0]
	if got.ID != node.ID || got.Address != node.Address || got.LearnedOriginsVersion != 7 {
		t.Errorf("node = %+v, want the values written", got)
	}
	if got.State != NodeHealthy {
		t.Errorf("state = %q, want %q", got.State, NodeHealthy)
	}
	if got.LastSeen.IsZero() {
		t.Error("last_seen not parsed")
	}
	if got.EffectiveState(time.Now()) != NodeHealthy {
		t.Errorf("fresh heartbeat is not healthy: %+v", got)
	}
	joined := got.JoinedAt

	// A second heartbeat updates the reported values but must not create a
	// second row or reset joined_at.
	node.Address = "10.0.0.2:8080"
	node.LearnedOriginsVersion = 9
	if err := Heartbeat(ctx, db, node); err != nil {
		t.Fatalf("Heartbeat 2: %v", err)
	}
	nodes, err = ListNodes(ctx, db)
	if err != nil {
		t.Fatalf("ListNodes 2: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes after re-heartbeat, want 1", len(nodes))
	}
	if nodes[0].Address != "10.0.0.2:8080" || nodes[0].LearnedOriginsVersion != 9 {
		t.Errorf("heartbeat did not refresh reported values: %+v", nodes[0])
	}
	if !nodes[0].JoinedAt.Equal(joined) {
		t.Errorf("joined_at moved from %v to %v", joined, nodes[0].JoinedAt)
	}
}

// TestHeartbeatDoesNotClearStickyState is the anti-zombie rule: a node that
// has been removed or marked stale must not un-mark itself simply by staying
// up.
func TestHeartbeatDoesNotClearStickyState(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	node := Node{ID: "host-abc", Hostname: "host"}
	if err := Heartbeat(ctx, db, node); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	for _, sticky := range []string{NodeRemoved, NodeStale} {
		if err := MarkNodeState(ctx, db, node.ID, sticky); err != nil {
			t.Fatalf("MarkNodeState(%s): %v", sticky, err)
		}
		if err := Heartbeat(ctx, db, node); err != nil {
			t.Fatalf("Heartbeat after %s: %v", sticky, err)
		}
		nodes, err := ListNodes(ctx, db)
		if err != nil {
			t.Fatalf("ListNodes: %v", err)
		}
		if nodes[0].State != sticky {
			t.Errorf("state = %q after heartbeat, want %q", nodes[0].State, sticky)
		}
		if _, ok := PrimaryNode(nodes); ok {
			t.Errorf("a %s node was elected primary", sticky)
		}
	}

	// Clearing the mark hands liveness back to the derived state.
	if err := MarkNodeState(ctx, db, node.ID, NodeHealthy); err != nil {
		t.Fatalf("MarkNodeState(healthy): %v", err)
	}
	nodes, err := ListNodes(ctx, db)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if _, ok := PrimaryNode(nodes); !ok {
		t.Error("cleared node is still ineligible to be primary")
	}
}

func TestMarkNodeStateRejectsUnknownState(t *testing.T) {
	db := openServerDB(t)
	if err := MarkNodeState(context.Background(), db, "host-abc", "confused"); err == nil {
		t.Error("MarkNodeState accepted an unknown state")
	}
}

func TestHeartbeatRejectsEmptyID(t *testing.T) {
	db := openServerDB(t)
	if err := Heartbeat(context.Background(), db, Node{}); err == nil {
		t.Error("Heartbeat accepted an empty node id")
	}
}

func TestListNodesOrderedByID(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	for _, id := range []string{"c-3", "a-1", "b-2"} {
		if err := Heartbeat(ctx, db, Node{ID: id}); err != nil {
			t.Fatalf("Heartbeat %s: %v", id, err)
		}
	}
	nodes, err := ListNodes(ctx, db)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	want := []string{"a-1", "b-2", "c-3"}
	if len(nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(nodes), len(want))
	}
	for i, w := range want {
		if nodes[i].ID != w {
			t.Errorf("nodes[%d] = %q, want %q", i, nodes[i].ID, w)
		}
	}
}

func TestPruneNodes(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	// One current node via Heartbeat, one ancient node written directly so its
	// last_seen can be placed in the past.
	if err := Heartbeat(ctx, db, Node{ID: "live-1"}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	old := FormatTime(time.Now().Add(-30 * 24 * time.Hour))
	if _, err := db.Exec(
		`INSERT INTO cluster_nodes (node_id, state, last_seen, joined_at) VALUES (?, ?, ?, ?)`,
		"gone-1", NodeOffline, old, old); err != nil {
		t.Fatalf("insert stale node: %v", err)
	}

	n, err := PruneNodes(ctx, db, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("PruneNodes: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned %d nodes, want 1", n)
	}

	nodes, err := ListNodes(ctx, db)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ID != "live-1" {
		t.Errorf("nodes after prune = %v, want only live-1", nodes)
	}
}

func TestNodeID(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	first, err := NodeID(ctx, db)
	if err != nil {
		t.Fatalf("NodeID: %v", err)
	}
	if first == "" {
		t.Fatal("NodeID returned an empty id")
	}

	// The suffix is persisted, so a second call — standing in for a restart —
	// must return the same id rather than orphaning the previous row.
	second, err := NodeID(ctx, db)
	if err != nil {
		t.Fatalf("NodeID 2: %v", err)
	}
	if first != second {
		t.Errorf("NodeID is not stable: %q then %q", first, second)
	}

	suffix, _, _, err := GetSecret(ctx, db, NodeIDSecret)
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if !strings.HasSuffix(first, suffix) {
		t.Errorf("id %q does not end in the persisted suffix %q", first, suffix)
	}
	if len(suffix) != nodeIDSuffixBytes*2 {
		t.Errorf("suffix %q is %d chars, want %d", suffix, len(suffix), nodeIDSuffixBytes*2)
	}
}

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "node1", want: "node1"},
		{name: "uppercase", in: "NODE1", want: "node1"},
		{name: "fqdn", in: "dns1.example.com", want: "dns1.example.com"},
		{name: "underscore", in: "my_node", want: "my-node"},
		{name: "spaces", in: "my node", want: "my-node"},
		{name: "leading and trailing junk", in: "--node--", want: "node"},
		{name: "unicode", in: "nöde", want: "n-de"},
		{name: "all junk", in: "!!!", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeHostname(tt.in); got != tt.want {
				t.Errorf("sanitizeHostname(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{
			name: "stored layout",
			in:   "2026-08-25 14:05:09",
			want: time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC),
		},
		{
			name: "rfc3339 fallback",
			in:   "2026-08-25T14:05:09Z",
			want: time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC),
		},
		{name: "empty", in: "", want: time.Time{}},
		{name: "garbage", in: "not a time", want: time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTime(tt.in); !got.Equal(tt.want) {
				t.Errorf("parseTime(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestToTime covers every concrete type a driver may hand back for a
// TIMESTAMP column.
func TestToTime(t *testing.T) {
	want := time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC)

	tests := []struct {
		name string
		in   any
		want time.Time
	}{
		{name: "nil is zero", in: nil, want: time.Time{}},
		{name: "time value", in: want, want: want},
		{name: "offset time is normalized", in: want.In(time.FixedZone("x", 3600)), want: want},
		{name: "text", in: "2026-08-25 14:05:09", want: want},
		{name: "bytes", in: []byte("2026-08-25 14:05:09"), want: want},
		{name: "unexpected type is zero", in: 42, want: time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toTime(tt.in); !got.Equal(tt.want) {
				t.Errorf("toTime(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if got := ScanTime(tt.in); !got.Equal(tt.want) {
				t.Errorf("ScanTime(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNullTimeFrom(t *testing.T) {
	if got := nullTimeFrom(nil); got.Valid {
		t.Errorf("nullTimeFrom(nil) = %+v, want invalid", got)
	}
	got := nullTimeFrom("2026-08-25 14:05:09")
	if !got.Valid || !got.Time.Equal(time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC)) {
		t.Errorf("nullTimeFrom = %+v, want the parsed time", got)
	}
}

// TestParseTimeRoundTrip closes the loop between the writer and the reader.
func TestParseTimeRoundTrip(t *testing.T) {
	want := time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC)
	if got := parseTime(FormatTime(want)); !got.Equal(want) {
		t.Errorf("round trip = %v, want %v", got, want)
	}
}
