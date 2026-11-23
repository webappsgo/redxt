package startup

import (
	"context"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/scheduler"
)

func noopHandler(context.Context) error { return nil }

// The ssl_renewal task must reach the scheduler with PART 19's daily
// 03:00 schedule and as a global (cluster-locked) task, so a cluster
// never has several nodes renewing the same certificate at once.
func TestRegisterTaskWiresSSLRenewal(t *testing.T) {
	sched := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil)
	tasks := config.DefaultConfig().Server.Scheduler.Tasks

	if err := registerTask(sched, tasks, "ssl_renewal", noopHandler); err != nil {
		t.Fatalf("registerTask: %v", err)
	}

	task, ok := sched.Registered("ssl_renewal")
	if !ok {
		t.Fatal("ssl_renewal was not registered")
	}
	if task.Schedule != "0 3 * * *" {
		t.Errorf("schedule = %q, want %q", task.Schedule, "0 3 * * *")
	}
	if !task.Enabled {
		t.Error("ssl_renewal must be enabled by default")
	}
	if !task.ClusterAware {
		t.Error("ssl_renewal is a PART 19 global task and must be cluster-aware")
	}
}

// The daily 03:00 cron expression must actually resolve to 03:00 local
// time, which is the concrete guarantee PART 15's renewal check relies
// on rather than the literal string alone.
func TestSSLRenewalScheduleFiresAtThree(t *testing.T) {
	tasks := config.DefaultConfig().Server.Scheduler.Tasks
	parsed, err := scheduler.ParseSchedule(tasks["ssl_renewal"].Schedule)
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}

	next, err := parsed.Next(time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next ssl_renewal run = %s, want %s", next, want)
	}
}

// A task the operator removed from the config table registers nothing
// rather than falling back to a hardcoded schedule.
func TestRegisterTaskSkipsUnconfiguredTask(t *testing.T) {
	sched := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil)

	if err := registerTask(sched, map[string]config.SchedulerTask{}, "ssl_renewal", noopHandler); err != nil {
		t.Fatalf("registerTask: %v", err)
	}
	if _, ok := sched.Registered("ssl_renewal"); ok {
		t.Fatal("an unconfigured task must not be registered")
	}
}

// Node-local tasks must not take a cluster lock, or only one node in
// the cluster would ever clean up its own sessions.
func TestRegisterTaskLeavesLocalTasksNodeLocal(t *testing.T) {
	sched := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil)
	tasks := config.DefaultConfig().Server.Scheduler.Tasks

	if err := registerTask(sched, tasks, "session_cleanup", noopHandler); err != nil {
		t.Fatalf("registerTask: %v", err)
	}
	task, ok := sched.Registered("session_cleanup")
	if !ok {
		t.Fatal("session_cleanup was not registered")
	}
	if task.ClusterAware {
		t.Error("session_cleanup is a PART 19 local task and must run on every node")
	}
}

func TestRegisterTaskRejectsInvalidSchedule(t *testing.T) {
	sched := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil)
	tasks := map[string]config.SchedulerTask{
		"ssl_renewal": {Schedule: "not a cron expression", Enabled: true},
	}

	if err := registerTask(sched, tasks, "ssl_renewal", noopHandler); err == nil {
		t.Fatal("expected an invalid schedule to be rejected")
	}
}
