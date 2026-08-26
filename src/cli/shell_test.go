package cli

import (
	"io"
	"strings"
	"testing"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{name: "unset", shell: "", want: "bash"},
		{name: "whitespace", shell: "   ", want: "bash"},
		{name: "bash", shell: "/bin/bash", want: "bash"},
		{name: "zsh", shell: "/usr/bin/zsh", want: "zsh"},
		{name: "fish", shell: "/usr/local/bin/fish", want: "fish"},
		{name: "bare name", shell: "ksh", want: "ksh"},
		{name: "root only", shell: "/", want: "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			if got := DetectShell(); got != tt.want {
				t.Fatalf("DetectShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCompletionsCoverEveryShell asserts that every advertised shell has
// a generator, so --shell help can never promise an unusable shell.
func TestCompletionsCoverEveryShell(t *testing.T) {
	for _, shell := range SupportedShells() {
		t.Run(shell, func(t *testing.T) {
			script, err := Completions(shell, "redxt")
			if err != nil {
				t.Fatalf("Completions(%q) error = %v", shell, err)
			}
			if strings.TrimSpace(script) == "" {
				t.Fatalf("Completions(%q) produced an empty script", shell)
			}
			if !strings.Contains(script, "redxt") {
				t.Fatalf("Completions(%q) does not name the binary: %q", shell, script)
			}
			if !strings.HasSuffix(script, "\n") {
				t.Fatalf("Completions(%q) does not end with a newline", shell)
			}
		})
	}
}

// TestCompletionsUseTheInvokedName covers the renaming rule: a renamed
// binary must generate completions bound to its own name.
func TestCompletionsUseTheInvokedName(t *testing.T) {
	for _, shell := range SupportedShells() {
		script, err := Completions(shell, "mydns")
		if err != nil {
			t.Fatalf("Completions(%q) error = %v", shell, err)
		}
		if strings.Contains(script, "redxt") {
			t.Fatalf("Completions(%q) leaked the project name: %q", shell, script)
		}
		if !strings.Contains(script, "mydns") {
			t.Fatalf("Completions(%q) missing the invoked name: %q", shell, script)
		}
	}
}

func TestCompletionsUnsupportedShell(t *testing.T) {
	if _, err := Completions("tcsh", "redxt"); err == nil {
		t.Fatalf("expected an error for an unsupported shell")
	}
}

// TestCompletionsCaseInsensitive keeps PowerShell's conventional casing
// working.
func TestCompletionsCaseInsensitive(t *testing.T) {
	script, err := Completions("PowerShell", "redxt")
	if err != nil {
		t.Fatalf("Completions() error = %v", err)
	}
	if !strings.Contains(script, "Register-ArgumentCompleter") {
		t.Fatalf("unexpected PowerShell script: %q", script)
	}
}

func TestInit(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{name: "bash", shell: "bash", want: "source <(redxt --shell completions bash)"},
		{name: "zsh", shell: "zsh", want: "source <(redxt --shell completions zsh)"},
		{name: "fish", shell: "fish", want: "redxt --shell completions fish | source"},
		{name: "sh", shell: "sh", want: `eval "$(redxt --shell completions sh)"`},
		{name: "dash", shell: "dash", want: `eval "$(redxt --shell completions dash)"`},
		{name: "ksh", shell: "ksh", want: `eval "$(redxt --shell completions ksh)"`},
		{name: "powershell", shell: "powershell", want: "Invoke-Expression (& redxt --shell completions powershell)"},
		{name: "pwsh", shell: "pwsh", want: "Invoke-Expression (& redxt --shell completions powershell)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Init(tt.shell, "redxt")
			if err != nil {
				t.Fatalf("Init(%q) error = %v", tt.shell, err)
			}
			if got != tt.want+"\n" {
				t.Fatalf("Init(%q) = %q, want %q", tt.shell, got, tt.want+"\n")
			}
		})
	}
}

func TestInitUnsupportedShell(t *testing.T) {
	if _, err := Init("tcsh", "redxt"); err == nil {
		t.Fatalf("expected an error for an unsupported shell")
	}
}

func TestRunShell(t *testing.T) {
	tests := []struct {
		name       string
		subcommand string
		shellName  string
		wantCode   int
		wantOut    string
		wantErrOut string
	}{
		{name: "help", subcommand: "help", wantCode: 0, wantOut: "shell integration"},
		{name: "completions", subcommand: "completions", shellName: "bash", wantCode: 0, wantOut: "complete -F"},
		{name: "init", subcommand: "init", shellName: "fish", wantCode: 0, wantOut: "| source"},
		{name: "bad shell", subcommand: "completions", shellName: "tcsh", wantCode: 1, wantErrOut: "unsupported shell"},
		{name: "bad init shell", subcommand: "init", shellName: "tcsh", wantCode: 1, wantErrOut: "unsupported shell"},
		{name: "unknown command", subcommand: "explode", wantCode: 1, wantErrOut: "unknown --shell command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", "/bin/bash")
			var out, errOut strings.Builder
			code := RunShell(tt.subcommand, tt.shellName, "redxt", &out, &errOut)
			if code != tt.wantCode {
				t.Fatalf("RunShell() = %d, want %d (stderr %q)", code, tt.wantCode, errOut.String())
			}
			if tt.wantOut != "" && !strings.Contains(out.String(), tt.wantOut) {
				t.Fatalf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
			if tt.wantErrOut != "" && !strings.Contains(errOut.String(), tt.wantErrOut) {
				t.Fatalf("stderr = %q, want it to contain %q", errOut.String(), tt.wantErrOut)
			}
		})
	}
}

// TestRunShellDetectsTheShell covers the optional [SHELL] argument: an
// omitted shell must fall back to $SHELL.
func TestRunShellDetectsTheShell(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	var out, errOut strings.Builder
	if code := RunShell("completions", "", "redxt", &out, &errOut); code != 0 {
		t.Fatalf("RunShell() = %d, stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "complete -c redxt") {
		t.Fatalf("expected a fish script, got %q", out.String())
	}
}

func TestFunctionName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "redxt", want: "redxt"},
		{name: "hyphen", in: "redxt-agent", want: "redxt_agent"},
		{name: "dot", in: "redxt.exe", want: "redxt_exe"},
		{name: "empty", in: "", want: "_completion"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := functionName(tt.in); got != tt.want {
				t.Fatalf("functionName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestCompletionFlagsMatchTheParser is the drift guard: every flag the
// completions advertise must actually be registered by the parser.
func TestCompletionFlagsMatchTheParser(t *testing.T) {
	for _, flag := range completionFlags {
		if _, err := Parse("redxt", []string{flag + "=x"}, io.Discard); err != nil &&
			strings.Contains(err.Error(), "flag provided but not defined") {
			t.Fatalf("completion flag %q is not registered by Parse", flag)
		}
	}
}
