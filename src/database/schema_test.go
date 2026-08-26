package database

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"testing"
)

// createTableRE and createIndexRE extract object names from the DDL slices, so
// the tests assert against the schema as written rather than against a
// hand-maintained duplicate list that would drift the moment a table is added.
var (
	createTableRE = regexp.MustCompile(`(?i)CREATE TABLE IF NOT EXISTS ([a-z_]+)`)
	createIndexRE = regexp.MustCompile(`(?i)CREATE INDEX IF NOT EXISTS ([a-z_]+)`)
)

// declaredNames returns the object names the given DDL statements create.
func declaredNames(stmts []string, re *regexp.Regexp) []string {
	var names []string
	for _, s := range stmts {
		if m := re.FindStringSubmatch(s); m != nil {
			names = append(names, m[1])
		}
	}
	return names
}

// existingObjects reads the object names of the given type out of
// sqlite_master.
func existingObjects(t *testing.T, db *DB, kind string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = ?`, kind)
	if err != nil {
		t.Fatalf("read sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan sqlite_master: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read sqlite_master: %v", err)
	}
	return found
}

func TestEnsureServerSchema(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := EnsureServerSchema(ctx, db); err != nil {
		t.Fatalf("EnsureServerSchema: %v", err)
	}

	tables := existingObjects(t, db, "table")
	for _, want := range declaredNames(serverTables, createTableRE) {
		if !tables[want] {
			t.Errorf("server.db missing table %s", want)
		}
	}
	indexes := existingObjects(t, db, "index")
	for _, want := range declaredNames(serverTables, createIndexRE) {
		if !indexes[want] {
			t.Errorf("server.db missing index %s", want)
		}
	}
}

func TestEnsureUsersSchema(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := EnsureUsersSchema(ctx, db); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}

	tables := existingObjects(t, db, "table")
	for _, want := range declaredNames(usersTables, createTableRE) {
		if !tables[want] {
			t.Errorf("users.db missing table %s", want)
		}
	}
	indexes := existingObjects(t, db, "index")
	for _, want := range declaredNames(usersTables, createIndexRE) {
		if !indexes[want] {
			t.Errorf("users.db missing index %s", want)
		}
	}
}

// TestEnsureSchemaIdempotent is the central guarantee of the self-creating
// schema: running it again on an already-current database is a successful
// no-op, because it runs on every single start.
func TestEnsureSchemaIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		if err := EnsureServerSchema(ctx, db); err != nil {
			t.Fatalf("EnsureServerSchema run %d: %v", i+1, err)
		}
		if err := EnsureUsersSchema(ctx, db); err != nil {
			t.Fatalf("EnsureUsersSchema run %d: %v", i+1, err)
		}
	}
}

// TestSchemaTablesAreUnique catches the copy-paste failure of declaring the
// same table twice, which CREATE TABLE IF NOT EXISTS would silently swallow —
// the second definition would be ignored and its columns would never exist.
func TestSchemaTablesAreUnique(t *testing.T) {
	tests := []struct {
		name  string
		stmts []string
	}{
		{name: "server", stmts: serverTables},
		{name: "users", stmts: usersTables},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, re := range []*regexp.Regexp{createTableRE, createIndexRE} {
				seen := map[string]bool{}
				for _, n := range declaredNames(tt.stmts, re) {
					if seen[n] {
						t.Errorf("duplicate object %s", n)
					}
					seen[n] = true
				}
			}
		})
	}
}

// TestSchemaUpdatesAreAdditive enforces the PART 10 rule that the update list
// only ever adds. A DROP or a RENAME would break an existing installation, and
// an ADD COLUMN with no DEFAULT fails outright on a table that already has
// rows.
func TestSchemaUpdatesAreAdditive(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(DROP|RENAME|DELETE FROM|TRUNCATE)\b`)
	addColumn := regexp.MustCompile(`(?i)ADD COLUMN`)
	hasDefault := regexp.MustCompile(`(?i)\bDEFAULT\b`)

	tests := []struct {
		name  string
		stmts []string
	}{
		{name: "server", stmts: serverUpdates},
		{name: "users", stmts: usersUpdates},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range tt.stmts {
				if forbidden.MatchString(s) {
					t.Errorf("destructive update statement: %s", s)
				}
				if addColumn.MatchString(s) && !hasDefault.MatchString(s) {
					t.Errorf("added column has no DEFAULT: %s", s)
				}
			}
		})
	}
}

// TestEnsureSchemaAppliesUpdates drives ensureSchema through a real additive
// update and then re-runs it, which is the case isColumnExistsError exists to
// handle: the second run's ALTER must fail harmlessly rather than aborting
// startup.
func TestEnsureSchemaAppliesUpdates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tables := []string{`CREATE TABLE IF NOT EXISTS widgets (id INTEGER PRIMARY KEY)`}
	updates := []string{`ALTER TABLE widgets ADD COLUMN color TEXT NOT NULL DEFAULT 'red'`}

	for i := range 2 {
		if err := ensureSchema(ctx, db, "test", tables, updates); err != nil {
			t.Fatalf("ensureSchema run %d: %v", i+1, err)
		}
	}

	if _, err := db.Exec(`INSERT INTO widgets (id) VALUES (1)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var color string
	if err := db.QueryRow(`SELECT color FROM widgets WHERE id = 1`).Scan(&color); err != nil {
		t.Fatalf("select: %v", err)
	}
	if color != "red" {
		t.Errorf("color = %q, want %q", color, "red")
	}
}

// TestEnsureSchemaRollsBack checks that a failed create phase leaves nothing
// behind, so a start that fails halfway does not leave a partially built
// database that the next start would consider complete.
func TestEnsureSchemaRollsBack(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tables := []string{
		`CREATE TABLE IF NOT EXISTS good (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS bad (this is not valid sql`,
	}
	if err := ensureSchema(ctx, db, "test", tables, nil); err == nil {
		t.Fatal("ensureSchema succeeded on invalid DDL, want error")
	}

	got := existingObjects(t, db, "table")
	if got["good"] {
		t.Error("table good survived a rolled-back schema run")
	}
}

func TestIsColumnExistsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "sqlite duplicate column", err: errors.New("duplicate column name: color"), want: true},
		{name: "mysql capitalized", err: errors.New("Error 1060: Duplicate column name 'color'"), want: true},
		{name: "postgres already exists", err: errors.New(`pq: column "color" of relation "widgets" already exists`), want: true},
		{name: "mixed case", err: errors.New("DUPLICATE COLUMN NAME"), want: true},
		{name: "wrapped", err: errors.New("database: schema test: duplicate column name: color"), want: true},
		{name: "unrelated syntax error", err: errors.New(`near "widgts": syntax error`), want: false},
		{name: "no such table", err: errors.New("no such table: widgets"), want: false},
		{name: "disk full", err: errors.New("database or disk is full"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isColumnExistsError(tt.err); got != tt.want {
				t.Errorf("isColumnExistsError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestForeignKeysEnforced confirms the foreign_keys pragma is live against the
// real schema: an organization_members row for a user that does not exist must
// be rejected, not silently stored.
func TestForeignKeysEnforced(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := EnsureUsersSchema(ctx, db); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}
	_, err := db.Exec(`INSERT INTO organization_members (org_id, user_id, role) VALUES (1, 1, 'viewer')`)
	if err == nil {
		t.Fatal("insert with dangling foreign keys succeeded, want rejection")
	}
}

// TestSchemaTablesAreOrderedForForeignKeys catches a table that references
// another one declared later in the same slice. With foreign_keys enabled that
// ordering is fragile, so the create order must always place a referenced
// table first.
func TestSchemaTablesAreOrderedForForeignKeys(t *testing.T) {
	refRE := regexp.MustCompile(`(?i)REFERENCES ([a-z_]+)\s*\(`)

	tests := []struct {
		name  string
		stmts []string
	}{
		{name: "server", stmts: serverTables},
		{name: "users", stmts: usersTables},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := map[string]bool{}
			for _, s := range tt.stmts {
				m := createTableRE.FindStringSubmatch(s)
				if m == nil {
					continue
				}
				table := m[1]
				for _, ref := range refRE.FindAllStringSubmatch(s, -1) {
					target := ref[1]
					if target == table {
						continue
					}
					if !created[target] {
						t.Errorf("table %s references %s, which is created later", table, target)
					}
				}
				created[table] = true
			}
		})
	}
}

// TestServerAndUsersTablesDoNotOverlap guards the two-file split: a table
// defined in both databases would give the application two copies of the same
// data with no way to tell which one is authoritative.
func TestServerAndUsersTablesDoNotOverlap(t *testing.T) {
	server := declaredNames(serverTables, createTableRE)
	users := map[string]bool{}
	for _, n := range declaredNames(usersTables, createTableRE) {
		users[n] = true
	}

	var overlap []string
	for _, n := range server {
		if users[n] {
			overlap = append(overlap, n)
		}
	}
	sort.Strings(overlap)
	if len(overlap) > 0 {
		t.Errorf("tables defined in both databases: %v", overlap)
	}
}
