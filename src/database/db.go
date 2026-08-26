// Package database implements AI.md PART 10 (DATABASE & CLUSTER) for redxt:
// pooled connections, the idempotent self-creating schema, query helpers with
// the PART 10 timeout budgets, the app_secrets row layer, and the cluster
// heartbeat and primary-election model.
//
// Three rules from PART 10 govern everything here:
//
//   - The schema creates itself on every start. It is always idempotent, never
//     destructive, and add-only. There are no migration files and no version
//     tracking table.
//   - Every query carries a timeout. The class-specific budgets live in
//     query.go.
//   - State is split across two SQLite files: server.db holds server and
//     instance state, users.db holds accounts, organizations, and the
//     org-owned DNS data. The split follows the PART 5 "Full Database Schema
//     Summary" rule.
//
// SQLite is the only driver compiled into this build. It is pure Go
// (modernc.org/sqlite), so the binary stays CGO_ENABLED=0. PostgreSQL, MySQL,
// and the other types config accepts are a later part; Open rejects them with
// a clear error rather than pretending to support them.
//
// SECURITY: every statement in this package binds user-controlled values as
// parameters. The only strings interpolated into SQL are compile-time constant
// table and column names inside the schema DDL.
package database

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// Driver names accepted by Open. Only DriverSQLite is available in this build.
const (
	// DriverSQLite is the pure-Go SQLite driver registered by
	// modernc.org/sqlite. The registered database/sql driver name is
	// "sqlite", not "sqlite3".
	DriverSQLite = "sqlite"
	// DriverPostgres names the PostgreSQL driver from server.yml.
	DriverPostgres = "postgres"
	// DriverMySQL names the MySQL/MariaDB driver from server.yml.
	DriverMySQL = "mysql"
	// DriverMSSQL names the Microsoft SQL Server driver from server.yml.
	DriverMSSQL = "mssql"
	// DriverLibSQL names the libSQL driver from server.yml.
	DriverLibSQL = "libsql"
	// DriverMongoDB names the MongoDB driver from server.yml.
	DriverMongoDB = "mongodb"
)

// Database file names for the two SQLite stores mandated by PART 10.
const (
	// ServerDBFile holds server and instance state: secrets, configuration,
	// tokens, admin sessions, cluster membership, rate limiting, auditing,
	// scheduling, and the instance-wide DNS policy objects.
	ServerDBFile = "server.db"
	// UsersDBFile holds accounts, organizations, and every org-owned record:
	// zones, records, keys, DDNS hosts, redirects, and query logs.
	UsersDBFile = "users.db"
)

// DB is a pooled handle to one of the application's databases.
//
// It embeds *sql.DB so callers may use the standard library directly, and adds
// the driver and store name so helpers can adapt statements and error messages
// without re-reading the configuration.
type DB struct {
	*sql.DB

	// driver is the registered database/sql driver name.
	driver string
	// name is the logical store name, "server" or "users".
	name string
	// path is the resolved on-disk location for a file-backed driver. It is
	// empty for network drivers.
	path string
}

// Driver returns the registered database/sql driver name backing this handle.
func (d *DB) Driver() string {
	return d.driver
}

// Name returns the logical store name, "server" or "users".
func (d *DB) Name() string {
	return d.name
}

// Path returns the on-disk file backing this handle, or an empty string for a
// network-backed driver.
func (d *DB) Path() string {
	return d.path
}

// IsSQLite reports whether this handle is backed by the pure-Go SQLite driver.
func (d *DB) IsSQLite() bool {
	return d.driver == DriverSQLite
}

// Close releases the pool. It is safe to call on a nil handle so shutdown
// paths do not need to guard a database that never opened.
func (d *DB) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	return d.DB.Close()
}
