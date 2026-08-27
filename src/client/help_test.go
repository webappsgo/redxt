package main

import (
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/common/version"
)

func TestHelpMentionsBinaryName(t *testing.T) {
	out := Help("mypaste")
	if !strings.Contains(out, "mypaste") {
		t.Error("Help() does not mention the renamed binary name")
	}
	if !strings.Contains(out, "--server") || !strings.Contains(out, "--token") {
		t.Error("Help() missing documented flags")
	}
	if !strings.Contains(out, "TUI mode") {
		t.Error("Help() should document TUI mode")
	}
}

func TestShellHelpMentionsShells(t *testing.T) {
	out := ShellHelp("redxt-cli")
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(out, shell) {
			t.Errorf("ShellHelp() missing shell %q", shell)
		}
	}
}

func TestVersionLineFormat(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")
	line := VersionLine("redxt-cli")
	want := "redxt-cli 1.2.3 (abc1234) built unknown\n"
	if line != want {
		t.Errorf("VersionLine() = %q, want %q", line, want)
	}
}

func TestVersionLineRenamedBinary(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")
	line := VersionLine("mypaste")
	if !strings.HasPrefix(line, "mypaste 1.2.3") {
		t.Errorf("VersionLine() = %q, want prefix %q", line, "mypaste 1.2.3")
	}
}

func TestUserAgentIgnoresBinaryRename(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")
	ua := UserAgent()
	if ua != "redxt-cli/1.2.3" {
		t.Errorf("UserAgent() = %q, want redxt-cli/1.2.3", ua)
	}
}
