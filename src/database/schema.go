package database

import (
	"context"
	"fmt"
	"strings"
)

// EnsureServerSchema creates and updates every server.db table.
//
// It implements the PART 10 self-creating schema: safe to run on every start,
// safe to run concurrently from several cluster nodes, and never destructive.
// Nothing here drops a table, drops a column, or deletes a row.
func EnsureServerSchema(ctx context.Context, db *DB) error {
	return ensureSchema(ctx, db, "server", serverTables, serverUpdates)
}

// EnsureUsersSchema creates and updates every users.db table.
//
// Same contract as EnsureServerSchema: idempotent, add-only, and safe to run
// from every node on every start.
func EnsureUsersSchema(ctx context.Context, db *DB) error {
	return ensureSchema(ctx, db, "users", usersTables, usersUpdates)
}

// ensureSchema runs the two PART 10 phases against one database.
//
// Phase one creates the base objects inside a transaction, so a partially
// created schema is never left behind by a crash mid-startup. SQLite executes
// DDL transactionally, and the CREATE ... IF NOT EXISTS form makes a second
// node's identical transaction a no-op rather than a conflict.
//
// Phase two applies the additive updates one statement at a time OUTSIDE that
// transaction. This is deliberate: an ALTER TABLE ADD COLUMN for a column that
// already exists is the expected outcome on every start after the first, and
// swallowing that error inside a transaction would still leave the surrounding
// batch at the mercy of drivers that abort a transaction on any statement
// error. Run individually, an ignorable duplicate-column error costs nothing.
//
// The whole run is bounded by TimeoutMigration when the caller's context has
// no deadline of its own.
func ensureSchema(ctx context.Context, db *DB, name string, tables, updates []string) error {
	ctx, cancel := withTimeout(ctx, TimeoutMigration)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("database: %s schema: begin: %w", name, err)
	}
	for _, stmt := range tables {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("database: %s schema: create table: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: %s schema: commit: %w", name, err)
	}

	for _, stmt := range updates {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			if isColumnExistsError(err) {
				continue
			}
			return fmt.Errorf("database: %s schema: update: %w", name, err)
		}
	}

	return nil
}

// isColumnExistsError reports whether err is a database's way of saying the
// column or object an additive schema update tried to add is already there.
//
// This is the PART 10 pattern that makes ALTER TABLE ADD COLUMN idempotent
// without a migration table. The comparison is case-insensitive so the same
// check covers every driver's capitalisation:
//
//   - SQLite:     "duplicate column name: x"
//   - PostgreSQL: "column \"x\" of relation \"y\" already exists"
//   - MySQL:      "Duplicate column name 'x'" (error 1060)
func isColumnExistsError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}
