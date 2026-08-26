package middleware

import (
	"context"
	"database/sql"
	"time"
)

// SQL statements backing SQLStore. They are plain UPDATE-then-INSERT
// rather than an upsert because ON CONFLICT is SQLite and PostgreSQL
// syntax: PART 9 requires every driver redxt supports — MySQL, MSSQL and
// the rest — to work from the same statements.
const (
	// selectRateWindows reads the current and previous window counters
	// for one identifier and bucket.
	selectRateWindows = `SELECT window_start, count FROM rate_limits
		WHERE identifier = ? AND bucket = ? AND window_start IN (?, ?)`
	// incrementRateWindow bumps an existing window counter.
	incrementRateWindow = `UPDATE rate_limits SET count = count + 1, updated_at = ?
		WHERE identifier = ? AND bucket = ? AND window_start = ?`
	// insertRateWindow opens a new window counter at one request.
	insertRateWindow = `INSERT INTO rate_limits (identifier, bucket, window_start, count, updated_at)
		VALUES (?, ?, ?, 1, ?)`
	// purgeRateWindows deletes counters for windows that can no longer
	// influence any verdict.
	purgeRateWindows = `DELETE FROM rate_limits WHERE window_start < ?`
)

// SQLStore is the cluster-wide rate-limit store. It keeps the sliding
// windows in the rate_limits table of server.db, so every node in a
// PART 10 cluster enforces one shared budget per client rather than one
// budget each.
//
// It is a weighted sliding-window counter, not a request log: each
// window holds a single count, and the previous window contributes in
// proportion to how much of it still falls inside the sliding period.
// That costs two rows per client per bucket instead of one row per
// request, which is what makes the table's size a function of active
// clients rather than of traffic.
type SQLStore struct {
	// db is the server database handle.
	db *sql.DB
	// timeout bounds every statement, so a stalled database delays a
	// request by a known amount instead of indefinitely.
	timeout time.Duration
}

// DefaultRateLimitTimeout bounds each rate-limit statement.
const DefaultRateLimitTimeout = 2 * time.Second

// NewSQLStore returns a store over db. A zero timeout selects
// DefaultRateLimitTimeout.
func NewSQLStore(db *sql.DB, timeout time.Duration) *SQLStore {
	if timeout <= 0 {
		timeout = DefaultRateLimitTimeout
	}
	return &SQLStore{db: db, timeout: timeout}
}

// Allow counts one request against a bucket and returns the verdict.
//
// A rejected request is not counted. Counting it would let a client that
// keeps retrying against a closed window push its own reopening time
// further out with every attempt, turning a rate limit into an
// escalating lockout.
func (s *SQLStore) Allow(identifier, bucket string, limit int, window time.Duration, now time.Time) (Decision, error) {
	if s == nil || s.db == nil || limit <= 0 || window <= 0 {
		return Decision{Allowed: true, Bucket: bucket, Limit: limit}, nil
	}

	now = now.UTC()
	current := now.Truncate(window)
	previous := current.Add(-window)
	elapsed := now.Sub(current)

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Decision{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	currentCount, previousCount, err := readWindows(ctx, tx, identifier, bucket, current, previous)
	if err != nil {
		return Decision{}, err
	}

	weight := 1 - float64(elapsed)/float64(window)
	estimate := float64(previousCount)*weight + float64(currentCount)
	if estimate+1 > float64(limit) {
		retry := window - elapsed
		if retry < 0 {
			retry = 0
		}
		return Decision{Allowed: false, Bucket: bucket, Limit: limit, RetryAfter: retry}, nil
	}

	if err := countRequest(ctx, tx, identifier, bucket, current, now); err != nil {
		return Decision{}, err
	}
	if err := tx.Commit(); err != nil {
		return Decision{}, err
	}

	remaining := limit - int(estimate) - 1
	if remaining < 0 {
		remaining = 0
	}
	return Decision{Allowed: true, Bucket: bucket, Limit: limit, Remaining: remaining}, nil
}

// Purge deletes every counter for a window that started before cutoff.
// The PART 19 scheduler runs it so the table tracks active clients
// rather than growing without bound.
func (s *SQLStore) Purge(cutoff time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	_, err := s.db.ExecContext(ctx, purgeRateWindows, cutoff.UTC())
	return err
}

// readWindows returns the counts recorded for the current and previous
// windows. A window with no row counts as zero.
func readWindows(ctx context.Context, tx *sql.Tx, identifier, bucket string, current, previous time.Time) (int, int, error) {
	rows, err := tx.QueryContext(ctx, selectRateWindows, identifier, bucket, current, previous)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		_ = rows.Close()
	}()

	currentCount, previousCount := 0, 0
	for rows.Next() {
		var start time.Time
		var count int
		if err := rows.Scan(&start, &count); err != nil {
			return 0, 0, err
		}
		if start.UTC().Equal(current) {
			currentCount = count
			continue
		}
		previousCount = count
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	return currentCount, previousCount, nil
}

// countRequest records one request in the current window, opening the
// window's row when this is the first request to land in it.
func countRequest(ctx context.Context, tx *sql.Tx, identifier, bucket string, current, now time.Time) error {
	result, err := tx.ExecContext(ctx, incrementRateWindow, now, identifier, bucket, current)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected > 0 {
		return nil
	}

	_, err = tx.ExecContext(ctx, insertRateWindow, identifier, bucket, current, now)
	return err
}
