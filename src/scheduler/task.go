package scheduler

import (
	"context"
	"time"
)

// Handler is the function a task runs. A non-nil error marks the run
// as failed in scheduler_history and, when RetryOnFail is set,
// triggers a single retry after RetryDelay.
type Handler func(ctx context.Context) error

// Task is one entry from AI.md PART 19's built-in task table, or an
// admin-added custom task sharing the same machinery.
type Task struct {
	// Name is the scheduler_tasks primary key, e.g. "ssl_renewal".
	Name string
	// Schedule is a 5-field cron expression or "@every"/"@hourly"/
	// "@daily" shorthand.
	Schedule string
	// Enabled is the built-in default; the persisted scheduler_tasks
	// row (editable from the admin panel) always wins once it exists.
	Enabled bool
	// ClusterAware means only the node that wins the cluster_locks
	// row for this task name runs it on a given occurrence.
	ClusterAware bool
	// RetryOnFail requests one retry, after RetryDelay, when Handler
	// returns an error.
	RetryOnFail bool
	// RetryDelay is how long to wait before the single retry.
	RetryDelay time.Duration
	// Run is the work the task performs.
	Run Handler
}
