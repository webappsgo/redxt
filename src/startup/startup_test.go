package startup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/cli"
	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/pidfile"
	"github.com/webappsgo/redxt/src/security"
)

// testArgs points every directory flag at a fresh temporary tree so a
// test never touches the real system locations.
func testArgs(t *testing.T, extra ...string) []string {
	t.Helper()

	root := t.TempDir()
	args := []string{
		"--config", filepath.Join(root, "config"),
		"--data", filepath.Join(root, "data"),
		"--cache", filepath.Join(root, "cache"),
		"--log", filepath.Join(root, "log"),
		"--backup", filepath.Join(root, "backup"),
		"--pid", filepath.Join(root, "run", "redxt.pid"),
		"--color", "no",
	}
	return append(args, extra...)
}

// startForTest runs steps 6 through 15 and registers the teardown.
func startForTest(t *testing.T, args []string) (*Server, *strings.Builder) {
	t.Helper()

	opts, err := cli.Parse("redxt", args, io.Discard)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	out := &strings.Builder{}
	server, err := Start(context.Background(), opts, IO{Out: out, Err: io.Discard})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if server.Log != nil {
			_ = server.Shutdown()
		}
	})
	return server, out
}

// TestStartCreatesTheWholeTree covers startup steps 7 through 15: every
// directory, the configuration, the log files, and both databases must
// exist once Start returns.
func TestStartCreatesTheWholeTree(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	for name, dir := range map[string]string{
		"config":   server.Paths.Config,
		"data":     server.Paths.Data,
		"cache":    server.Paths.Cache,
		"logs":     server.Paths.Logs,
		"backup":   server.Paths.Backup,
		"database": server.Paths.DB,
		"ssl":      server.Paths.SSL,
		"security": server.Paths.Security,
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("%s directory missing: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", name)
		}
	}

	if _, err := os.Stat(server.Paths.ConfigFile); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(server.Paths.Logs, "server.log")); err != nil {
		t.Fatalf("server log missing: %v", err)
	}
	if server.ServerDB == nil || server.UsersDB == nil {
		t.Fatalf("databases not opened")
	}
}

// TestStartSensitiveDirectoriesAre0700 guards the PART 8 rule that key
// material directories are never group- or world-readable.
func TestStartSensitiveDirectoriesAre0700(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	for _, dir := range []string{server.Paths.SSL, server.Paths.Security} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Fatalf("%s mode = %o, want 700", dir, perm)
		}
	}
}

// TestStartWritesThePIDFile covers steps 11 and 12, and the container
// carve-out: inside a container no PID file is written at all.
func TestStartWritesThePIDFile(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	if pidfile.IsContainer() {
		if _, err := os.Stat(server.Paths.PIDFile); !os.IsNotExist(err) {
			t.Fatalf("a PID file was written inside a container")
		}
		return
	}

	data, err := os.ReadFile(server.Paths.PIDFile)
	if err != nil {
		t.Fatalf("reading the PID file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("PID file does not hold a number: %q", data)
	}
	if pid != os.Getpid() {
		t.Fatalf("PID file holds %d, want %d", pid, os.Getpid())
	}
}

// TestShutdownRemovesThePIDFile covers the teardown half of steps 11
// and 12 and confirms Shutdown is idempotent.
func TestShutdownRemovesThePIDFile(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))
	pidPath := server.Paths.PIDFile

	if err := server.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("PID file survived shutdown: %v", err)
	}
	if err := server.Shutdown(); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

// TestSetupTokenIsStoredHashedOnly is the security guard for the
// first-run token: the console gets the token, the database gets only
// its hash, and a second run issues nothing.
func TestSetupTokenIsStoredHashedOnly(t *testing.T) {
	args := testArgs(t)
	server, _ := startForTest(t, args)

	if !server.FirstRun {
		t.Fatalf("expected the first run to be detected")
	}
	if len(server.SetupToken) != security.SetupTokenLength {
		t.Fatalf("setup token = %q, want %d characters", server.SetupToken, security.SetupTokenLength)
	}

	stored, _, _, err := database.GetSecret(context.Background(), server.ServerDB, setupTokenSecret)
	if err != nil {
		t.Fatalf("reading the stored setup token: %v", err)
	}
	if stored == server.SetupToken {
		t.Fatalf("the setup token was stored in plaintext")
	}
	if stored != security.HashToken(server.SetupToken) {
		t.Fatalf("stored value is not the token hash")
	}

	// The log files must never carry the token.
	for _, name := range []string{"server.log", "error.log", "security.log"} {
		data, err := os.ReadFile(filepath.Join(server.Paths.Logs, name))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), server.SetupToken) {
			t.Fatalf("%s leaked the setup token", name)
		}
	}

	if err := server.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// A second run over the same tree is not a first run.
	second, _ := startForTest(t, args)
	if second.FirstRun {
		t.Fatalf("the second run was treated as a first run")
	}
	if second.SetupToken != "" {
		t.Fatalf("the second run issued a setup token")
	}
}

// TestCLIOverridesBeatTheConfigFile covers the PART 12 precedence rule
// that a command-line flag wins over every other source.
func TestCLIOverridesBeatTheConfigFile(t *testing.T) {
	server, _ := startForTest(t, testArgs(t, "--address", "127.0.0.1", "--port", "8654", "--baseurl", "/dns"))

	if server.Config.Server.Listen != "127.0.0.1" {
		t.Fatalf("Listen = %q, want 127.0.0.1", server.Config.Server.Listen)
	}
	if server.Config.Server.Port != 8654 {
		t.Fatalf("Port = %d, want 8654", server.Config.Server.Port)
	}
	if server.Config.Server.BaseURL != "/dns" {
		t.Fatalf("BaseURL = %q, want /dns", server.Config.Server.BaseURL)
	}
}

// TestDualPortUsesTheHTTPPort confirms the "{http},{https}" form takes
// its first field as the HTTP port.
func TestDualPortUsesTheHTTPPort(t *testing.T) {
	server, _ := startForTest(t, testArgs(t, "--port", "8080,8443"))

	if server.Config.Server.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", server.Config.Server.Port)
	}
}

// TestReopenLogsSurvivesRotation covers the SIGUSR1 handler end to end.
func TestReopenLogsSurvivesRotation(t *testing.T) {
	server, _ := startForTest(t, testArgs(t))

	live := filepath.Join(server.Paths.Logs, "server.log")
	moved := live + ".moved"
	if err := os.Rename(live, moved); err != nil {
		t.Fatalf("rotating the log aside: %v", err)
	}

	server.reopenLogs()
	server.dumpStatus()

	data, err := os.ReadFile(live)
	if err != nil {
		t.Fatalf("reading the reopened log: %v", err)
	}
	if !strings.Contains(string(data), "Status:") {
		t.Fatalf("the reopened log did not receive the status dump: %q", data)
	}
}

// TestHandleImmediateHelp covers the PHASE 1 flags that print and exit
// without starting anything.
func TestHandleImmediate(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")

	tests := []struct {
		name     string
		args     []string
		wantDone bool
		wantCode int
		wantOut  string
	}{
		{name: "help", args: []string{"--help"}, wantDone: true, wantCode: ExitOK, wantOut: "Usage:"},
		{name: "version", args: []string{"--version"}, wantDone: true, wantCode: ExitOK, wantOut: "1.2.3"},
		{
			name:     "shell completions",
			args:     []string{"--shell", "completions", "bash"},
			wantDone: true,
			wantCode: ExitOK,
			wantOut:  "complete -F",
		},
		{name: "no immediate flag", args: nil, wantDone: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := cli.Parse("redxt", tt.args, io.Discard)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			var out strings.Builder
			code, done := handleImmediate(context.Background(), opts, "redxt", IO{Out: &out, Err: io.Discard})
			if done != tt.wantDone {
				t.Fatalf("done = %v, want %v", done, tt.wantDone)
			}
			if !tt.wantDone {
				return
			}
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Fatalf("output = %q, want it to contain %q", out.String(), tt.wantOut)
			}
		})
	}
}

// TestRunRejectsAnUnknownFlag confirms a bad command line exits with
// the usage code rather than starting the server.
func TestRunRejectsAnUnknownFlag(t *testing.T) {
	code := Run(context.Background(), []string{"--not-a-flag"}, IO{Out: io.Discard, Err: io.Discard})
	if code != ExitUsage {
		t.Fatalf("Run() = %d, want %d", code, ExitUsage)
	}
}

// TestRunStopsOnACancelledContext drives the full sequence and confirms
// step 21 unblocks and tears down cleanly. The context is cancelled from
// the readiness hook so the cancellation lands on the main loop rather
// than on a half-built subsystem.
func TestRunStopsOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var pidPath string
	previous := readyHook
	readyHook = func(server *Server) {
		pidPath = server.Paths.PIDFile
		cancel()
	}
	t.Cleanup(func() { readyHook = previous })

	var out strings.Builder
	code := Run(ctx, testArgs(t), IO{Out: &out, Err: io.Discard})
	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d (output %q)", code, ExitOK, out.String())
	}
	if pidPath == "" {
		t.Fatalf("the sequence never reached step 21")
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("PID file survived the shutdown: %v", err)
	}
}

// TestRunFailsOnACancelledContext confirms a context that is already
// cancelled before the sequence starts is a startup failure rather than
// a clean exit: the database work it carries cannot complete.
func TestRunFailsOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	code := Run(ctx, testArgs(t), IO{Out: io.Discard, Err: io.Discard})
	if code != ExitFailure {
		t.Fatalf("Run() = %d, want %d", code, ExitFailure)
	}
}
