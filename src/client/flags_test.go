package main

import (
	"bytes"
	"testing"
)

func TestParseBasicFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Options
	}{
		{
			name: "help short",
			args: []string{"-h"},
			want: Options{Help: true, Color: "auto"},
		},
		{
			name: "version long",
			args: []string{"--version"},
			want: Options{Version: true, Color: "auto"},
		},
		{
			name: "server and token",
			args: []string{"--server", "https://example.com", "--token", "abc"},
			want: Options{Server: "https://example.com", Token: "abc", Color: "auto"},
		},
		{
			name: "equals syntax",
			args: []string{"--server=https://example.com", "--color=yes"},
			want: Options{Server: "https://example.com", Color: "yes"},
		},
		{
			name: "positional command",
			args: []string{"health"},
			want: Options{Color: "auto", Args: []string{"health"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errBuf bytes.Buffer
			got, err := Parse("redxt-cli", tt.args, &errBuf)
			if err != nil {
				t.Fatalf("Parse() error: %v (stderr: %s)", err, errBuf.String())
			}
			if got.Help != tt.want.Help || got.Version != tt.want.Version ||
				got.Server != tt.want.Server || got.Token != tt.want.Token ||
				got.Color != tt.want.Color {
				t.Errorf("Parse() = %+v, want %+v", *got, tt.want)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("Parse() Args = %v, want %v", got.Args, tt.want.Args)
			}
		})
	}
}

func TestParseShellPositional(t *testing.T) {
	var errBuf bytes.Buffer
	got, err := Parse("redxt-cli", []string{"--shell", "completions", "bash"}, &errBuf)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got.Shell != "completions" {
		t.Errorf("Shell = %q, want completions", got.Shell)
	}
	if got.ShellName != "bash" {
		t.Errorf("ShellName = %q, want bash", got.ShellName)
	}
}

func TestDebugFlag(t *testing.T) {
	var errBuf bytes.Buffer

	unset, err := Parse("redxt-cli", nil, &errBuf)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if unset.DebugFlag() != nil {
		t.Error("DebugFlag() should be nil when --debug was never given")
	}

	set, err := Parse("redxt-cli", []string{"--debug"}, &errBuf)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	flag := set.DebugFlag()
	if flag == nil || *flag != true {
		t.Errorf("DebugFlag() = %v, want pointer to true", flag)
	}
}

func TestParseInvalidFlag(t *testing.T) {
	var errBuf bytes.Buffer
	if _, err := Parse("redxt-cli", []string{"--nope"}, &errBuf); err == nil {
		t.Fatal("Parse() expected error for unknown flag")
	}
}

func TestBinaryName(t *testing.T) {
	if BinaryName() == "" {
		t.Error("BinaryName() returned empty string")
	}
}
