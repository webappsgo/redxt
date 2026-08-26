package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/webappsgo/redxt/src/apierror"
)

// Query timeout budgets from the PART 10 "Timeout Configuration" table. Every
// query in the application runs under one of these.
const (
	// TimeoutSimple bounds a single-table SELECT.
	TimeoutSimple = 5 * time.Second
	// TimeoutComplex bounds a multi-table SELECT with joins.
	TimeoutComplex = 15 * time.Second
	// TimeoutWrite bounds an INSERT, UPDATE, or DELETE.
	TimeoutWrite = 10 * time.Second
	// TimeoutBulk bounds a bulk import or export over a large data set.
	TimeoutBulk = 60 * time.Second
	// TimeoutMigration bounds a schema run.
	TimeoutMigration = 5 * time.Minute
	// TimeoutReport bounds an aggregation or analytics query.
	TimeoutReport = 2 * time.Minute
	// TimeoutTransaction bounds a WithTransaction unit of work.
	TimeoutTransaction = 30 * time.Second
)

// QueryContext runs a multi-row query under the supplied timeout, defaulting
// to the PART 10 simple-SELECT budget when timeout is not positive.
//
// If ctx already carries a deadline, that deadline is respected unchanged: a
// caller that has budgeted the whole request is never handed a longer one by a
// helper. Only a context without a deadline receives the class timeout.
//
// The returned cancel func releases the deadline and MUST be called once the
// caller is finished with the rows. It cannot be released here, because the
// rows are scanned after this function returns and cancelling the context
// would abort that scan. The usual shape is:
//
//	rows, cancel, err := database.QueryContext(ctx, db, database.TimeoutSimple, q, id)
//	if err != nil {
//		return err
//	}
//	defer cancel()
//	defer rows.Close()
//
// args are bound as query parameters. Never build the query string by
// concatenating a value into it.
func QueryContext(ctx context.Context, db *DB, timeout time.Duration, query string, args ...any) (*sql.Rows, context.CancelFunc, error) {
	if timeout <= 0 {
		timeout = TimeoutSimple
	}
	ctx, cancel := withTimeout(ctx, timeout)
	rows, err := db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	return rows, cancel, nil
}

// QueryRowContext runs a single-row query under the supplied timeout,
// defaulting to the PART 10 simple-SELECT budget when timeout is not positive,
// and hands the row to scan.
//
// The deadline is released once scan has run, which is why this helper takes a
// scan callback rather than returning a *sql.Row: a returned Row would be
// scanned after the deadline had already been cancelled.
//
// args are bound as query parameters.
func QueryRowContext(ctx context.Context, db *DB, timeout time.Duration, scan func(*sql.Row) error, query string, args ...any) error {
	if timeout <= 0 {
		timeout = TimeoutSimple
	}
	ctx, cancel := withTimeout(ctx, timeout)
	defer cancel()
	return scan(db.DB.QueryRowContext(ctx, query, args...))
}

// ExecContext runs a statement under the supplied timeout, defaulting to the
// PART 10 write budget when timeout is not positive.
//
// args are bound as query parameters.
func ExecContext(ctx context.Context, db *DB, timeout time.Duration, query string, args ...any) (sql.Result, error) {
	if timeout <= 0 {
		timeout = TimeoutWrite
	}
	ctx, cancel := withTimeout(ctx, timeout)
	defer cancel()
	return db.DB.ExecContext(ctx, query, args...)
}

// withTimeout applies timeout to ctx unless ctx already has a deadline, in
// which case the existing deadline stands. The returned cancel is always safe
// to call.
func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// HandleQueryError maps a database error onto the PART 9 error taxonomy,
// implementing the PART 10 handleQueryError table.
//
//	sql.ErrNoRows            -> NOT_FOUND
//	context.DeadlineExceeded -> SERVER_ERROR, timeout message
//	context.Canceled         -> SERVER_ERROR, canceled message
//	anything else            -> SERVER_ERROR wrapping the driver error
//
// PART 10 names a distinct TIMEOUT and CANCELED class, but the PART 9
// error-code table this project's clients switch on has no code for either.
// Rather than invent a code that no client knows, both map to
// apierror.CodeServerError and are distinguished by their user-facing message.
// A timeout is a server-side failure, so the 500 status that code carries is
// the correct answer either way.
//
// The driver error is attached as the Internal cause, which apierror never
// serializes into a response — it reaches the logs only. That matters here
// because a driver error can quote the failing statement, and a statement can
// quote a bound value.
//
// A nil error returns nil.
func HandleQueryError(err error) *apierror.Error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return apierror.Wrap(apierror.CodeNotFound, err)
	case errors.Is(err, context.DeadlineExceeded):
		e := apierror.Wrap(apierror.CodeServerError, err)
		e.Message = "Database request timed out"
		return e
	case errors.Is(err, context.Canceled):
		e := apierror.Wrap(apierror.CodeServerError, err)
		e.Message = "Database request was canceled"
		return e
	default:
		return apierror.Wrap(apierror.CodeServerError, err)
	}
}

// IsNotFound reports whether err is, or wraps, sql.ErrNoRows.
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// TimeLayout is the textual form every timestamp is written in.
//
// It is SQLite's own CURRENT_TIMESTAMP format, in UTC. Writing bound times in
// exactly this layout keeps application-written timestamps lexically
// comparable with database-generated defaults, which is what makes an ordinary
// "WHERE last_seen < ?" comparison correct. Handing the driver a time.Time
// instead would store an RFC 3339 string with an offset, and the two forms do
// not sort against each other.
const TimeLayout = "2006-01-02 15:04:05"

// FormatTime renders t for storage in a TIMESTAMP column.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// NullTime renders t for storage, mapping the zero time to SQL NULL.
func NullTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: FormatTime(t), Valid: true}
}
