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
	for _, section := range []string{"Usage:", "Information:", "Shell Integration:", "Server Configuration:"} {
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
