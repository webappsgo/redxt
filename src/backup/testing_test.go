package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/paths"

	// modernc.org/sqlite registers the "sqlite" database/sql driver used by
	// writeTestSQLiteDB below to build real, integrity-check-passing
	// database files for the tests in this package.
	_ "modernc.org/sqlite"
)

// writeTestSQLiteDB creates a real, valid SQLite database file at path
// containing a single table with one row. Verify's database_integrity
// check runs PRAGMA integrity_check via the same pure-Go driver, so the
// fixture files Service.CreateManual/RunDaily/RunHourly back up must be
// genuine SQLite files, not arbitrary bytes, for that check to pass.
func writeTestSQLiteDB(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open(database.DriverSQLite, path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatalf("create table in %s: %v", path, err)
	}
	if _, err := db.Exec("INSERT INTO t (v) VALUES (?)", filepath.Base(path)); err != nil {
		t.Fatalf("insert into %s: %v", path, err)
	}
}

// newTestPaths builds a minimal, real on-disk paths.Paths inside t.TempDir()
// with the three files collectEntries always requires already present, so
// Service.CreateManual/RunDaily/RunHourly can run against it unmodified.
func newTestPaths(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()

	cfgDir := filepath.Join(root, "config")
	dbDir := filepath.Join(root, "db")
	backupDir := filepath.Join(root, "backup")
	sslDir := filepath.Join(root, "ssl")
	dataDir := filepath.Join(root, "data")
	for _, d := range []string{cfgDir, dbDir, backupDir, sslDir, dataDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	cfgFile := filepath.Join(cfgDir, "server.yml")
	if err := os.WriteFile(cfgFile, []byte("port: 8080\n"), 0o640); err != nil {
		t.Fatalf("write server.yml: %v", err)
	}
	writeTestSQLiteDB(t, filepath.Join(dbDir, database.ServerDBFile))
	writeTestSQLiteDB(t, filepath.Join(dbDir, database.UsersDBFile))

	return paths.Paths{
		Config:     cfgDir,
		ConfigFile: cfgFile,
		Data:       dataDir,
		Backup:     backupDir,
		SSL:        sslDir,
		DB:         dbDir,
	}
}
