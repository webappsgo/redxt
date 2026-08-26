package database

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTransaction runs fn inside a transaction bounded by the PART 10
// 30-second transaction timeout.
//
// The transaction commits when fn returns nil and rolls back when it returns
// an error. If fn panics, the transaction is rolled back first and the panic
// is then re-raised unchanged, so a bug in a callback can never leave a
// SQLite write lock held for the life of the process.
//
// A rollback failure is deliberately not reported over fn's error: the caller
// needs to see why the work failed, not that the cleanup after it also failed.
//
// The context handed to fn carries the transaction deadline, so statements
// inside fn should use it directly rather than creating their own.
func WithTransaction(ctx context.Context, db *DB, fn func(*sql.Tx) error) error {
	return WithTransactionOpts(ctx, db, nil, fn)
}

// WithTransactionOpts is WithTransaction with explicit transaction options,
// for the cases PART 10 calls out that need serializable isolation or a
// read-only transaction.
func WithTransactionOpts(ctx context.Context, db *DB, opts *sql.TxOptions, fn func(*sql.Tx) error) error {
	ctx, cancel := withTimeout(ctx, TimeoutTransaction)
	defer cancel()

	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("database: begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		// This runs for both the error return below and an unwound panic.
		// Rolling back here is what guarantees the lock is released before
		// the panic continues up the stack.
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("database: commit transaction: %w", err)
	}
	committed = true
	return nil
}

// InTransaction runs fn inside a transaction and maps any failure onto the
// PART 9 error taxonomy, for callers that answer an API request directly.
func InTransaction(ctx context.Context, db *DB, fn func(*sql.Tx) error) error {
	if err := WithTransaction(ctx, db, fn); err != nil {
		return HandleQueryError(err)
	}
	return nil
}
