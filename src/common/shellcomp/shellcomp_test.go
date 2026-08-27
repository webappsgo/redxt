package shellcomp

import (
	"strings"
	"testing"
)

func testSpec() Spec {
	return Spec{
		Flags:     []string{"--help", "--version", "--config", "--pid", "--color"},
		DirFlags:  []string{"--config"},
		FileFlags: []string{"--pid"},
		EnumFlags: map[string][]string{"--color": {"auto", "yes", "no"}},
	}
}

func TestShells(t *testing.T) {
	got := Shells()
	want := []string{"bash", "zsh", "fish", "sh", "dash", "ksh", "powershell", "pwsh"}
	if len(got) != len(want) {
		t.Fatalf("Shells() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Shells()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Mutating the returned slice must not affect the package's list.
	got[0] = "mutated"
	if Shells()[0] != "bash" {
		t.Error("Shells() returned a slice aliasing internal state")
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{"bash path", "/bin/bash", "bash"},
		{"zsh path", "/usr/bin/zsh", "zsh"},
		{"empty falls back", "", "bash"},
		{"root falls back", "/", "bash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			if got := DetectShell(); got != tt.want {
				t.Errorf("DetectShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompletionsSupportedShells(t *testing.T) {
	spec := testSpec()
	for _, shell := range Shells() {
		t.Run(shell, func(t *testing.T) {
			script, err := Completions(spec, shell, "mycli")
			if err != nil {
				t.Fatalf("Completions(%q) error: %v", shell, err)
			}
			if !strings.Contains(script, "mycli") {
				t.Errorf("Completions(%q) output does not mention binary name", shell)
			}
		})
	}
}

func TestCompletionsUnsupportedShell(t *testing.T) {
	if _, err := Completions(testSpec(), "nushell", "mycli"); err == nil {
		t.Fatal("Completions(nushell) expected error, got nil")
	}
}

func TestCompletionsDeterministic(t *testing.T) {
	spec := testSpec()
	a, err := Completions(spec, "bash", "mycli")
	if err != nil {
		t.Fatalf("Completions error: %v", err)
	}
	b, err := Completions(spec, "bash", "mycli")
	if err != nil {
		t.Fatalf("Completions error: %v", err)
	}
	if a != b {
		t.Error("Completions(bash) is not deterministic across calls")
	}
}

func TestInit(t *testing.T) {
	tests := []struct {
		shell   string
		want    string
		wantErr bool
	}{
		{"bash", "source <(mycli --shell completions bash)\n", false},
		{"zsh", "source <(mycli --shell completions zsh)\n", false},
		{"fish", "mycli --shell completions fish | source\n", false},
		{"sh", "eval \"$(mycli --shell completions sh)\"\n", false},
		{"powershell", "Invoke-Expression (& mycli --shell completions powershell)\n", false},
		{"nushell", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			got, err := Init(tt.shell, "mycli")
			if tt.wantErr {
				if err == nil {
					t.Fatal("Init() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Init() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Init() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFunctionNameSanitizes(t *testing.T) {
	spec := testSpec()
	script, err := Completions(spec, "bash", "my-paste.exe")
	if err != nil {
		t.Fatalf("Completions error: %v", err)
	}
	if strings.Contains(script, "_my-paste.exe_completions") {
		t.Error("bash function name was not sanitized")
	}
	if !strings.Contains(script, "_my_paste_exe_completions") {
		t.Errorf("expected sanitized function name in script, got: %s", script)
	}
}

func TestFunctionNameEmpty(t *testing.T) {
	script, err := Completions(testSpec(), "bash", "")
	if err != nil {
		t.Fatalf("Completions error: %v", err)
	}
	if !strings.Contains(script, "__completion_completions") {
		t.Errorf("expected fallback function name, got: %s", script)
	}
}
