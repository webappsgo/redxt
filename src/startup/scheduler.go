package startup

import (
	"context"
	"os"
	"time"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/scheduler"
)

// startScheduler builds and starts the AI.md PART 19 internal task
// scheduler, registering every built-in task this build can already
// perform end to end. A task whose subsystem does not exist yet
// (geoip, backup, update, tor/i2p health) is intentionally left
// unregistered rather than wired to a stub — its schedule still shows
// up in scheduler_tasks (seeded from config.defaultSchedulerTasks) for
// the admin panel to display, but nothing runs against it until its
// owning package lands. See TODO.AI.md for the tracked follow-ups.
func (s *Server) startScheduler(ctx context.Context) error {
	nodeID, err := database.NodeID(ctx, s.ServerDB)
	if err != nil {
		return err
	}

	tz, err := time.LoadLocation(s.Config.Server.Scheduler.Timezone)
	if err != nil {
		tz = time.UTC
	}

	sched := scheduler.New(s.ServerDB, nodeID, tz, s.Config.Server.Scheduler.CatchUpWindow.Duration(), s.Metrics, s.Log)

	tasks := s.Config.Server.Scheduler.Tasks

	if def, ok := tasks["session_cleanup"]; ok {
		if err := sched.Register(scheduler.Task{
			Name: "session_cleanup", Schedule: def.Schedule, Enabled: def.Enabled,
			Run: s.cleanupExpiredSessions,
		}); err != nil {
			return err
		}
	}
	if def, ok := tasks["token_cleanup"]; ok {
		if err := sched.Register(scheduler.Task{
			Name: "token_cleanup", Schedule: def.Schedule, Enabled: def.Enabled,
			Run: s.cleanupExpiredTokens,
		}); err != nil {
			return err
		}
	}
	if def, ok := tasks["log_rotation"]; ok {
		if err := sched.Register(scheduler.Task{
			Name: "log_rotation", Schedule: def.Schedule, Enabled: def.Enabled,
			Run: func(context.Context) error { return s.Log.Reopen() },
		}); err != nil {
			return err
		}
	}
	if def, ok := tasks["cluster_heartbeat"]; ok {
		if err := sched.Register(scheduler.Task{
			Name: "cluster_heartbeat", Schedule: def.Schedule, Enabled: def.Enabled,
			Run: s.clusterHeartbeat,
		}); err != nil {
			return err
		}
	}
	if def, ok := tasks["healthcheck_self"]; ok {
		if err := sched.Register(scheduler.Task{
			Name: "healthcheck_self", Schedule: def.Schedule, Enabled: def.Enabled,
			Run: s.selfHealthCheck,
		}); err != nil {
			return err
		}
	}
	if s.SSL != nil {
		if def, ok := tasks["ssl_renewal"]; ok {
			if err := sched.Register(scheduler.Task{
				Name: "ssl_renewal", Schedule: def.Schedule, Enabled: def.Enabled,
				RetryOnFail: def.RetryOnFail, RetryDelay: def.RetryDelay.Duration(),
				Run: func(ctx context.Context) error {
					_, err := s.SSL.RenewAll(ctx, time.Now())
					return err
				},
			}); err != nil {
				return err
			}
		}
	}

	if err := sched.Start(ctx); err != nil {
		return err
	}
	s.scheduler = sched
	return nil
}

// stopScheduler drains any in-flight task run before the databases it
// depends on close, mirroring the HTTP listener's own drain-before-close
// ordering in Shutdown.
func (s *Server) stopScheduler() error {
	if s.scheduler == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := s.scheduler.Stop(ctx)
	s.scheduler = nil
	return err
}

// cleanupExpiredSessions deletes expired admin and end-user sessions,
// per PART 19's "session_cleanup" task.
func (s *Server) cleanupExpiredSessions(ctx context.Context) error {
	now := time.Now()
	if _, err := database.ExecContext(ctx, s.UsersDB, database.TimeoutWrite,
		`DELETE FROM admin_sessions WHERE expires_at < ?`, now); err != nil {
		return err
	}
	_, err := database.ExecContext(ctx, s.UsersDB, database.TimeoutWrite,
		`DELETE FROM user_sessions WHERE expires_at < ?`, now)
	return err
}

// cleanupExpiredTokens deletes expired admin/agent tokens and expired
// end-user API tokens, per PART 19's "token_cleanup" task.
func (s *Server) cleanupExpiredTokens(ctx context.Context) error {
	now := time.Now()
	if _, err := database.ExecContext(ctx, s.ServerDB, database.TimeoutWrite,
		`DELETE FROM tokens WHERE expires_at IS NOT NULL AND expires_at < ?`, now); err != nil {
		return err
	}
	_, err := database.ExecContext(ctx, s.UsersDB, database.TimeoutWrite,
		`DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at < ?`, now)
	return err
}

// clusterHeartbeat writes this node's cluster_nodes row, per PART 19's
// "cluster_heartbeat" task and PART 10's heartbeat model.
func (s *Server) clusterHeartbeat(ctx context.Context) error {
	nodeID, err := database.NodeID(ctx, s.ServerDB)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	return database.Heartbeat(ctx, s.ServerDB, database.Node{
		ID:         nodeID,
		Hostname:   host,
		AppVersion: version.Version(),
		CommitHash: version.Commit(),
	})
}

// selfHealthCheck confirms both database pools are reachable, per
// PART 19's "healthcheck_self" task. A failure here is recorded to
// scheduler_history the same as any other task failure, which is what
// makes it visible to an operator watching the scheduler admin panel.
func (s *Server) selfHealthCheck(ctx context.Context) error {
	if err := s.ServerDB.DB.PingContext(ctx); err != nil {
		return err
	}
	return s.UsersDB.DB.PingContext(ctx)
}
