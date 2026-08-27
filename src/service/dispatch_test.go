package service

import (
	"bytes"
	"strings"
	"testing"
)

func TestDispatchEmptyArgsShowsHelp(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	var out bytes.Buffer
	if err := m.Dispatch(nil, &out); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(out.String(), "Service management commands:") {
		t.Errorf("expected help output, got %q", out.String())
	}
}

func TestDispatchHelpAndBareHelp(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	for _, arg := range []string{"--help", "help"} {
		var out bytes.Buffer
		if err := m.Dispatch([]string{arg}, &out); err != nil {
			t.Fatalf("Dispatch(%s): %v", arg, err)
		}
		if !strings.Contains(out.String(), "Service management commands:") {
			t.Errorf("Dispatch(%s): expected help output, got %q", arg, out.String())
		}
	}
}

func TestDispatchInstallStartStopRestartReload(t *testing.T) {
	m, runner := newTestManager(t, "linux")
	var out bytes.Buffer
	if err := m.Dispatch([]string{"--install"}, &out); err != nil {
		t.Fatalf("Dispatch(--install): %v", err)
	}

	cases := []struct {
		arg  string
		want []string
	}{
		{"start", []string{"systemctl", "start", "redxt"}},
		{"stop", []string{"systemctl", "stop", "redxt"}},
		{"restart", []string{"systemctl", "restart", "redxt"}},
		{"reload", []string{"systemctl", "reload", "redxt"}},
	}
	for _, tc := range cases {
		runner.calls = nil
		if err := m.Dispatch([]string{tc.arg}, &out); err != nil {
			t.Fatalf("Dispatch(%s): %v", tc.arg, err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("Dispatch(%s): got %v, want one call", tc.arg, runner.calls)
		}
		got := runner.calls[0]
		if len(got) != len(tc.want) {
			t.Fatalf("Dispatch(%s): got %v, want %v", tc.arg, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Dispatch(%s) arg %d: got %q, want %q", tc.arg, i, got[i], tc.want[i])
			}
		}
	}
}

func TestDispatchDisable(t *testing.T) {
	m, runner := newTestManager(t, "linux")
	var out bytes.Buffer
	if err := m.Dispatch([]string{"--install"}, &out); err != nil {
		t.Fatalf("Dispatch(--install): %v", err)
	}
	runner.calls = nil
	if err := m.Dispatch([]string{"--disable"}, &out); err != nil {
		t.Fatalf("Dispatch(--disable): %v", err)
	}
	wantCalls := [][]string{
		{"systemctl", "stop", "redxt"},
		{"systemctl", "disable", "redxt"},
	}
	assertCommandsEqual(t, runner.calls, wantCalls)
}

func TestDispatchUninstallPrintsMessage(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	var out bytes.Buffer
	if err := m.Dispatch([]string{"--install"}, &out); err != nil {
		t.Fatalf("Dispatch(--install): %v", err)
	}
	out.Reset()
	if err := m.Dispatch([]string{"--uninstall"}, &out); err != nil {
		t.Fatalf("Dispatch(--uninstall): %v", err)
	}
	want := "Service uninstalled. Delete binary manually: rm " + m.Ctx.BinaryPath + "\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestDispatchUninstallAbortedPropagatesError(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	m.Confirm = func(string) bool { return false }
	var out bytes.Buffer
	err := m.Dispatch([]string{"--uninstall"}, &out)
	if err != ErrUninstallAborted {
		t.Fatalf("got %v, want ErrUninstallAborted", err)
	}
}

func TestDispatchUnknownSubcommand(t *testing.T) {
	m, _ := newTestManager(t, "linux")
	var out bytes.Buffer
	if err := m.Dispatch([]string{"--bogus"}, &out); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}
