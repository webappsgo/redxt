package metrics

import (
	"database/sql"
	"testing"
	"time"
)

func TestSnapshotDBReadsRegisteredSources(t *testing.T) {
	r := New("redxt")
	r.RegisterDBStatsSource("server", func() sql.DBStats {
		return sql.DBStats{OpenConnections: 4, InUse: 1}
	})

	r.snapshotDB()

	r.mu.Lock()
	f := r.gauges["db_connections_open"]
	r.mu.Unlock()
	got := f.series[seriesKey(map[string]string{"database": "server"})].get()
	if got != 4 {
		t.Fatalf("db_connections_open = %v, want 4", got)
	}
}

func TestSnapshotUptimeNoopBeforeRegisterApp(t *testing.T) {
	r := New("redxt")
	r.snapshotUptime(time.Now())
	r.mu.Lock()
	_, ok := r.gauges["app_uptime_seconds"]
	r.mu.Unlock()
	if ok {
		t.Fatalf("app_uptime_seconds should not exist before RegisterApp")
	}
}
