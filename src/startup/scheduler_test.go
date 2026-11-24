package startup

import (
	"context"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/scheduler"
)

func noopHandler(context.Context) error { return nil }

// Built-in task registration must use PART 19's global/local classification
// so global tasks acquire a cluster lock and local tasks run on every node.
func TestConfiguredTaskUsesGlobalTaskClassification(t *testing.T) {
	defaults := config.DefaultConfig().Server.Scheduler.Tasks
	globalTasks := []string{
		"ssl_renewal", "geoip_update", "blocklist_update",
		"cve_update", "backup_daily", "update_check",
	}
	for _, name := range globalTasks {
		task := configuredTask(name, defaults[name], noopHandler)
		if !task.ClusterAware {
			t.Errorf("configuredTask(%q).ClusterAware = false, want true", name)
		}
		if err := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil).Register(task); err != nil {
			t.Errorf("Register(%q): %v", name, err)
		}
	}

	localTasks := []string{
		"session_cleanup", "token_cleanup", "log_rotation",
		"healthcheck_self", "tor_health", "cluster_heartbeat",
	}
	for _, name := range localTasks {
		task := configuredTask(name, defaults[name], noopHandler)
		if task.ClusterAware {
			t.Errorf("configuredTask(%q).ClusterAware = true, want false", name)
		}
		if err := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil).Register(task); err != nil {
			t.Errorf("Register(%q): %v", name, err)
		}
	}
}

// The daily 03:00 cron expression must resolve to 03:00 in the configured
// timezone, which is the concrete guarantee PART 15's renewal check relies on.
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

func TestConfiguredTaskRejectsInvalidSchedule(t *testing.T) {
	task := configuredTask("ssl_renewal", config.SchedulerTask{
		Schedule: "not a cron expression",
		Enabled:  true,
	}, noopHandler)
	if err := scheduler.New(nil, "node-a", time.UTC, time.Hour, nil, nil).Register(task); err == nil {
		t.Fatal("expected an invalid schedule to be rejected")
	}
}
