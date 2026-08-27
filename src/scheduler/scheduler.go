package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/database"
)

// pollInterval is how often the run loop checks for due tasks. It is
// well below the tightest built-in schedule (cluster_heartbeat's
// "@every 30s"), so no occurrence is ever missed by more than a
// fraction of a second.
const pollInterval = 5 * time.Second

// MetricsRecorder is the subset of *metrics.Registry the scheduler
// needs. Taking an interface here, rather than importing src/metrics
// directly, keeps the two packages free to evolve independently and
// avoids a dependency edge that isn't otherwise needed.
type MetricsRecorder interface {
	RecordTaskRun(task, status string, durationSeconds float64)
}

// Logger is the subset of *logging.Logger the scheduler needs.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// entry is the scheduler's in-memory bookkeeping for one registered
// task, layered on top of the persisted scheduler_tasks row.
type entry struct {
	task    Task
	sched   *schedule
	next    time.Time
	running bool
}

// Scheduler is the always-running internal task runner AI.md PART 19
// requires in place of any external cron mechanism. It is
// database-backed (scheduler_tasks/scheduler_history), cluster-aware
// (cluster_locks), and drives its own catch-up window on startup.
type Scheduler struct {
	db            *database.DB
	nodeID        string
	timezone      *time.Location
	catchUpWindow time.Duration
	metrics       MetricsRecorder
	log           Logger

	mu      sync.Mutex
	entries map[string]*entry

	stop  chan struct{}
	wg    sync.WaitGroup
	nowFn func() time.Time
}

// New builds a Scheduler bound to db, identifying itself as nodeID in
// scheduler_history and cluster_locks rows. timezone drives cron
// field evaluation (AI.md PART 19's "Timezone" setting); catchUpWindow
// is how far in the past a missed occurrence is still worth running
// on startup instead of being skipped to the next scheduled time.
func New(db *database.DB, nodeID string, timezone *time.Location, catchUpWindow time.Duration, metrics MetricsRecorder, log Logger) *Scheduler {
	if timezone == nil {
		timezone = time.UTC
	}
	return &Scheduler{
		db:            db,
		nodeID:        nodeID,
		timezone:      timezone,
		catchUpWindow: catchUpWindow,
		metrics:       metrics,
		log:           log,
		entries:       map[string]*entry{},
		nowFn:         time.Now,
	}
}

// Register adds a task definition. It must be called before Start.
// An invalid schedule expression is a programming error in a built-in
// task table, so Register returns the parse error rather than
// silently dropping the task.
func (s *Scheduler) Register(t Task) error {
	sched, err := ParseSchedule(t.Schedule)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[t.Name] = &entry{task: t, sched: sched}
	return nil
}

// Start seeds every registered task's scheduler_tasks row, computes
// (or resumes, honoring the catch-up window) its next run time, and
// begins the poll loop. It returns once initial seeding is complete;
// the loop itself runs in a background goroutine until Stop is
// called.
func (s *Scheduler) Start(ctx context.Context) error {
	now := s.nowFn()

	s.mu.Lock()
	names := make([]string, 0, len(s.entries))
	for name := range s.entries {
		names = append(names, name)
	}
	s.mu.Unlock()

	for _, name := range names {
		s.mu.Lock()
		e := s.entries[name]
		s.mu.Unlock()

		if err := seedTask(ctx, s.db, e.task.Name, e.task.Schedule, e.task.Enabled, now); err != nil {
			return err
		}
		row, err := loadTask(ctx, s.db, e.task.Name)
		if err != nil {
			return err
		}

		next, err := s.resumeNext(e, row, now)
		if err != nil {
			return err
		}

		s.mu.Lock()
		e.next = next
		e.task.Enabled = row.enabled
		s.mu.Unlock()

		if err := setNextRun(ctx, s.db, e.task.Name, next, now); err != nil {
			return err
		}
	}

	s.stop = make(chan struct{})
	s.wg.Add(1)
	go s.loop()
	return nil
}

// resumeNext decides the next run time for a task being (re)started:
// a persisted next_run_at inside the catch-up window is honored as-is
// so a missed occurrence still runs; one further in the past is
// skipped forward to the next occurrence after now so a long-stopped
// server does not fire a storm of overdue tasks at once.
func (s *Scheduler) resumeNext(e *entry, row taskRow, now time.Time) (time.Time, error) {
	if row.nextRunAt.Valid {
		persisted := row.nextRunAt.Time
		if !persisted.Before(now.Add(-s.catchUpWindow)) {
			return persisted, nil
		}
	}
	return e.sched.Next(now, s.timezone)
}

// Stop signals the poll loop to exit and waits for any in-flight task
// runs to finish, up to the caller's context deadline.
func (s *Scheduler) Stop(ctx context.Context) error {
	if s.stop == nil {
		return nil
	}
	close(s.stop)
	waited := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	now := s.nowFn()

	s.mu.Lock()
	var due []*entry
	for _, e := range s.entries {
		if e.task.Enabled && !e.running && !now.Before(e.next) {
			e.running = true
			due = append(due, e)
		}
	}
	s.mu.Unlock()

	for _, e := range due {
		s.wg.Add(1)
		go func(e *entry) {
			defer s.wg.Done()
			s.runEntry(e)
		}(e)
	}
}

func (s *Scheduler) runEntry(e *entry) {
	ctx := context.Background()
	now := s.nowFn()

	defer func() {
		s.mu.Lock()
		e.running = false
		next, err := e.sched.Next(now, s.timezone)
		if err == nil {
			e.next = next
			_ = setNextRun(ctx, s.db, e.task.Name, next, now)
		}
		s.mu.Unlock()
	}()

	if e.task.ClusterAware {
		got, err := acquireLock(ctx, s.db, "scheduler:"+e.task.Name, s.nodeID, now)
		if err != nil {
			s.log.Errorf("scheduler: acquire lock for %s: %v", e.task.Name, err)
			return
		}
		if !got {
			return
		}
		defer func() { _ = releaseLock(ctx, s.db, "scheduler:"+e.task.Name, s.nodeID) }()
	}

	s.execute(ctx, e.task)
}

// execute runs one task occurrence, retrying once after RetryDelay
// when it fails and RetryOnFail is set, and records the outcome to
// scheduler_history and the metrics registry either way.
func (s *Scheduler) execute(ctx context.Context, t Task) {
	started := s.nowFn()
	err := t.Run(ctx)

	if err != nil && t.RetryOnFail {
		s.log.Warnf("scheduler: task %s failed, retrying in %s: %v", t.Name, t.RetryDelay, err)
		select {
		case <-time.After(t.RetryDelay):
		case <-s.stop:
			return
		}
		err = t.Run(ctx)
	}

	finished := s.nowFn()
	status := "ok"
	message := ""
	if err != nil {
		status = "failed"
		message = err.Error()
		s.log.Errorf("scheduler: task %s failed: %v", t.Name, err)
	}

	if recErr := recordRun(ctx, s.db, t.Name, s.nodeID, status, message, finished.Sub(started), started, finished); recErr != nil {
		s.log.Errorf("scheduler: recording run of %s: %v", t.Name, recErr)
	}
	if s.metrics != nil {
		s.metrics.RecordTaskRun(t.Name, status, finished.Sub(started).Seconds())
	}
}
