package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/health"
)

// statusPath is the canonical health route. AI.md PART 13 keeps
// /server/healthz canonical even when the root alias is enabled, so the
// client never has to guess which aliases a given server mounts.
const statusPath = "/server/healthz"

// statusTimeout bounds the whole probe. The documented Docker
// healthcheck allows five seconds, so the client must give up first.
const statusTimeout = 4 * time.Second

// StatusOptions describes the server the --status client should probe.
type StatusOptions struct {
	// Address is the listen address from the resolved configuration. A
	// wildcard bind is probed over the loopback interface instead.
	Address string
	// Port is the HTTP port the server listens on.
	Port int
	// BaseURL is the URL path prefix the server was configured with.
	BaseURL string
}

// statusURL builds the absolute URL of the health route.
func statusURL(o StatusOptions) string {
	host := o.Address
	switch host {
	case "", "0.0.0.0", "::", "[::]", "*":
		host = "127.0.0.1"
	}

	prefix := o.BaseURL
	if prefix == "/" {
		prefix = ""
	}
	for len(prefix) > 0 && prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}
	if len(prefix) > 0 && prefix[0] != '/' {
		prefix = "/" + prefix
	}

	return "http://" + net.JoinHostPort(host, strconv.Itoa(o.Port)) + prefix + statusPath
}

// Status probes the running server and prints its health. It returns
// the process exit code defined in AI.md PART 8: 0 when the server is
// running and reports a healthy-equivalent status, 1 for every other
// outcome, including an unreachable server.
func Status(ctx context.Context, o StatusOptions, out, errOut io.Writer) int {
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()

	url := statusURL(o)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(errOut, "status: %v\n", err)
		return 1
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: statusTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// The error text carries the dial target only, which the caller
		// already knows; no configuration detail is disclosed.
		fmt.Fprintf(errOut, "status: server not responding on %s\n", url)
		return 1
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// The payload is bounded because a health response is small and the
	// endpoint is reachable before authentication.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		fmt.Fprintf(errOut, "status: reading response: %v\n", err)
		return 1
	}

	var parsed health.Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Fprintf(errOut, "status: server returned an unreadable health response\n")
		return 1
	}

	fmt.Fprint(out, parsed.Text())
	return StatusExitCode(parsed.Status)
}

// StatusExitCode maps a health status to the --status exit code. The
// statuses that still serve traffic exit 0; everything else exits 1.
func StatusExitCode(status string) int {
	switch status {
	case health.StatusHealthy, health.StatusDegraded, health.StatusRestartRequired:
		return 0
	default:
		return 1
	}
}
