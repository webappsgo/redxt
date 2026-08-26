package format

import (
	"testing"
	"time"
)

func TestDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero", d: 0, want: "0 seconds"},
		{name: "sub second", d: 500 * time.Millisecond, want: "0 seconds"},
		{name: "one second", d: time.Second, want: "1 second"},
		{name: "forty five seconds", d: 45 * time.Second, want: "45 seconds"},
		{name: "fifty nine seconds", d: 59 * time.Second, want: "59 seconds"},
		{name: "one minute", d: time.Minute, want: "1 minute"},
		{name: "ninety seconds", d: 90 * time.Second, want: "1 minute 30 seconds"},
		{name: "two minutes five seconds", d: 125 * time.Second, want: "2 minutes 5 seconds"},
		{name: "three minutes", d: 3 * time.Minute, want: "3 minutes"},
		{name: "one hour", d: time.Hour, want: "1 hour"},
		{name: "one hour thirty minutes", d: 90 * time.Minute, want: "1 hour 30 minutes"},
		{name: "two hours", d: 2 * time.Hour, want: "2 hours"},
		{name: "hours drop seconds", d: 2*time.Hour + 30*time.Minute + 15*time.Second, want: "2 hours 30 minutes"},
		{name: "one day", d: 24 * time.Hour, want: "1 day"},
		{name: "three days four hours", d: 76 * time.Hour, want: "3 days 4 hours"},
		{name: "two days five hours thirty minutes", d: 53*time.Hour + 30*time.Minute, want: "2 days 5 hours"},
		{name: "six days", d: 6 * 24 * time.Hour, want: "6 days"},
		{name: "one week", d: 7 * 24 * time.Hour, want: "1 week"},
		{name: "one week one day", d: 8 * 24 * time.Hour, want: "1 week 1 day"},
		{name: "one month", d: 30 * 24 * time.Hour, want: "1 month"},
		{name: "two months", d: 61 * 24 * time.Hour, want: "2 months"},
		{name: "one year", d: 365 * 24 * time.Hour, want: "1 year"},
		{name: "one year one month", d: 400 * 24 * time.Hour, want: "1 year 1 month"},
		{name: "negative", d: -90 * time.Second, want: "-1 minute 30 seconds"},
		{name: "negative sub second", d: -500 * time.Millisecond, want: "0 seconds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Duration(tt.d); got != tt.want {
				t.Errorf("Duration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestSize(t *testing.T) {
	const (
		kb = int64(1024)
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
		pb = tb * 1024
	)

	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 bytes"},
		{name: "one byte", bytes: 1, want: "1 byte"},
		{name: "two bytes", bytes: 2, want: "2 bytes"},
		{name: "512 bytes", bytes: 512, want: "512 bytes"},
		{name: "1023 bytes", bytes: 1023, want: "1023 bytes"},
		{name: "one kilobyte", bytes: kb, want: "1 kilobyte"},
		{name: "one and a half kilobytes", bytes: 1536, want: "1.5 kilobytes"},
		{name: "two kilobytes", bytes: 2 * kb, want: "2 kilobytes"},
		{name: "one megabyte", bytes: mb, want: "1 megabyte"},
		{name: "two and a half megabytes", bytes: mb * 5 / 2, want: "2.5 megabytes"},
		{name: "five gigabytes", bytes: 5 * gb, want: "5 gigabytes"},
		{name: "one point two terabytes", bytes: tb*6/5 + 1, want: "1.2 terabytes"},
		{name: "three petabytes", bytes: 3 * pb, want: "3 petabytes"},
		{name: "negative kilobyte", bytes: -kb, want: "-1 kilobyte"},
		{name: "negative bytes", bytes: -512, want: "-512 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Size(tt.bytes); got != tt.want {
				t.Errorf("Size(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{name: "zero", n: 0, want: "0"},
		{name: "one", n: 1, want: "1"},
		{name: "999", n: 999, want: "999"},
		{name: "1000", n: 1000, want: "1,000"},
		{name: "12847", n: 12847, want: "12,847"},
		{name: "1234567", n: 1234567, want: "1,234,567"},
		{name: "negative small", n: -42, want: "-42"},
		{name: "negative large", n: -1234567, want: "-1,234,567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Count(tt.n); got != tt.want {
				t.Errorf("Count(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestTimestamp(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "documented example",
			in:   time.Date(2026, time.January, 5, 14, 3, 7, 0, time.UTC),
			want: "January 05, 2026 at 14:03:07 UTC",
		},
		{
			name: "double digit day",
			in:   time.Date(2025, time.December, 31, 23, 59, 59, 0, time.UTC),
			want: "December 31, 2025 at 23:59:59 UTC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Timestamp(tt.in); got != tt.want {
				t.Errorf("Timestamp() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUptime(t *testing.T) {
	tests := []struct {
		name  string
		since time.Duration
		want  string
	}{
		{name: "just started", since: 0, want: "0 seconds"},
		{name: "two minutes", since: 2 * time.Minute, want: "2 minutes"},
		{name: "one hour thirty", since: 90 * time.Minute, want: "1 hour 30 minutes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now().Add(-tt.since)
			if got := Uptime(start); got != tt.want {
				t.Errorf("Uptime() = %q, want %q", got, tt.want)
			}
		})
	}
}
