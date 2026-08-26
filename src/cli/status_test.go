package cli

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/health"
)

func TestStatusURL(t *testing.T) {
	tests := []struct {
		name string
		opts StatusOptions
		want string
	}{
		{
			name: "wildcard bind is probed over loopback",
			opts: StatusOptions{Address: "0.0.0.0", Port: 8080},
			want: "http://127.0.0.1:8080/server/healthz",
		},
		{
			name: "empty address",
			opts: StatusOptions{Port: 80},
			want: "http://127.0.0.1:80/server/healthz",
		},
		{
			name: "ipv6 wildcard",
			opts: StatusOptions{Address: "::", Port: 8080},
			want: "http://127.0.0.1:8080/server/healthz",
		},
		{
			name: "explicit host",
			opts: StatusOptions{Address: "10.0.0.5", Port: 9000},
			want: "http://10.0.0.5:9000/server/healthz",
		},
		{
			name: "ipv6 literal is bracketed",
			opts: StatusOptions{Address: "fd00::1", Port: 9000},
			want: "http://[fd00::1]:9000/server/healthz",
		},
		{
			name: "root baseurl adds no prefix",
			opts: StatusOptions{Address: "127.0.0.1", Port: 80, BaseURL: "/"},
			want: "http://127.0.0.1:80/server/healthz",
		},
		{
			name: "baseurl prefix",
			opts: StatusOptions{Address: "127.0.0.1", Port: 80, BaseURL: "/app"},
			want: "http://127.0.0.1:80/app/server/healthz",
		},
		{
			name: "baseurl trailing slash",
			opts: StatusOptions{Address: "127.0.0.1", Port: 80, BaseURL: "/app/"},
			want: "http://127.0.0.1:80/app/server/healthz",
		},
		{
			name: "baseurl without a leading slash",
			opts: StatusOptions{Address: "127.0.0.1", Port: 80, BaseURL: "app"},
			want: "http://127.0.0.1:80/app/server/healthz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusURL(tt.opts); got != tt.want {
				t.Fatalf("statusURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusExitCode(t *testing.T) {
	tests := []struct {
		status string
		want   int
	}{
		{status: health.StatusHealthy, want: 0},
		{status: health.StatusDegraded, want: 0},
		{status: health.StatusRestartRequired, want: 0},
		{status: health.StatusUnhealthy, want: 1},
		{status: health.StatusMaintenance, want: 1},
		{status: health.StatusShuttingDown, want: 1},
		{status: "", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := StatusExitCode(tt.status); got != tt.want {
				t.Fatalf("StatusExitCode(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

// serveStatus starts a health endpoint and returns the options that
// point the client at it.
func serveStatus(t *testing.T, handler http.HandlerFunc) StatusOptions {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	host, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("splitting test server address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parsing test server port: %v", err)
	}
	return StatusOptions{Address: host, Port: port}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name: "healthy server exits zero",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != statusPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(health.Response{Status: health.StatusHealthy})
			},
			wantCode: 0,
			wantOut:  "status: healthy",
		},
		{
			name: "unhealthy server exits one but still prints",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(health.Response{Status: health.StatusUnhealthy})
			},
			wantCode: 1,
			wantOut:  "status: unhealthy",
		},
		{
			name: "unreadable payload",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("<html>not json</html>"))
			},
			wantCode: 1,
			wantErr:  "unreadable health response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := serveStatus(t, tt.handler)
			var out, errOut strings.Builder
			code := Status(context.Background(), opts, &out, &errOut)
			if code != tt.wantCode {
				t.Fatalf("Status() = %d, want %d (stderr %q)", code, tt.wantCode, errOut.String())
			}
			if tt.wantOut != "" && !strings.Contains(out.String(), tt.wantOut) {
				t.Fatalf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(errOut.String(), tt.wantErr) {
				t.Fatalf("stderr = %q, want it to contain %q", errOut.String(), tt.wantErr)
			}
		})
	}
}

// TestStatusServerNotRunning is the healthcheck path that matters most:
// nothing listening must exit 1 rather than hang or panic.
func TestStatusServerNotRunning(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("closing the reserved listener: %v", err)
	}

	var out, errOut strings.Builder
	code := Status(context.Background(), StatusOptions{Address: "127.0.0.1", Port: port}, &out, &errOut)
	if code != 1 {
		t.Fatalf("Status() = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "not responding") {
		t.Fatalf("stderr = %q, want it to report an unresponsive server", errOut.String())
	}
}
