package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// Connection-pool defaults from PART 10 "Pool Sizing Guidelines" for a small
// deployment of one or two nodes. A configured value always wins; these apply
// only when server.yml leaves the setting at zero or negative.
const (
	// DefaultMaxOpenConns caps total open connections.
	DefaultMaxOpenConns = 25
	// DefaultMaxIdleConns caps idle connections held in the pool.
	DefaultMaxIdleConns = 5
	// DefaultConnMaxLifetime retires a connection after this age.
	DefaultConnMaxLifetime = 5 * time.Minute
	// DefaultConnMaxIdleTime retires an idle connection after this long.
	DefaultConnMaxIdleTime = time.Minute
)

// PingTimeout bounds the connectivity check performed after opening a pool,
// per the PART 10 NewDB implementation.
const PingTimeout = 5 * time.Second

// Directory and file permissions for a file-backed database. The database
// holds hashed credentials and encrypted key material, so it is never
// world-readable.
const (
	// DirMode is the mode of the database directory.
	DirMode os.FileMode = 0o750
	// FileMode is the mode of a created database file.
	FileMode os.FileMode = 0o640
)

// SQLiteBusyTimeout is the busy_timeout pragma value applied to every SQLite
// connection, in milliseconds. Writes serialize under WAL, so a writer that
// finds the database locked waits this long before returning SQLITE_BUSY.
const SQLiteBusyTimeout = 5000

// ErrDriverUnavailable reports a configured database type that this build
// cannot open. Only SQLite is compiled in; the remote drivers land in a later
// part and this package refuses to pretend otherwise.
var ErrDriverUnavailable = errors.New("database: driver not available in this build")

// OpenServer opens server.db inside dir: the server and instance state store.
func OpenServer(cfg config.Database, dir string) (*DB, error) {
	return Open(cfg, dir, ServerDBFile, "server")
}

// OpenUsers opens users.db inside dir: the accounts, organizations, and
// org-owned DNS data store.
func OpenUsers(cfg config.Database, dir string) (*DB, error) {
	return Open(cfg, dir, UsersDBFile, "users")
}

// Open creates a pooled handle for one of the application's databases.
//
// dir is the SQLite directory, normally paths.Paths.DB; file is the SQLite
// file name; name is the logical store name recorded on the handle. The
// directory is created 0750 and a newly created database file is 0640.
//
// Pool limits come from cfg, falling back to the PART 10 defaults for any
// value left unset. A 5-second PingContext verifies the connection; a failed
// ping is an error and the pool is closed before returning.
//
// A configured type other than sqlite returns ErrDriverUnavailable. The
// connection string in cfg.URL, cfg.Host, cfg.User, and cfg.Password is never
// echoed into an error, so a password in server.yml cannot leak through a
// startup failure message.
func Open(cfg config.Database, dir, file, name string) (*DB, error) {
	driver := cfg.Type
	if driver == "" {
		driver = DriverSQLite
	}
	if driver != DriverSQLite {
		return nil, fmt.Errorf("database: type %q: %w", driver, ErrDriverUnavailable)
	}

	path, err := prepareFile(dir, file)
	if err != nil {
		return nil, err
	}

	handle, err := sql.Open(DriverSQLite, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("database: open %s: %w", name, err)
	}

	applyPool(handle, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), PingTimeout)
	defer cancel()
	if err := handle.PingContext(ctx); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("database: ping %s: %w", name, err)
	}

	return &DB{DB: handle, driver: DriverSQLite, name: name, path: path}, nil
}

// prepareFile creates the database directory and, when the database does not
// yet exist, the file itself with the restrictive mode. Creating the file here
// rather than letting SQLite create it is what guarantees the 0640 mode: the
// driver would otherwise create it subject to the process umask.
func prepareFile(dir, file string) (string, error) {
	if dir == "" {
		return "", errors.New("database: empty database directory")
	}
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return "", fmt.Errorf("database: create %s: %w", dir, err)
	}
	path := filepath.Join(dir, file)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, FileMode)
	if err != nil {
		return "", fmt.Errorf("database: create %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("database: create %s: %w", path, err)
	}
	return path, nil
}

// sqliteDSN builds the modernc.org/sqlite connection string for path.
//
// Each _pragma value is executed verbatim as a PRAGMA statement by the driver,
// in the order given:
//
//   - journal_mode(WAL) enables write-ahead logging so readers never block the
//     writer. It is persistent, but it is set on every connection because a
//     freshly created database starts in rollback-journal mode.
//   - foreign_keys(1) enforces the referential integrity the schema declares.
//     SQLite defaults this OFF per connection, so it must be set every time.
//   - busy_timeout blocks a contended writer instead of failing immediately.
//
// The path is carried in the URL Path component so that a directory name
// containing a query or fragment character cannot terminate the DSN early and
// smuggle in extra pragmas.
func sqliteDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", SQLiteBusyTimeout))
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}

// applyPool applies the PART 10 pool settings, substituting the documented
// default for any value server.yml left unset.
//
// NOTE for SQLite: a MaxOpenConns above one is correct and useful under WAL —
// readers run concurrently with the writer. Writes still serialize inside
// SQLite, so the surplus connections buy read concurrency, not write
// throughput. The configured value is honoured as-is rather than being clamped
// to one, because the same setting governs the remote drivers.
func applyPool(handle poolSetter, cfg config.Database) {
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = DefaultMaxOpenConns
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = DefaultMaxIdleConns
	}
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	lifetime := cfg.ConnMaxLifetime.Duration()
	if lifetime <= 0 {
		lifetime = DefaultConnMaxLifetime
	}
	idleTime := cfg.ConnMaxIdleTime.Duration()
	if idleTime <= 0 {
		idleTime = DefaultConnMaxIdleTime
	}

	handle.SetMaxOpenConns(maxOpen)
	handle.SetMaxIdleConns(maxIdle)
	handle.SetConnMaxLifetime(lifetime)
	handle.SetConnMaxIdleTime(idleTime)
}

// poolSetter is the subset of *sql.DB that applyPool needs, so the pool
// resolution can be tested without opening a database.
type poolSetter interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
	SetConnMaxIdleTime(time.Duration)
}
