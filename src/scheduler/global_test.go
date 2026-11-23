package scheduler

import "testing"

// The classification must match AI.md PART 19's "Global Tasks" and
// "Local Tasks" lists exactly: a global task mistakenly treated as
// local would run on every cluster node at once, which is precisely
// what the cluster_locks acquisition exists to prevent.
func TestIsGlobalTaskMatchesSpec(t *testing.T) {
	global := []string{
		"ssl_renewal",
		"geoip_update",
		"blocklist_update",
		"cve_update",
		"backup_daily",
		"update_check",
	}
	for _, name := range global {
		if !IsGlobalTask(name) {
			t.Errorf("IsGlobalTask(%q) = false, want true", name)
		}
	}

	local := []string{
		"session_cleanup",
		"token_cleanup",
		"log_rotation",
		"healthcheck_self",
		"tor_health",
		"cluster_heartbeat",
	}
	for _, name := range local {
		if IsGlobalTask(name) {
			t.Errorf("IsGlobalTask(%q) = true, want false", name)
		}
	}
}

func TestIsGlobalTaskUnknownIsLocal(t *testing.T) {
	if IsGlobalTask("some_custom_admin_task") {
		t.Fatal("an unknown task must default to local")
	}
}
