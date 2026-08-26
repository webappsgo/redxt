package cli

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, o *Options)
	}{
		{
			name: "no arguments leaves everything unset",
			args: nil,
			check: func(t *testing.T, o *Options) {
				if o.Help || o.Version || o.Status || o.Daemon || o.Debug {
					t.Fatalf("expected all booleans false, got %+v", o)
				}
				if o.Color != "auto" {
					t.Fatalf("Color = %q, want auto", o.Color)
				}
			},
		},
		{
			name: "short help",
			args: []string{"-h"},
			check: func(t *testing.T, o *Options) {
				if !o.Help {
					t.Fatalf("expected Help true")
				}
			},
		},
		{
			name: "short version",
			args: []string{"-v"},
			check: func(t *testing.T, o *Options) {
				if !o.Version {
					t.Fatalf("expected Version true")
				}
			},
		},
		{
			name: "status",
			args: []string{"--status"},
			check: func(t *testing.T, o *Options) {
				if !o.Status {
					t.Fatalf("expected Status true")
				}
			},
		},
		{
			name: "shell with explicit shell name",
			args: []string{"--shell", "completions", "zsh"},
			check: func(t *testing.T, o *Options) {
				if o.Shell != "completions" || o.ShellName != "zsh" {
					t.Fatalf("Shell = %q, ShellName = %q", o.Shell, o.ShellName)
				}
			},
		},
		{
			name: "shell without shell name",
			args: []string{"--shell", "init"},
			check: func(t *testing.T, o *Options) {
				if o.Shell != "init" || o.ShellName != "" {
					t.Fatalf("Shell = %q, ShellName = %q", o.Shell, o.ShellName)
				}
			},
		},
		{
			name: "directories",
			args: []string{
				"--config", "/etc/x", "--data", "/var/lib/x", "--cache", "/var/cache/x",
				"--log", "/var/log/x", "--backup", "/mnt/b", "--pid", "/run/x.pid",
			},
			check: func(t *testing.T, o *Options) {
				got := []string{o.Config, o.Data, o.Cache, o.Log, o.Backup, o.PIDFile}
				want := []string{"/etc/x", "/var/lib/x", "/var/cache/x", "/var/log/x", "/mnt/b", "/run/x.pid"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("directories = %v, want %v", got, want)
				}
			},
		},
		{
			name: "listen configuration",
			args: []string{"--address", "127.0.0.1", "--port", "80,443", "--baseurl", "/app"},
			check: func(t *testing.T, o *Options) {
				if o.Address != "127.0.0.1" || o.Port != "80,443" || o.BaseURL != "/app" {
					t.Fatalf("listen options = %+v", o)
				}
				ports, err := o.Ports()
				if err != nil {
					t.Fatalf("Ports() error = %v", err)
				}
				if !reflect.DeepEqual(ports, []int{80, 443}) {
					t.Fatalf("Ports() = %v, want [80 443]", ports)
				}
			},
		},
		{
			name: "single-dash long flags are accepted",
			args: []string{"-mode", "debug", "-daemon"},
			check: func(t *testing.T, o *Options) {
				if o.Mode != "debug" || !o.Daemon {
					t.Fatalf("options = %+v", o)
				}
			},
		},
		{
			name: "explicit debug false",
			args: []string{"--debug=false"},
			check: func(t *testing.T, o *Options) {
				if !o.DebugSet {
					t.Fatalf("expected DebugSet true")
				}
				flag := o.DebugFlag()
				if flag == nil || *flag {
					t.Fatalf("DebugFlag() = %v, want pointer to false", flag)
				}
			},
		},
		{
			name: "lang and color",
			args: []string{"--lang", "es", "--color", "no"},
			check: func(t *testing.T, o *Options) {
				if o.Lang != "es" || o.Color != "no" {
					t.Fatalf("options = %+v", o)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := Parse("redxt", tt.args, io.Discard)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			tt.check(t, o)
		})
	}
}

// TestParseUnknownFlag confirms an unknown flag is an error the caller
// handles rather than an os.Exit inside the flag package.
func TestParseUnknownFlag(t *testing.T) {
	var out strings.Builder
	if _, err := Parse("redxt", []string{"--nope"}, &out); err == nil {
		t.Fatalf("expected an error for an unknown flag")
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("usage output missing: %q", out.String())
	}
}

// TestDebugFlagUnset guards the precedence rule: without --debug the
// DEBUG environment variable must still decide the mode.
func TestDebugFlagUnset(t *testing.T) {
	o, err := Parse("redxt", nil, io.Discard)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if o.DebugFlag() != nil {
		t.Fatalf("DebugFlag() = %v, want nil when --debug is absent", o.DebugFlag())
	}
}

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []int
		wantErr bool
	}{
		{name: "empty", raw: "", want: nil},
		{name: "whitespace only", raw: "   ", want: nil},
		{name: "single", raw: "8080", want: []int{8080}},
		{name: "dual", raw: "80,443", want: []int{80, 443}},
		{name: "spaced dual", raw: " 80 , 443 ", want: []int{80, 443}},
		{name: "lowest", raw: "1", want: []int{1}},
		{name: "highest", raw: "65535", want: []int{65535}},
		{name: "three ports", raw: "80,443,8080", wantErr: true},
		{name: "not a number", raw: "http", wantErr: true},
		{name: "zero", raw: "0", wantErr: true},
		{name: "too high", raw: "65536", wantErr: true},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "empty second field", raw: "80,", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePorts(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePorts(%q) = %v, want an error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePorts(%q) error = %v", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParsePorts(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestBinaryName covers the renaming rule: user-facing output uses the
// invoked name, with the Windows suffix removed.
func TestBinaryName(t *testing.T) {
	tests := []struct {
		name string
		argv string
		want string
	}{
		{name: "bare name", argv: "redxt", want: "redxt"},
		{name: "absolute path", argv: "/usr/local/bin/redxt", want: "redxt"},
		{name: "renamed binary", argv: "/opt/dns/mydns", want: "mydns"},
		{name: "windows suffix", argv: "/opt/dns/redxt.exe", want: "redxt"},
	}

	original := os.Args
	t.Cleanup(func() { os.Args = original })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = []string{tt.argv}
			if got := BinaryName(); got != tt.want {
				t.Fatalf("BinaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}
