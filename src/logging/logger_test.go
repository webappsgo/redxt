package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// testLogs returns a logging configuration equivalent to the PART 11
// defaults, with rotation disabled so tests observe a single file per
// destination.
func testLogs() config.Logs {
	plain := func(name, format string) config.LogFile {
		return config.LogFile{Filename: name, Format: format, Rotate: "never", Keep: "forever"}
	}
	return config.Logs{
		Level:  "info",
		Access: config.AccessLog{LogFile: plain("access.log", FormatApacheName)},
		Server: plain("server.log", FormatTextName),
		Error:  plain("error.log", FormatTextName),
		Audit: config.AuditLog{
			Enabled: true,
			LogFile: plain("audit.log", FormatJSONName),
		},
		Security: plain("security.log", FormatFail2banName),
		Debug:    config.DebugLog{LogFile: plain("debug.log", FormatTextName)},
	}
}

// newTestLogger builds a logger over a temporary directory and returns
// it with that directory. The logger is closed when the test ends.
func newTestLogger(t *testing.T, cfg config.Logs, opts Options) (*Logger, string) {
	t.Helper()
	dir := t.TempDir()
	logger, err := New(cfg, dir, opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = logger.Close()
	})
	return logger, dir
}

// readLog returns the non-empty lines of one log file.
func readLog(t *testing.T, dir, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestIsHealthCheckPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "canonical api path", path: "/api/v1/server/healthz", want: true},
		{name: "base url prefixed api path", path: "/redxt/api/v1/server/readyz", want: true},
		{name: "root alias", path: "/healthz", want: true},
		{name: "root readyz", path: "/readyz", want: true},
		{name: "root livez", path: "/livez", want: true},
		{name: "base url prefixed root alias", path: "/redxt/healthz", want: true},
		{name: "trailing slash", path: "/healthz/", want: true},
		{name: "query string ignored", path: "/healthz?verbose=1", want: true},
		{name: "case insensitive", path: "/HEALTHZ", want: true},
		{name: "unrelated deep path", path: "/api/v1/zones/healthz", want: false},
		{name: "unrelated path", path: "/api/v1/zones", want: false},
		{name: "prefix only", path: "/healthzz", want: false},
		{name: "empty", path: "", want: false},
		{name: "root", path: "/", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsHealthCheckPath(tc.path); got != tc.want {
				t.Errorf("IsHealthCheckPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestAccessHealthCheckSuppression(t *testing.T) {
	tests := []struct {
		name            string
		logHealthChecks bool
		debug           bool
		path            string
		status          int
		wantLogged      bool
	}{
		{name: "successful health check is suppressed", path: "/api/v1/server/healthz", status: 200, wantLogged: false},
		{name: "failed health check is always logged", path: "/api/v1/server/healthz", status: 503, wantLogged: true},
		{name: "health check logged when configured", logHealthChecks: true, path: "/healthz", status: 200, wantLogged: true},
		{name: "health check logged in debug mode", debug: true, path: "/healthz", status: 200, wantLogged: true},
		{name: "ordinary request is logged", path: "/api/v1/zones", status: 200, wantLogged: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testLogs()
			cfg.Access.LogHealthChecks = tc.logHealthChecks
			logger, dir := newTestLogger(t, cfg, Options{Debug: tc.debug})

			logger.Access(Entry{
				Time:     testTime,
				RemoteIP: "127.0.0.1",
				Method:   "GET",
				Path:     tc.path,
				Protocol: "HTTP/1.1",
				Status:   tc.status,
			})

			lines := readLog(t, dir, "access.log")
			if got := len(lines) == 1; got != tc.wantLogged {
				t.Errorf("logged = %v, want %v (lines: %v)", got, tc.wantLogged, lines)
			}
		})
	}
}

func TestLogLevelGatingAndErrorMirroring(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})

	logger.Debugf("dropped below level")
	logger.Infof("server started on :%d", 8080)
	logger.Warnf("cache unavailable")
	logger.Errorf("database connection failed: %s", "timeout")

	server := readLog(t, dir, "server.log")
	if len(server) != 3 {
		t.Fatalf("server.log has %d lines, want 3: %v", len(server), server)
	}
	if !strings.Contains(server[0], "[INFO] server started on :8080") {
		t.Errorf("unexpected first server line: %q", server[0])
	}
	for _, line := range server {
		if strings.Contains(line, "dropped below level") {
			t.Errorf("debug message survived level gating: %q", line)
		}
	}

	errLines := readLog(t, dir, "error.log")
	if len(errLines) != 2 {
		t.Fatalf("error.log has %d lines, want 2 (warn and error mirrored): %v", len(errLines), errLines)
	}
	if !strings.Contains(errLines[0], "[WARN] cache unavailable") {
		t.Errorf("unexpected first error line: %q", errLines[0])
	}
	if !strings.Contains(errLines[1], "[ERROR] database connection failed: timeout") {
		t.Errorf("unexpected second error line: %q", errLines[1])
	}
}

func TestDebugLogRecordsEveryLevel(t *testing.T) {
	cfg := testLogs()
	cfg.Debug.Enabled = true
	logger, dir := newTestLogger(t, cfg, Options{})

	logger.Debugf("verbose detail")
	logger.Infof("ordinary event")

	debug := readLog(t, dir, "debug.log")
	if len(debug) != 2 {
		t.Fatalf("debug.log has %d lines, want 2: %v", len(debug), debug)
	}
}

func TestFileOutputIsPlainASCII(t *testing.T) {
	console := &bytes.Buffer{}
	logger, dir := newTestLogger(t, testLogs(), Options{Console: console, ConsoleColor: true})

	logger.Errorf("\x1b[31mboom\x1b[0m\n2024-10-10T13:55:36-07:00 [INFO] forged")

	for _, name := range []string{"server.log", "error.log"} {
		lines := readLog(t, dir, name)
		if len(lines) != 1 {
			t.Fatalf("%s has %d lines, want 1 (a control character forged a line): %v", name, len(lines), lines)
		}
		for _, r := range lines[0] {
			if r == 0x1b || r < 0x20 || r > 0x7e {
				t.Fatalf("%s contains a non-plain-ASCII byte %q in %q", name, r, lines[0])
			}
		}
	}

	if !strings.Contains(console.String(), "\x1b[") {
		t.Errorf("console output lost its color decoration: %q", console.String())
	}
}

func TestConsoleWithoutColorUsesPlainTags(t *testing.T) {
	console := &bytes.Buffer{}
	logger, _ := newTestLogger(t, testLogs(), Options{Console: console})

	logger.Infof("server started")
	logger.Errorf("it broke")

	out := console.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("console emitted ANSI color with ConsoleColor disabled: %q", out)
	}
	if !strings.Contains(out, "[INFO] server started") || !strings.Contains(out, "[ERROR] it broke") {
		t.Errorf("console output = %q, want plain level tags", out)
	}
}

func TestSecurityLogFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string
	}{
		{name: "fail2ban", format: FormatFail2banName, want: "[security] security.ip_blocked ip=10.0.0.1"},
		{name: "text", format: FormatTextName, want: "[WARN] security.ip_blocked ip=10.0.0.1"},
		{name: "json", format: FormatJSONName, want: `"msg":"security.ip_blocked"`},
		{name: "syslog", format: FormatSyslogName, want: "<36>1 "},
		{name: "cef", format: FormatCEFName, want: "CEF:0|webappsgo|redxt|"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testLogs()
			cfg.Security.Format = tc.format
			logger, dir := newTestLogger(t, cfg, Options{Version: "1.2.3", Hostname: "dns1"})

			logger.Security("security.ip_blocked", slog.String("ip", "10.0.0.1"), slog.String("token", "abcdef"))

			lines := readLog(t, dir, "security.log")
			if len(lines) != 1 {
				t.Fatalf("security.log has %d lines, want 1", len(lines))
			}
			if !strings.Contains(lines[0], tc.want) {
				t.Errorf("security line = %q, want it to contain %q", lines[0], tc.want)
			}
			if strings.Contains(lines[0], "abcdef") {
				t.Errorf("security line leaked a token: %q", lines[0])
			}
		})
	}
}

func TestSlogHandlerRoutesThroughLogger(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})

	log := logger.Slog().With(slog.String("component", "dns")).WithGroup("request")
	log.Info("query answered", slog.String("qname", "example.com"), slog.String("session", "abc123"))

	lines := readLog(t, dir, "server.log")
	if len(lines) != 1 {
		t.Fatalf("server.log has %d lines, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "[INFO] query answered") {
		t.Errorf("line = %q, want the slog message", lines[0])
	}
	if !strings.Contains(lines[0], "component=dns") {
		t.Errorf("line = %q, want the handler attribute", lines[0])
	}
	if !strings.Contains(lines[0], "request.qname=example.com") {
		t.Errorf("line = %q, want the grouped attribute", lines[0])
	}
	if strings.Contains(lines[0], "abc123") {
		t.Errorf("line leaked a session value: %q", lines[0])
	}
}

func TestAccessJSONAndCustomFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		custom string
		want   string
	}{
		{name: "json", format: FormatJSONName, want: `{"ip":"127.0.0.1",`},
		{name: "nginx", format: FormatNginxName, want: `"GET /api/v1/zones HTTP/1.1" 200 12`},
		{name: "custom", format: FormatCustomName, custom: "{remote_ip} {status} {request_id}", want: "127.0.0.1 200 req-9"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testLogs()
			cfg.Access.Format = tc.format
			cfg.Access.Custom = tc.custom
			logger, dir := newTestLogger(t, cfg, Options{})

			logger.Access(Entry{
				Time:      time.Now(),
				RemoteIP:  "127.0.0.1",
				Method:    "GET",
				Path:      "/api/v1/zones",
				Protocol:  "HTTP/1.1",
				Status:    200,
				Bytes:     12,
				RequestID: "req-9",
			})

			lines := readLog(t, dir, "access.log")
			if len(lines) != 1 {
				t.Fatalf("access.log has %d lines, want 1", len(lines))
			}
			if !strings.Contains(lines[0], tc.want) {
				t.Errorf("access line = %q, want it to contain %q", lines[0], tc.want)
			}
		})
	}
}

func TestLoggerCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	logger, err := New(testLogs(), dir, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want slog.Level
	}{
		{name: "debug", in: "debug", want: slog.LevelDebug},
		{name: "info", in: "INFO", want: slog.LevelInfo},
		{name: "warn", in: "warn", want: slog.LevelWarn},
		{name: "error", in: " error ", want: slog.LevelError},
		{name: "unknown falls back to warn", in: "loud", want: slog.LevelWarn},
		{name: "empty falls back to warn", in: "", want: slog.LevelWarn},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLevel(tc.in); got != tc.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
