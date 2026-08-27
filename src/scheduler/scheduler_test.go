package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
)

type noopLogger struct{}

func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}

type recordedRun struct {
	task, status string
	duration     float64
}

type fakeMetrics struct {
	runs []recordedRun
}

func (f *fakeMetrics) RecordTaskRun(task, status string, durationSeconds float64) {
	f.runs = append(f.runs, recordedRun{task, status, durationSeconds})
}

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.OpenServer(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenServer: %v", err)
	}
	if err := database.EnsureServerSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureServerSchema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSchedulerRunsDueTask(t *testing.T) {
	db := openTestDB(t)
	metrics := &fakeMetrics{}

	var calls int32
	sched := New(db, "node-a", time.UTC, time.Hour, metrics, noopLogger{})
	if err := sched.Register(Task{
		Name:     "test_task",
		Schedule: "@every 1h",
		Enabled:  true,
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Force the task due immediately by starting from a fixed clock
	// and then pretending it's already past the computed next run.
	sched.nowFn = func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) }
	if err := sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sched.Stop(ctx)
	}()

	sched.mu.Lock()
	sched.entries["test_task"].next = sched.nowFn()
	sched.mu.Unlock()

	sched.tick()

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("handler called %d times, want 1", got)
	}
}

func TestSchedulerRetriesOnFailure(t *testing.T) {
	db := openTestDB(t)
	metrics := &fakeMetrics{}

	var attempts int32
	sched := New(db, "node-a", time.UTC, time.Hour, metrics, noopLogger{})
	if err := sched.Register(Task{
		Name:        "retry_task",
		Schedule:    "@every 1h",
		Enabled:     true,
		RetryOnFail: true,
		RetryDelay:  10 * time.Millisecond,
		Run: func(ctx context.Context) error {
			n := atomic.AddInt32(&attempts, 1)
			if n == 1 {
				return errors.New("first attempt fails")
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	fixed := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	sched.nowFn = func() time.Time { return fixed }
	if err := sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sched.Stop(ctx)
	}()

	sched.mu.Lock()
	sched.entries["retry_task"].next = fixed
	sched.mu.Unlock()

	sched.tick()

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&attempts) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("handler ran %d times, want 2 (initial + retry)", got)
	}
}

func TestSchedulerClusterAwareSkipsWhenLockHeld(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	got, err := acquireLock(context.Background(), db, "scheduler:cluster_task", "other-node", now)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}
	if !got {
		t.Fatalf("expected other-node to win the lock")
	}

	metrics := &fakeMetrics{}
	var calls int32
	sched := New(db, "node-a", time.UTC, time.Hour, metrics, noopLogger{})
	if err := sched.Register(Task{
		Name:         "cluster_task",
		Schedule:     "@every 1h",
		Enabled:      true,
		ClusterAware: true,
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sched.nowFn = func() time.Time { return now }
	if err := sched.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sched.Stop(ctx)
	}()

	sched.mu.Lock()
	sched.entries["cluster_task"].next = now
	sched.mu.Unlock()

	sched.runEntry(sched.entries["cluster_task"])

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("handler ran %d times, want 0 (lock held by another node)", got)
	}
}

func TestResumeNextHonorsCatchUpWindow(t *testing.T) {
	db := openTestDB(t)
	sched := New(db, "node-a", time.UTC, time.Hour, nil, noopLogger{})
	e := &entry{}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	recent := taskRow{nextRunAt: sqlNullTime(now.Add(-30 * time.Minute))}
	next, err := sched.resumeNext(e, recent, now)
	if err != nil {
		t.Fatalf("resumeNext: %v", err)
	}
	if !next.Equal(now.Add(-30 * time.Minute)) {
		t.Fatalf("expected the missed run to be honored, got %s", next)
	}

	sched2, err := ParseSchedule("@every 1h")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	e.sched = sched2
	stale := taskRow{nextRunAt: sqlNullTime(now.Add(-3 * time.Hour))}
	next2, err := sched.resumeNext(e, stale, now)
	if err != nil {
		t.Fatalf("resumeNext: %v", err)
	}
	if !next2.After(now) {
		t.Fatalf("expected a stale run to be skipped forward, got %s", next2)
	}
}
