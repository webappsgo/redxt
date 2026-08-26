// Package store is the data-access layer for AI.md PART 34 (Multi-User),
// PART 35 (Organizations), and PART 36 (Custom Domains).
//
// Every method takes a context, runs under one of the PART 10 query
// timeout classes, and binds all user input as query parameters; no
// query in this package is assembled by concatenating a caller-supplied
// value into SQL.
//
// The store enforces no policy. It reads and writes rows, reports what
// it found, and leaves every authorization decision to the service
// layer, so there is exactly one place where a permission is checked.
package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// Store reads and writes the users.db tables. It never touches
// server.db: Regular Users, organizations, and their credentials live
// in their own database, separate from the server's operational state.
type Store struct {
	db *database.DB
}

// New returns a Store backed by an open users.db handle.
func New(db *database.DB) *Store {
	return &Store{db: db}
}

// DB exposes the underlying handle for a caller that needs to run its
// own transaction across several store methods.
func (s *Store) DB() *database.DB {
	return s.db
}

var (
	// ErrNotFound reports that no row matched. The service layer maps it
	// to a 404, or, on a login path, to the same generic failure a wrong
	// password produces.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict reports a uniqueness violation: a username, email,
	// slug, or domain that is already taken.
	ErrConflict = errors.New("store: already exists")
)

// notFound maps a driver's no-rows error onto ErrNotFound and leaves
// every other error untouched.
func notFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) || database.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// now returns the current UTC time truncated to the second, which is
// the resolution the TIMESTAMP columns store.
func now() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

// nullTime renders a time for a nullable TIMESTAMP column, writing SQL
// NULL for the zero time rather than a year-one date.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return database.FormatTime(t.UTC())
}

// nullInt renders an identifier for a nullable INTEGER column, writing
// SQL NULL for zero so an absent reference is stored as absent rather
// than as a foreign key pointing at row zero.
func nullInt(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// scanInt reads a nullable INTEGER identifier, treating SQL NULL as zero.
func scanInt(v sql.NullInt64) int64 {
	if !v.Valid {
		return 0
	}
	return v.Int64
}

// boolInt converts a Go bool to the INTEGER the schema stores.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
