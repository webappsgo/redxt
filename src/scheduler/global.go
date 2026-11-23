package scheduler

// globalTasks is AI.md PART 19's "Global Tasks (run once per cluster)"
// list, verbatim (line 34477) — six tasks, no more. Every other built-in
// task is local and runs on every node, matching the fallback below.
//
// PART 19 does not name backup_hourly or i2p_health in either list.
// Rather than guess a cluster scope the spec never states, both are
// left out of this map and fall through to the local default; the
// scheduler backlog item in the project's task file tracks resolving
// this properly instead of assuming an answer.
var globalTasks = map[string]bool{
	"ssl_renewal":      true,
	"geoip_update":     true,
	"blocklist_update": true,
	"cve_update":       true,
	"backup_daily":     true,
	"update_check":     true,
}

// IsGlobalTask reports whether a built-in task runs on one cluster node
// only, which is what Task.ClusterAware turns into a cluster_locks
// acquisition at run time. An unknown name is treated as local, since a
// task with no cluster-wide side effect is safe to run everywhere.
func IsGlobalTask(name string) bool {
	return globalTasks[name]
}
