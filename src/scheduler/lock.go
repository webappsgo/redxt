package scheduler

import (
	"context"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// lockTTL is how long a cluster lock is held before it is considered
// abandoned and reclaimable by another node, in case the holder
// crashed mid-task without releasing it.
const lockTTL = 5 * time.Minute

// acquireLock attempts to take the named cluster_locks row for
// holder. It reports true only when this call is the one that took
// or already owns the lock.
//
// The single INSERT ... ON CONFLICT DO UPDATE ... WHERE statement is
// atomic under SQLite's single-writer model: the WHERE clause only
// lets the update through when the row is unheld, already owned by
// this holder, or its TTL has expired, so two nodes racing to run the
// same task can never both win.
func acquireLock(ctx context.Context, db *database.DB, name, holder string, now time.Time) (bool, error) {
	expires := now.Add(lockTTL)
	res, err := database.ExecContext(ctx, db, database.TimeoutWrite, `
		INSERT INTO cluster_locks (name, holder, acquired_at, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			holder      = excluded.holder,
			acquired_at = excluded.acquired_at,
			expires_at  = excluded.expires_at
		WHERE cluster_locks.holder = excluded.holder
		   OR cluster_locks.expires_at IS NULL
		   OR cluster_locks.expires_at < excluded.acquired_at
	`, name, holder, now, expires)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// releaseLock drops the lock row, but only if it is still held by
// holder, so a node whose lock already expired and was reclaimed by
// someone else never releases the new holder's lock out from under it.
func releaseLock(ctx context.Context, db *database.DB, name, holder string) error {
	_, err := database.ExecContext(ctx, db, database.TimeoutWrite,
		`DELETE FROM cluster_locks WHERE name = ? AND holder = ?`, name, holder)
	return err
}
