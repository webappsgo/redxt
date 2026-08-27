package metrics

// RecordTaskRun records one scheduler task execution, per AI.md
// PART 19/21's scheduler metrics category. status is "ok" or
// "failed".
func (r *Registry) RecordTaskRun(task, status string, durationSeconds float64) {
	r.Counter("scheduler_task_runs_total", "Total scheduler task executions.",
		map[string]string{"task": task, "status": status}, 1)
	r.Histogram("scheduler_task_duration_seconds", "Scheduler task execution latency.",
		[]float64{0.01, 0.1, 0.5, 1, 5, 30, 60, 300, 900}, map[string]string{"task": task}, durationSeconds)
	if status != "ok" {
		r.Counter("scheduler_task_failures_total", "Total scheduler task failures.",
			map[string]string{"task": task}, 1)
	}
}

// RecordAuthAttempt records one authentication attempt, per AI.md
// PART 21's required auth metrics. method is e.g. "password" or
// "token"; status is "success" or "failure".
func (r *Registry) RecordAuthAttempt(method, status string) {
	r.Counter("auth_attempts_total", "Total authentication attempts.",
		map[string]string{"method": method, "status": status}, 1)
}

// SetActiveSessions sets the current active-session gauge.
func (r *Registry) SetActiveSessions(n float64) {
	r.Gauge("auth_sessions_active", "Currently active sessions.", nil, n)
}
