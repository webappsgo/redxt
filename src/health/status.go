package health

import (
	"net/http"
)

// HTTPStatus maps an overall health status onto its HTTP status code,
// per the AI.md PART 13 "Health Status Values & HTTP Codes" table. The
// body renders normally in every state; only the code changes. An
// unrecognized status is treated as unhealthy so that a bug here can
// never make a broken server look available to a load balancer.
func HTTPStatus(status string) int {
	switch status {
	case StatusHealthy, StatusDegraded, StatusRestartRequired:
		return http.StatusOK
	case StatusUnhealthy, StatusMaintenance, StatusShuttingDown:
		return http.StatusServiceUnavailable
	default:
		return http.StatusServiceUnavailable
	}
}

// Check converts a boolean probe result into the CheckOK or CheckError
// word that the public health document is allowed to carry.
func Check(ok bool) string {
	if ok {
		return CheckOK
	}
	return CheckError
}

// State carries the process-level conditions that outrank component
// checks when deciding the overall status.
type State struct {
	// ShuttingDown reports that graceful shutdown has begun.
	ShuttingDown bool
	// Maintenance reports that maintenance mode is active.
	Maintenance bool
	// PendingRestart reports a config change awaiting a restart.
	PendingRestart bool
}

// criticalChecks returns the checks whose failure makes the whole
// instance unhealthy. A DNS server that cannot reach its datastore,
// cannot bind its listeners, or failed to load its zones is not
// serving, so those three are critical; everything else degrades.
func criticalChecks(c ChecksInfo) []string {
	return []string{c.Database, c.DNSListener, c.Zones}
}

// optionalChecks returns the checks whose failure only degrades the
// instance. Empty values are skipped by Overall because an omitted
// check means the component is not enabled on this node.
func optionalChecks(c ChecksInfo) []string {
	return []string{
		c.Cache,
		c.Disk,
		c.Scheduler,
		c.Cluster,
		c.Tor,
		c.I2P,
		c.Forwarders,
		c.Blocklists,
	}
}

// Overall resolves the single status word for a health response from
// the process state and the component checks. Precedence runs from the
// most severe condition down: a shutting-down or maintenance instance
// reports that regardless of its checks, a failed critical check makes
// it unhealthy, a failed optional check degrades it, and a healthy
// instance holding a pending restart reports restart_required.
func Overall(s State, c ChecksInfo) string {
	if s.ShuttingDown {
		return StatusShuttingDown
	}
	if s.Maintenance {
		return StatusMaintenance
	}
	for _, v := range criticalChecks(c) {
		if v == CheckError {
			return StatusUnhealthy
		}
	}
	for _, v := range optionalChecks(c) {
		if v == CheckError {
			return StatusDegraded
		}
	}
	if s.PendingRestart {
		return StatusRestartRequired
	}
	return StatusHealthy
}

// Apply sets Status, PendingRestart, and RestartReason on a response
// from the process state and the checks already filled in. Restart
// reasons are only carried while a restart is actually pending, so a
// stale reason list can never appear on a healthy response.
func (r *Response) Apply(s State, reasons []string) {
	r.Status = Overall(s, r.Checks)
	r.PendingRestart = s.PendingRestart
	if s.PendingRestart && len(reasons) > 0 {
		r.RestartReason = append([]string(nil), reasons...)
	} else {
		r.RestartReason = nil
	}
}
