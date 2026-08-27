package cli

import (
	"strings"
	"testing"
)

// TestHelpListsEveryFlag guards the PART 8 rule that the server command
// set is fixed: every flag the parser accepts must be documented.
func TestHelpListsEveryFlag(t *testing.T) {
	help := Help("redxt")
	for _, flag := range completionFlags {
		if !strings.Contains(help, flag) {
			t.Fatalf("--help does not document %q", flag)
		}
	}
	for _, section := range []string{"Usage:", "Information:", "Shell Integration:", "Server Configuration:", "Service Management:"} {
		if !strings.Contains(help, section) {
			t.Fatalf("--help is missing the %q section", section)
		}
	}
}

// TestHelpUsesTheInvokedName covers the renaming rule for help output.
func TestHelpUsesTheInvokedName(t *testing.T) {
	help := Help("mydns")
	if strings.Contains(help, "redxt") {
		t.Fatalf("--help leaked the project name: %q", help)
	}
	if !strings.Contains(help, "mydns [flags]") {
		t.Fatalf("--help usage line is missing the invoked name: %q", help)
	}
}

// TestMaintenanceHelp checks the PART 24 maintenance block documents
// every subcommand and resolves the real backup directory.
func TestMaintenanceHelp(t *testing.T) {
	out := MaintenanceHelp("redxt", "/var/backups/redxt/")
	for _, subcommand := range []string{"backup [file]", "restore <file>", "update [cmd]", "mode <mode>", "setup"} {
		if !strings.Contains(out, subcommand) {
			t.Fatalf("--maintenance help does not document %q: %q", subcommand, out)
		}
	}
	if !strings.Contains(out, "Default: /var/backups/redxt/redxt-{timestamp}.tar.gz") {
		t.Fatalf("--maintenance help has the wrong default backup path: %q", out)
	}
	if !strings.Contains(out, "  redxt --maintenance setup\n") {
		t.Fatalf("--maintenance help is missing the examples block: %q", out)
	}
}

// TestUpdateHelp checks the PART 24 update block and its conditional
// "Latest" line.
func TestUpdateHelp(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		latest     string
		wantLatest bool
	}{
		{name: "no check made", current: "1.0.0", latest: "", wantLatest: false},
		{name: "already current", current: "1.0.0", latest: "1.0.0", wantLatest: false},
		{name: "newer release", current: "1.0.0", latest: "1.1.0", wantLatest: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := UpdateHelp("redxt", tt.current, "stable", tt.latest)
			if !strings.Contains(out, "branch <name>") {
				t.Fatalf("--update help does not document branch: %q", out)
			}
			if got := strings.Contains(out, "Latest:"); got != tt.wantLatest {
				t.Fatalf("Latest line present = %v, want %v: %q", got, tt.wantLatest, out)
			}
		})
	}
}

func TestShellHelp(t *testing.T) {
	out := ShellHelp("redxt")
	for _, subcommand := range []string{"completions", "init", "help"} {
		if !strings.Contains(out, "--shell "+subcommand) {
			t.Fatalf("--shell help does not document %q: %q", subcommand, out)
		}
	}
	for _, shell := range SupportedShells() {
		if !strings.Contains(out, shell) {
			t.Fatalf("--shell help does not list %q: %q", shell, out)
		}
	}
}
