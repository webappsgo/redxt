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
			args: []string{"--server", "https://example.com", "--token", "adm_agt_abc"},
			want: Options{Server: "https://example.com", Token: "adm_agt_abc", Color: "auto"},
		},
		{
			name: "equals syntax",
			args: []string{"--server=https://example.com"},
			want: Options{Server: "https://example.com", Color: "auto"},
		},
		{
			name: "status flag",
			args: []string{"--status"},
			want: Options{Status: true, Color: "auto"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			got, err := Parse("redxt-agent", tt.args, &errOut)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if got.Help != tt.want.Help || got.Version != tt.want.Version ||
				got.Server != tt.want.Server || got.Token != tt.want.Token ||
				got.Color != tt.want.Color || got.Status != tt.want.Status {
				t.Errorf("Parse() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestParseShellPositional(t *testing.T) {
	var errOut bytes.Buffer
	got, err := Parse("redxt-agent", []string{"--shell", "completions", "bash"}, &errOut)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got.Shell != "completions" || got.ShellName != "bash" {
		t.Errorf("Parse() Shell=%q ShellName=%q, want completions/bash", got.Shell, got.ShellName)
	}
}

func TestDebugFlag(t *testing.T) {
	var errOut bytes.Buffer

	opts, err := Parse("redxt-agent", nil, &errOut)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if opts.DebugFlag() != nil {
		t.Error("DebugFlag() should be nil when --debug was never given")
	}

	opts, err = Parse("redxt-agent", []string{"--debug"}, &errOut)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if opts.DebugFlag() == nil || *opts.DebugFlag() != true {
		t.Error("DebugFlag() should be true when --debug was given")
	}
}

func TestParseInvalidFlag(t *testing.T) {
	var errOut bytes.Buffer
	if _, err := Parse("redxt-agent", []string{"--bogus"}, &errOut); err == nil {
		t.Fatal("Parse() expected error for unknown flag")
	}
}

func TestParseCommandLeftover(t *testing.T) {
	var errOut bytes.Buffer
	got, err := Parse("redxt-agent", []string{"status"}, &errOut)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if len(got.Args) != 1 || got.Args[0] != "status" {
		t.Errorf("Parse() Args = %v, want [status]", got.Args)
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName(); got == "" {
		t.Error("BinaryName() returned empty string")
	}
}
