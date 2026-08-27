package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/health"
)

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--help"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("Run(--help) = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "--server") {
		t.Error("Run(--help) did not print help text")
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--version"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("Run(--version) = %d, want 0", code)
	}
	if out.Len() == 0 {
		t.Error("Run(--version) produced no output")
	}
}

func TestRunInvalidFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--bogus"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 2 {
		t.Fatalf("Run(--bogus) = %d, want 2", code)
	}
}

func TestRunShellHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--shell", "help"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("Run(--shell help) = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "bash") {
		t.Error("Run(--shell help) missing shell list")
	}
}

func TestRunShellCompletions(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--shell", "completions", "bash"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("Run(--shell completions bash) = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if out.Len() == 0 {
		t.Error("Run(--shell completions bash) produced no output")
	}
}

func TestRunShellUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"--shell", "bogus"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 1 {
		t.Fatalf("Run(--shell bogus) = %d, want 1", code)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)

	var out, errOut bytes.Buffer
	code := Run([]string{"--server", "http://127.0.0.1:0", "bogus"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 2 {
		t.Fatalf("Run(bogus) = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Error("Run(bogus) should report the unknown command")
	}
}

func TestRunHealthCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health.Response{Status: health.StatusHealthy})
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)

	var out, errOut bytes.Buffer
	code := Run([]string{"--server", srv.URL, "health"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("Run(health) = %d, want 0 (stderr: %s)", code, errOut.String())
	}
}

func TestRunNoArgsLaunchesTUI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)

	var out, errOut bytes.Buffer
	code := Run(nil, IO{In: strings.NewReader("quit\n"), Out: &out, Err: &errOut})
	if code != 0 {
		t.Fatalf("Run(nil) = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "interactive mode") {
		t.Error("Run(nil) did not launch the TUI")
	}
}

func TestRunTokenFileMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)

	var out, errOut bytes.Buffer
	code := Run([]string{"--token-file", dir + "/missing"}, IO{In: strings.NewReader(""), Out: &out, Err: &errOut})
	if code != 1 {
		t.Fatalf("Run(--token-file missing) = %d, want 1", code)
	}
}
