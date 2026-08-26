package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "bare number is seconds", in: "30", want: 30 * time.Second},
		{name: "zero", in: "0", want: 0},
		{name: "seconds unit", in: "45s", want: 45 * time.Second},
		{name: "minutes", in: "5m", want: 5 * time.Minute},
		{name: "hours", in: "2h", want: 2 * time.Hour},
		{name: "milliseconds", in: "250ms", want: 250 * time.Millisecond},
		{name: "days", in: "3d", want: 72 * time.Hour},
		{name: "weeks", in: "2w", want: 14 * 24 * time.Hour},
		{name: "years", in: "1y", want: 365 * 24 * time.Hour},
		{name: "compound go units", in: "1h30m", want: 90 * time.Minute},
		{name: "surrounding space", in: "  10m  ", want: 10 * time.Minute},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "soon", wantErr: true},
		{name: "unknown unit", in: "5q", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseDuration(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDuration(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseDuration(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{name: "bare bytes", in: "1024", want: 1024},
		{name: "kilobytes", in: "10KB", want: 10 * 1024},
		{name: "megabytes", in: "50MB", want: 50 * 1024 * 1024},
		{name: "gigabytes", in: "2GB", want: 2 * 1024 * 1024 * 1024},
		{name: "short suffix", in: "5M", want: 5 * 1024 * 1024},
		{name: "byte suffix", in: "512B", want: 512},
		{name: "lowercase", in: "50mb", want: 50 * 1024 * 1024},
		{name: "spaced", in: " 50 MB ", want: 50 * 1024 * 1024},
		{name: "empty", in: "", wantErr: true},
		{name: "garbage", in: "big", wantErr: true},
		{name: "negative", in: "-5MB", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseByteSize(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseByteSize(%q) = %d, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseByteSize(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ParseByteSize(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestDurationYAMLRoundTrip(t *testing.T) {
	type holder struct {
		D Duration `yaml:"d"`
	}
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{name: "bare int is seconds", in: "d: 30\n", want: 30 * time.Second},
		{name: "quoted unit string", in: "d: \"5m\"\n", want: 5 * time.Minute},
		{name: "unquoted unit string", in: "d: 90d\n", want: 90 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h holder
			if err := yaml.Unmarshal([]byte(tt.in), &h); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if h.D.Duration() != tt.want {
				t.Fatalf("got %v, want %v", h.D.Duration(), tt.want)
			}
			out, err := yaml.Marshal(h)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back holder
			if err := yaml.Unmarshal(out, &back); err != nil {
				t.Fatalf("re-unmarshal %q: %v", out, err)
			}
			if back.D.Duration() != tt.want {
				t.Fatalf("round trip through %q gave %v, want %v", out, back.D.Duration(), tt.want)
			}
		})
	}
}

func TestByteSizeYAMLRoundTrip(t *testing.T) {
	type holder struct {
		S ByteSize `yaml:"s"`
	}
	tests := []struct {
		name string
		in   string
		want int64
	}{
		{name: "bare int is bytes", in: "s: 4096\n", want: 4096},
		{name: "suffixed string", in: "s: 50MB\n", want: 50 * 1024 * 1024},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h holder
			if err := yaml.Unmarshal([]byte(tt.in), &h); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if h.S.Bytes() != tt.want {
				t.Fatalf("got %d, want %d", h.S.Bytes(), tt.want)
			}
			out, err := yaml.Marshal(h)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back holder
			if err := yaml.Unmarshal(out, &back); err != nil {
				t.Fatalf("re-unmarshal %q: %v", out, err)
			}
			if back.S.Bytes() != tt.want {
				t.Fatalf("round trip through %q gave %d, want %d", out, back.S.Bytes(), tt.want)
			}
		})
	}
}
