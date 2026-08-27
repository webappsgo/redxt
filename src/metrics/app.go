package metrics

import (
	"database/sql"
	"time"
)

// AppInfo holds the build identity AI.md PART 21's required
// application-info metrics carry as labels.
type AppInfo struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

// RegisterApp records the {project_name}_app_info,
// {project_name}_app_uptime_seconds, and
// {project_name}_app_start_timestamp metrics. start is the process
// start time; uptime is computed fresh on every scrape rather than
// stored, so it never drifts from wall-clock time.
func (r *Registry) RegisterApp(info AppInfo, start time.Time) {
	r.Gauge("app_info", "Always 1; labels carry build information.", map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"build_date": info.BuildDate,
		"go_version": info.GoVersion,
	}, 1)
	r.Gauge("app_start_timestamp", "Unix timestamp of process start.", nil, float64(start.Unix()))
	r.mu.Lock()
	r.uptimeStart = start
	r.mu.Unlock()
}

// snapshotUptime refreshes app_uptime_seconds against the wall clock.
// It is called just before every render so the value is always
// current without a background goroutine.
func (r *Registry) snapshotUptime(now time.Time) {
	r.mu.Lock()
	start := r.uptimeStart
	r.mu.Unlock()
	if start.IsZero() {
		return
	}
	r.Gauge("app_uptime_seconds", "Seconds since process start.", nil, now.Sub(start).Seconds())
}

// DBStatsSource reports the database/sql pool stats for one named
// database (e.g. "server", "users"). Registry.snapshotDB reads these
// just before every render, so the *_db_connections_* gauges are
// always current without a background goroutine polling the pool.
type DBStatsSource func() sql.DBStats

// RegisterDBStatsSource attaches a database's pool to the registry
// under name (used as the "database" label).
func (r *Registry) RegisterDBStatsSource(name string, source DBStatsSource) {
	r.mu.Lock()
	if r.dbSources == nil {
		r.dbSources = map[string]DBStatsSource{}
	}
	r.dbSources[name] = source
	r.mu.Unlock()
}

func (r *Registry) snapshotDB() {
	r.mu.Lock()
	sources := make(map[string]DBStatsSource, len(r.dbSources))
	for k, v := range r.dbSources {
		sources[k] = v
	}
	r.mu.Unlock()

	for name, source := range sources {
		stats := source()
		r.Gauge("db_connections_open", "Open database connections.", map[string]string{"database": name}, float64(stats.OpenConnections))
		r.Gauge("db_connections_in_use", "Database connections currently in use.", map[string]string{"database": name}, float64(stats.InUse))
	}
}
