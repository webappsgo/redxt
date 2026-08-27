package startup

import (
	"database/sql"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/metrics"
	"github.com/webappsgo/redxt/src/paths"
)

// startMetrics builds the AI.md PART 21 registry every HTTP request,
// scheduled task run, and database pool reports to for the rest of
// the startup sequence. It is called before startHTTP and
// startScheduler so both have somewhere to report to from the moment
// they start.
func (s *Server) startMetrics() {
	s.Metrics = metrics.New(paths.ProjectName())
	s.Metrics.RegisterApp(metrics.AppInfo{
		Version:   version.Version(),
		Commit:    version.Commit(),
		BuildDate: version.BuildDate(),
		GoVersion: version.GoVersion(),
	}, s.Started)

	s.Metrics.RegisterDBStatsSource("server", func() sql.DBStats {
		return s.ServerDB.DB.Stats()
	})
	s.Metrics.RegisterDBStatsSource("users", func() sql.DBStats {
		return s.UsersDB.DB.Stats()
	})

	loki := s.Config.Server.Metrics.Loki
	s.MetricsLoki = metrics.NewLokiBuffer(loki.MaxEntries, loki.MaxAge.Duration())
}
