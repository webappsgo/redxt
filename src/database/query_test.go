package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/apierror"
)

// TestTimeoutBudgets pins the PART 10 timeout table. These are contract
// values: changing one silently changes how long a stuck query holds a
// connection, so a change must be deliberate enough to update this test.
func TestTimeoutBudgets(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "simple", got: TimeoutSimple, want: 5 * time.Second},
		{name: "complex", got: TimeoutComplex, want: 15 * time.Second},
		{name: "write", got: TimeoutWrite, want: 10 * time.Second},
		{name: "bulk", got: TimeoutBulk, want: 60 * time.Second},
		{name: "migration", got: TimeoutMigration, want: 5 * time.Minute},
		{name: "report", got: TimeoutReport, want: 2 * time.Minute},
		{name: "transaction", got: TimeoutTransaction, want: 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("timeout = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

// TestWithTimeoutRespectsCallerDeadline is the rule that keeps a request's own
// budget authoritative: a helper may never extend a deadline the caller
// already set.
func TestWithTimeoutRespectsCallerDeadline(t *testing.T) {
	caller, cancelCaller := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelCaller()
	callerDeadline, _ := caller.Deadline()

	ctx, cancel := withTimeout(caller, time.Hour)
	defer cancel()

	got, ok := ctx.Deadline()
	if !ok {
		t.Fatal("derived context has no deadline")
	}
	if !got.Equal(callerDeadline) {
		t.Errorf("deadline = %v, want caller deadline %v", got, callerDeadline)
	}
}

func TestWithTimeoutAppliesClassBudget(t *testing.T) {
	before := time.Now()
	ctx, cancel := withTimeout(context.Background(), time.Minute)
	defer cancel()

	got, ok := ctx.Deadline()
	if !ok {
		t.Fatal("derived context has no deadline")
	}
	if got.Before(before.Add(30*time.Second)) || got.After(before.Add(90*time.Second)) {
		t.Errorf("deadline = %v, want roughly one minute after %v", got, before)
	}
}

// TestWithTimeoutNonPositive covers the zero case: no deadline is invented,
// but the returned cancel is still safe to call.
func TestWithTimeoutNonPositive(t *testing.T) {
	ctx, cancel := withTimeout(context.Background(), 0)
	defer cancel()
	if _, ok := ctx.Deadline(); ok {
		t.Error("zero timeout produced a deadline")
	}
}

func TestHandleQueryError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantNil  bool
		wantCode string
		wantMsg  string
	}{
		{name: "nil", err: nil, wantNil: true},
		{name: "no rows", err: sql.ErrNoRows, wantCode: apierror.CodeNotFound},
		{
			name:     "wrapped no rows",
			err:      fmt.Errorf("read zone: %w", sql.ErrNoRows),
			wantCode: apierror.CodeNotFound,
		},
		{
			name:     "deadline",
			err:      context.DeadlineExceeded,
			wantCode: apierror.CodeServerError,
			wantMsg:  "Database request timed out",
		},
		{
			name:     "canceled",
			err:      context.Canceled,
			wantCode: apierror.CodeServerError,
			wantMsg:  "Database request was canceled",
		},
		{
			name:     "driver failure",
			err:      errors.New("database or disk is full"),
			wantCode: apierror.CodeServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HandleQueryError(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("HandleQueryError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("HandleQueryError returned nil, want an error")
			}
			if got.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tt.wantCode)
			}
			if tt.wantMsg != "" && got.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMsg)
			}
			if !errors.Is(got, tt.err) {
				t.Errorf("returned error does not wrap the cause %v", tt.err)
			}
		})
	}
}

// TestHandleQueryErrorHidesDriverDetail is the privacy half of the mapping:
// the driver's text can quote the failing statement, and a statement can quote
// a bound value, so it must live in the internal cause and never in the
// user-facing message.
func TestHandleQueryErrorHidesDriverDetail(t *testing.T) {
	driverErr := errors.New(`UNIQUE constraint failed: users.email (alice@example.com)`)
	got := HandleQueryError(driverErr)
	if got == nil {
		t.Fatal("HandleQueryError returned nil")
	}
	if strings.Contains(got.Message, "alice@example.com") {
		t.Errorf("user-facing message leaks bound value: %s", got.Message)
	}
	if !errors.Is(got, driverErr) {
		t.Error("driver error not retained as the internal cause")
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "no rows", err: sql.ErrNoRows, want: true},
		{name: "wrapped", err: fmt.Errorf("x: %w", sql.ErrNoRows), want: true},
		{name: "other", err: errors.New("boom"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "utc",
			in:   time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC),
			want: "2026-08-25 14:05:09",
		},
		{
			name: "offset is normalized to utc",
			in:   time.Date(2026, 8, 25, 14, 5, 9, 0, time.FixedZone("x", 3600)),
			want: "2026-08-25 13:05:09",
		},
		{
			name: "sub-second is truncated",
			in:   time.Date(2026, 8, 25, 14, 5, 9, 999999999, time.UTC),
			want: "2026-08-25 14:05:09",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatTime(tt.in); got != tt.want {
				t.Errorf("FormatTime = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatTimeSortsLexically is the property the whole layout choice exists
// for: string comparison in SQL must agree with chronological order.
func TestFormatTimeSortsLexically(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prev := FormatTime(base)
	for _, step := range []time.Duration{time.Second, time.Minute, time.Hour, 24 * time.Hour, 40 * 24 * time.Hour} {
		base = base.Add(step)
		next := FormatTime(base)
		if !(prev < next) {
			t.Errorf("%q is not lexically before %q", prev, next)
		}
		prev = next
	}
}

func TestNullTime(t *testing.T) {
	if got := NullTime(time.Time{}); got.Valid {
		t.Errorf("NullTime(zero) = %+v, want invalid", got)
	}
	ts := time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC)
	got := NullTime(ts)
	if !got.Valid || got.String != "2026-08-25 14:05:09" {
		t.Errorf("NullTime = %+v, want 2026-08-25 14:05:09", got)
	}
}

// TestQueryHelpersRoundTrip exercises the three helpers against a real
// database, including the parameter binding that every query in the package
// relies on.
func TestQueryHelpersRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := ExecContext(ctx, db, TimeoutWrite,
		`INSERT INTO items (id, name) VALUES (?, ?)`, 1, "alpha"); err != nil {
		t.Fatalf("ExecContext: %v", err)
	}
	if _, err := ExecContext(ctx, db, TimeoutWrite,
		`INSERT INTO items (id, name) VALUES (?, ?)`, 2, "beta"); err != nil {
		t.Fatalf("ExecContext: %v", err)
	}

	var name string
	if err := QueryRowContext(ctx, db, TimeoutSimple, func(row *sql.Row) error {
		return row.Scan(&name)
	}, `SELECT name FROM items WHERE id = ?`, 2); err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if name != "beta" {
		t.Errorf("name = %q, want beta", name)
	}

	rows, cancel, err := QueryContext(ctx, db, TimeoutSimple, `SELECT name FROM items ORDER BY id`)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	defer cancel()
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("names = %v, want [alpha beta]", names)
	}
}

// TestQueryRowContextNoRows confirms a miss surfaces as sql.ErrNoRows so
// HandleQueryError can turn it into NOT_FOUND.
func TestQueryRowContextNoRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	var id int
	err := QueryRowContext(ctx, db, TimeoutSimple, func(row *sql.Row) error {
		return row.Scan(&id)
	}, `SELECT id FROM items WHERE id = ?`, 42)
	if !IsNotFound(err) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
	if got := HandleQueryError(err); got == nil || got.Code != apierror.CodeNotFound {
		t.Errorf("HandleQueryError = %v, want NOT_FOUND", got)
	}
}

// TestQueryContextParameterBinding is the injection guard: a value that would
// terminate the statement if it were concatenated must be treated as data.
func TestQueryContextParameterBinding(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	hostile := `'; DROP TABLE items; --`
	if _, err := ExecContext(ctx, db, TimeoutWrite,
		`INSERT INTO items (id, name) VALUES (?, ?)`, 1, hostile); err != nil {
		t.Fatalf("ExecContext: %v", err)
	}

	var name string
	if err := QueryRowContext(ctx, db, TimeoutSimple, func(row *sql.Row) error {
		return row.Scan(&name)
	}, `SELECT name FROM items WHERE name = ?`, hostile); err != nil {
		t.Fatalf("QueryRowContext: %v", err)
	}
	if name != hostile {
		t.Errorf("name = %q, want the value stored verbatim", name)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// TestQueryContextCancelledContext checks that an already-cancelled caller
// context aborts immediately rather than running the query.
func TestQueryContextCancelledContext(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, done, err := QueryContext(ctx, db, TimeoutSimple, `SELECT id FROM items`)
	done()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if got := HandleQueryError(err); got == nil || got.Code != apierror.CodeServerError {
		t.Errorf("HandleQueryError = %v, want SERVER_ERROR", got)
	}
}

func TestWithTransactionCommits(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := WithTransaction(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO items (id) VALUES (1)`)
		return err
	})
	if err != nil {
		t.Fatalf("WithTransaction: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestWithTransactionRollsBack(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	sentinel := errors.New("callback failed")
	err := WithTransaction(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO items (id) VALUES (1)`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the callback error", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 after rollback", count)
	}
}

// TestWithTransactionPanic is the lock-safety case: a panicking callback must
// still release the SQLite write lock, or the process deadlocks on its next
// write.
func TestWithTransactionPanic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("panic did not propagate out of WithTransaction")
			}
		}()
		_ = WithTransaction(ctx, db, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO items (id) VALUES (1)`); err != nil {
				t.Errorf("insert: %v", err)
			}
			panic("callback exploded")
		})
	}()

	// A later write proves the lock was released by the rollback.
	if _, err := db.Exec(`INSERT INTO items (id) VALUES (2)`); err != nil {
		t.Fatalf("write after panic: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want only the post-panic row", count)
	}
}

func TestInTransactionMapsError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := InTransaction(ctx, db, func(tx *sql.Tx) error {
		return sql.ErrNoRows
	})
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *apierror.Error", err)
	}
	if apiErr.Code != apierror.CodeNotFound {
		t.Errorf("Code = %q, want %q", apiErr.Code, apierror.CodeNotFound)
	}
}
