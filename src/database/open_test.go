package database

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// sqliteConfig is the minimal configuration every test opens with.
func sqliteConfig() config.Database {
	return config.Database{Type: DriverSQLite}
}

// openTestDB opens a throwaway SQLite database in the test's temp directory
// and registers its cleanup.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(sqliteConfig(), dir, "test.db", "test")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return db
}

func TestOpenSQLite(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(sqliteConfig(), dir, "test.db", "test")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if got := db.Driver(); got != DriverSQLite {
		t.Errorf("Driver() = %q, want %q", got, DriverSQLite)
	}
	if got := db.Name(); got != "test" {
		t.Errorf("Name() = %q, want %q", got, "test")
	}
	if !db.IsSQLite() {
		t.Error("IsSQLite() = false, want true")
	}
	if want := filepath.Join(dir, "test.db"); db.Path() != want {
		t.Errorf("Path() = %q, want %q", db.Path(), want)
	}
	if _, err := os.Stat(db.Path()); err != nil {
		t.Errorf("database file not created: %v", err)
	}
}

// TestOpenCreatesDirectory covers the mkdir -p behaviour: a nested data
// directory that does not exist yet must be created rather than failing the
// first start.
func TestOpenCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "db")
	db, err := Open(sqliteConfig(), dir, "test.db", "test")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	// The process umask can only remove bits, so the assertion is that no bit
	// beyond the requested mode is present. Testing for exact equality would
	// fail on a host running a stricter umask than the usual 022.
	if perm := info.Mode().Perm(); perm&^DirMode != 0 {
		t.Errorf("directory mode = %04o, want no bits beyond %04o", perm, DirMode)
	}

	fi, err := os.Stat(db.Path())
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if perm := fi.Mode().Perm(); perm&^FileMode != 0 {
		t.Errorf("file mode = %04o, want no bits beyond %04o", perm, FileMode)
	}
}

// TestOpenPragmas verifies the DSN pragmas actually took effect on the
// connection rather than merely being present in the DSN string. WAL and
// foreign key enforcement are correctness requirements, not tuning.
func TestOpenPragmas(t *testing.T) {
	db := openTestDB(t)

	var journal string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if !strings.EqualFold(journal, "wal") {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

// TestOpenUnavailableDriver covers the requirement that a configured but
// uncompiled driver reports a clear error instead of silently falling back to
// SQLite or returning a non-functional handle.
func TestOpenUnavailableDriver(t *testing.T) {
	tests := []struct {
		name   string
		driver string
	}{
		{name: "postgres", driver: DriverPostgres},
		{name: "mysql", driver: DriverMySQL},
		{name: "mssql", driver: DriverMSSQL},
		{name: "mongodb", driver: DriverMongoDB},
		{name: "unknown", driver: "cassandra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Database{Type: tt.driver}
			db, err := Open(cfg, t.TempDir(), "test.db", "test")
			if err == nil {
				_ = db.Close()
				t.Fatal("Open succeeded, want ErrDriverUnavailable")
			}
			if !errors.Is(err, ErrDriverUnavailable) {
				t.Errorf("Open error = %v, want ErrDriverUnavailable", err)
			}
		})
	}
}

// TestOpenErrorOmitsCredentials guards the rule that a connection error never
// leaks the configured password into a log line.
func TestOpenErrorOmitsCredentials(t *testing.T) {
	cfg := config.Database{
		Type:     DriverPostgres,
		Host:     "db.internal",
		User:     "redxt",
		Password: "hunter2-super-secret",
		URL:      "postgres://redxt:hunter2-super-secret@db.internal/redxt",
	}
	_, err := Open(cfg, t.TempDir(), "test.db", "test")
	if err == nil {
		t.Fatal("Open succeeded, want error")
	}
	msg := err.Error()
	for _, leak := range []string{cfg.Password, cfg.URL, cfg.Host, cfg.User} {
		if strings.Contains(msg, leak) {
			t.Errorf("error message leaks %q: %s", leak, msg)
		}
	}
}

func TestOpenServerAndUsers(t *testing.T) {
	dir := t.TempDir()

	server, err := OpenServer(sqliteConfig(), dir)
	if err != nil {
		t.Fatalf("OpenServer: %v", err)
	}
	defer func() { _ = server.Close() }()

	users, err := OpenUsers(sqliteConfig(), dir)
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	defer func() { _ = users.Close() }()

	if want := filepath.Join(dir, ServerDBFile); server.Path() != want {
		t.Errorf("server path = %q, want %q", server.Path(), want)
	}
	if want := filepath.Join(dir, UsersDBFile); users.Path() != want {
		t.Errorf("users path = %q, want %q", users.Path(), want)
	}
	if server.Path() == users.Path() {
		t.Error("server and users databases share a file")
	}
}

// TestSQLiteDSN checks that the DSN is a well-formed URL carrying exactly the
// three required pragmas, and that a path containing URL metacharacters is
// escaped rather than being able to smuggle in a fourth pragma.
func TestSQLiteDSN(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "plain", path: "/var/lib/redxt/server.db"},
		{name: "spaces", path: "/var/lib/my data/server.db"},
		{name: "query metachar", path: "/var/lib/x?_pragma=synchronous(OFF)/server.db"},
		{name: "fragment metachar", path: "/var/lib/x#frag/server.db"},
		{name: "ampersand", path: "/var/lib/a&b/server.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := sqliteDSN(tt.path)

			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", dsn, err)
			}
			if u.Path != tt.path {
				t.Errorf("parsed path = %q, want %q", u.Path, tt.path)
			}

			q, err := url.ParseQuery(u.RawQuery)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			pragmas := q["_pragma"]
			if len(pragmas) != 3 {
				t.Fatalf("got %d pragmas %v, want 3", len(pragmas), pragmas)
			}
			want := map[string]bool{
				"journal_mode(WAL)": false,
				"foreign_keys(1)":   false,
			}
			for _, p := range pragmas {
				if _, ok := want[p]; ok {
					want[p] = true
				}
				if strings.Contains(p, "synchronous") {
					t.Errorf("path smuggled a pragma: %q", p)
				}
			}
			for p, seen := range want {
				if !seen {
					t.Errorf("missing pragma %q in %v", p, pragmas)
				}
			}
		})
	}
}

// fakePool records the pool settings applied to it.
type fakePool struct {
	maxOpen     int
	maxIdle     int
	maxLifetime time.Duration
	maxIdleTime time.Duration
}

func (f *fakePool) SetMaxOpenConns(n int)              { f.maxOpen = n }
func (f *fakePool) SetMaxIdleConns(n int)              { f.maxIdle = n }
func (f *fakePool) SetConnMaxLifetime(d time.Duration) { f.maxLifetime = d }
func (f *fakePool) SetConnMaxIdleTime(d time.Duration) { f.maxIdleTime = d }

func TestApplyPool(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.Database
		wantMaxOpen     int
		wantMaxIdle     int
		wantMaxLifetime time.Duration
		wantMaxIdleTime time.Duration
	}{
		{
			name:            "defaults on empty config",
			cfg:             config.Database{},
			wantMaxOpen:     DefaultMaxOpenConns,
			wantMaxIdle:     DefaultMaxIdleConns,
			wantMaxLifetime: DefaultConnMaxLifetime,
			wantMaxIdleTime: DefaultConnMaxIdleTime,
		},
		{
			name: "configured values win",
			cfg: config.Database{
				MaxOpenConns:    40,
				MaxIdleConns:    9,
				ConnMaxLifetime: config.Duration(10 * time.Minute),
				ConnMaxIdleTime: config.Duration(2 * time.Minute),
			},
			wantMaxOpen:     40,
			wantMaxIdle:     9,
			wantMaxLifetime: 10 * time.Minute,
			wantMaxIdleTime: 2 * time.Minute,
		},
		{
			name:            "negative values fall back to defaults",
			cfg:             config.Database{MaxOpenConns: -1, MaxIdleConns: -1},
			wantMaxOpen:     DefaultMaxOpenConns,
			wantMaxIdle:     DefaultMaxIdleConns,
			wantMaxLifetime: DefaultConnMaxLifetime,
			wantMaxIdleTime: DefaultConnMaxIdleTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p fakePool
			applyPool(&p, tt.cfg)
			if p.maxOpen != tt.wantMaxOpen {
				t.Errorf("maxOpen = %d, want %d", p.maxOpen, tt.wantMaxOpen)
			}
			if p.maxIdle != tt.wantMaxIdle {
				t.Errorf("maxIdle = %d, want %d", p.maxIdle, tt.wantMaxIdle)
			}
			if p.maxLifetime != tt.wantMaxLifetime {
				t.Errorf("maxLifetime = %v, want %v", p.maxLifetime, tt.wantMaxLifetime)
			}
			if p.maxIdleTime != tt.wantMaxIdleTime {
				t.Errorf("maxIdleTime = %v, want %v", p.maxIdleTime, tt.wantMaxIdleTime)
			}
		})
	}
}

// TestCloseNil covers the nil-safe Close, which lets a shutdown path close
// both handles without first checking whether each one was opened.
func TestCloseNil(t *testing.T) {
	var db *DB
	if err := db.Close(); err != nil {
		t.Errorf("Close on nil DB = %v, want nil", err)
	}
}
