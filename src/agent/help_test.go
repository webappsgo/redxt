package main

import (
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/common/version"
)

func TestHelpMentionsBinaryName(t *testing.T) {
	out := Help("myagent")
	if !strings.Contains(out, "myagent") {
		t.Error("Help() does not mention the renamed binary name")
	}
	if !strings.Contains(out, "--server") || !strings.Contains(out, "--token") {
		t.Error("Help() missing documented flags")
	}
	if !strings.Contains(out, "--status") {
		t.Error("Help() missing --status flag")
	}
}

func TestHelpExcludesUnimplementedFlags(t *testing.T) {
	out := Help("redxt-agent")
	for _, flag := range []string{"--service", "--update"} {
		if strings.Contains(out, flag) {
			t.Errorf("Help() should not document unimplemented flag %q", flag)
		}
	}
}

func TestShellHelpMentionsShells(t *testing.T) {
	out := ShellHelp("redxt-agent")
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(out, shell) {
			t.Errorf("ShellHelp() missing shell %q", shell)
		}
	}
}

func TestVersionLineFormat(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")
	line := VersionLine("redxt-agent")
	want := "redxt-agent 1.2.3 (abc1234) built unknown\n"
	if line != want {
		t.Errorf("VersionLine() = %q, want %q", line, want)
	}
}

func TestVersionLineRenamedBinary(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")
	line := VersionLine("myagent")
	if !strings.HasPrefix(line, "myagent 1.2.3") {
		t.Errorf("VersionLine() = %q, want prefix %q", line, "myagent 1.2.3")
	}
}

func TestUserAgentIgnoresBinaryRename(t *testing.T) {
	version.Set("1.2.3", "abc1234", "0", "redxt.us")
	ua := UserAgent()
	if ua != "redxt-agent/1.2.3" {
		t.Errorf("UserAgent() = %q, want redxt-agent/1.2.3", ua)
	}
}
