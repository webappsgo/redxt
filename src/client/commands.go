package main

import (
	"fmt"
	"io"

	"github.com/webappsgo/redxt/src/health"
)

// RunHealth calls GET {server}/server/healthz (an unauthenticated route,
// per AI.md PART 13) and prints a short human-readable summary. It
// returns 0 when the server reports healthy, 1 otherwise, matching the
// --status exit-code convention used across every redxt binary.
func RunHealth(client *HTTPClient, out, errOut io.Writer) int {
	var resp health.Response
	httpResp, err := client.Get("/server/healthz", &resp)
	if err != nil {
		fmt.Fprintf(errOut, "health check failed: %s\n", err)
		return 1
	}
	if httpResp.StatusCode >= 500 && resp.Status == "" {
		fmt.Fprintf(errOut, "health check failed: server returned HTTP %d\n", httpResp.StatusCode)
		return 1
	}

	fmt.Fprintf(out, "Project:  %s\n", resp.Project.Name)
	fmt.Fprintf(out, "Status:   %s\n", resp.Status)
	fmt.Fprintf(out, "Version:  %s\n", resp.Version)
	fmt.Fprintf(out, "Mode:     %s\n", resp.Mode)
	fmt.Fprintf(out, "Uptime:   %s\n", resp.Uptime)

	switch resp.Status {
	case health.StatusHealthy, health.StatusRestartRequired:
		return 0
	default:
		return 1
	}
}
