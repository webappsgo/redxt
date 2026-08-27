package metrics

import "testing"

func TestRecordTaskRunOK(t *testing.T) {
	r := New("redxt")
	r.RecordTaskRun("ssl_renewal", "ok", 1.5)

	r.mu.Lock()
	runs := r.counters["scheduler_task_runs_total"]
	_, hasFailure := r.counters["scheduler_task_failures_total"]
	r.mu.Unlock()

	key := seriesKey(map[string]string{"task": "ssl_renewal", "status": "ok"})
	if got := runs.series[key].get(); got != 1 {
		t.Fatalf("scheduler_task_runs_total = %v, want 1", got)
	}
	if hasFailure {
		t.Fatalf("scheduler_task_failures_total should not exist on an ok run")
	}
}

func TestRecordTaskRunFailure(t *testing.T) {
	r := New("redxt")
	r.RecordTaskRun("ssl_renewal", "failed", 0.5)

	r.mu.Lock()
	failures := r.counters["scheduler_task_failures_total"]
	r.mu.Unlock()

	key := seriesKey(map[string]string{"task": "ssl_renewal"})
	if got := failures.series[key].get(); got != 1 {
		t.Fatalf("scheduler_task_failures_total = %v, want 1", got)
	}
}

func TestRecordAuthAttemptAndActiveSessions(t *testing.T) {
	r := New("redxt")
	r.RecordAuthAttempt("password", "success")
	r.SetActiveSessions(7)

	r.mu.Lock()
	attempts := r.counters["auth_attempts_total"]
	sessions := r.gauges["auth_sessions_active"]
	r.mu.Unlock()

	key := seriesKey(map[string]string{"method": "password", "status": "success"})
	if got := attempts.series[key].get(); got != 1 {
		t.Fatalf("auth_attempts_total = %v, want 1", got)
	}
	if got := sessions.series[seriesKey(nil)].get(); got != 7 {
		t.Fatalf("auth_sessions_active = %v, want 7", got)
	}
}
