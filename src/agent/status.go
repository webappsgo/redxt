package main

import (
	"fmt"
	"io"

	"github.com/webappsgo/redxt/src/health"
)

// RunStatus implements the --status flag (AI.md PART 33 Agent Flags:
// "Health check (exit 0=healthy, 1=unhealthy)"). It probes the
// configured server's unauthenticated /server/healthz route, since no
// server-side agent-registration or heartbeat API exists yet to report
// this agent's own registered state (see TODO.AI.md) — this checks
// reachability of the configured server, not the agent's registration
// status.
func RunStatus(client *HTTPClient, out, errOut io.Writer) int {
	var resp health.Response
	httpResp, err := client.Get("/server/healthz", &resp)
	if err != nil {
		fmt.Fprintf(errOut, "status check failed: %s\n", err)
		return 1
	}
	if httpResp.StatusCode >= 500 && resp.Status == "" {
		fmt.Fprintf(errOut, "status check failed: server returned HTTP %d\n", httpResp.StatusCode)
		return 1
	}

	fmt.Fprintf(out, "Server:   %s\n", client.BaseURL)
	fmt.Fprintf(out, "Status:   %s\n", resp.Status)
	fmt.Fprintf(out, "Version:  %s\n", resp.Version)
	fmt.Fprintf(out, "Mode:     %s\n", resp.Mode)

	switch resp.Status {
	case health.StatusHealthy, health.StatusRestartRequired:
		return 0
	default:
		return 1
	}
}
