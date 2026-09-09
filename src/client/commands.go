package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/webappsgo/redxt/src/health"
)

// Exit codes from the AI.md PART 8 "CLI Exit Codes" table. Commands use
// these named constants instead of bare numbers so the mapping to the
// documented table stays obvious at every call site.
const (
	// ExitAuthError is returned when a request fails because the
	// caller's credentials are invalid, expired, or revoked.
	ExitAuthError = 4
)

// RunHealth calls GET {server}/server/healthz (an unauthenticated route,
// per AI.md PART 13) and prints a short human-readable summary. It
// returns 0 when the server reports healthy, 1 otherwise, matching the
// --status exit-code convention used across every redxt binary.
func RunHealth(client *HTTPClient, out, errOut io.Writer) int {
	var resp health.Response
	httpResp, err := client.Get("/server/healthz", &resp)
	if err != nil {
		if errors.Is(err, ErrTokenRevoked) {
			// AI.md PART 33 "CLI Token Revocation Handling",
			// non-interactive scenario: print the documented
			// message and exit with the authentication-error
			// code so shell pipelines see the failure. The
			// cached token was already cleared by HTTPClient.
			fmt.Fprintf(errOut, "error: your API token has been revoked. Run '%s login' to re-authenticate.\n", BinaryName())
			return ExitAuthError
		}
		if errors.Is(err, ErrTokenExpired) {
			// Same handling as TOKEN_REVOKED per AI.md's "Same
			// behavior on 401 TOKEN_EXPIRED" instruction.
			fmt.Fprintf(errOut, "error: your API token has expired. Run '%s login' to re-authenticate.\n", BinaryName())
			return ExitAuthError
		}
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
