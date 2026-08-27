package scheduler

import (
	"context"
	"database/sql"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// taskRow is one persisted scheduler_tasks row.
type taskRow struct {
	schedule  string
	enabled   bool
	lastRunAt sql.NullTime
	nextRunAt sql.NullTime
}

// seedTask inserts a task definition if it does not already exist,
// leaving an existing row (and any admin-panel edits to it) untouched.
func seedTask(ctx context.Context, db *database.DB, name, schedule string, enabled bool, now time.Time) error {
	_, err := database.ExecContext(ctx, db, database.TimeoutWrite, `
		INSERT INTO scheduler_tasks (name, schedule, enabled, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO NOTHING
	`, name, schedule, boolToInt(enabled), now)
	return err
}

// loadTask reads one scheduler_tasks row.
func loadTask(ctx context.Context, db *database.DB, name string) (taskRow, error) {
	var row taskRow
	var enabled int
	err := database.QueryRowContext(ctx, db, database.TimeoutSimple, func(r *sql.Row) error {
		return r.Scan(&row.schedule, &enabled, &row.lastRunAt, &row.nextRunAt)
	}, `SELECT schedule, enabled, last_run_at, next_run_at FROM scheduler_tasks WHERE name = ?`, name)
	if err != nil {
		return taskRow{}, err
	}
	row.enabled = enabled != 0
	return row, nil
}

// setNextRun persists the next scheduled run time for a task.
func setNextRun(ctx context.Context, db *database.DB, name string, next, now time.Time) error {
	_, err := database.ExecContext(ctx, db, database.TimeoutWrite,
		`UPDATE scheduler_tasks SET next_run_at = ?, updated_at = ? WHERE name = ?`, next, now, name)
	return err
}

// recordRun writes one execution to scheduler_history and updates the
// task's last_run_at.
func recordRun(ctx context.Context, db *database.DB, taskName, nodeID, status, message string, duration time.Duration, started, finished time.Time) error {
	_, err := database.ExecContext(ctx, db, database.TimeoutWrite, `
		INSERT INTO scheduler_history (task_name, node_id, status, message, duration_ms, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, taskName, nodeID, status, message, duration.Milliseconds(), started, finished)
	if err != nil {
		return err
	}
	_, err = database.ExecContext(ctx, db, database.TimeoutWrite,
		`UPDATE scheduler_tasks SET last_run_at = ? WHERE name = ?`, finished, taskName)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
