package scheduler

import (
	"testing"
	"time"
)

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"every valid", "@every 15m", false},
		{"every invalid duration", "@every nope", true},
		{"every zero", "@every 0s", true},
		{"hourly", "@hourly", false},
		{"daily", "@daily", false},
		{"midnight", "@midnight", false},
		{"weekly", "@weekly", false},
		{"monthly", "@monthly", false},
		{"cron valid", "0 3 * * *", false},
		{"cron wrong fields", "0 3 * *", true},
		{"cron bad minute", "60 3 * * *", true},
		{"cron step", "*/15 * * * *", false},
		{"cron list", "0 3,6,9 * * *", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchedule(tc.expr)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseSchedule(%q) error = %v, wantErr %v", tc.expr, err, tc.wantErr)
			}
		})
	}
}

func TestScheduleNextCron(t *testing.T) {
	utc := time.UTC
	tests := []struct {
		name  string
		expr  string
		after time.Time
		want  time.Time
	}{
		{
			name:  "daily 3am before",
			expr:  "0 3 * * *",
			after: time.Date(2026, 8, 25, 1, 0, 0, 0, utc),
			want:  time.Date(2026, 8, 25, 3, 0, 0, 0, utc),
		},
		{
			name:  "daily 3am rolls to next day",
			expr:  "0 3 * * *",
			after: time.Date(2026, 8, 25, 3, 0, 0, 0, utc),
			want:  time.Date(2026, 8, 26, 3, 0, 0, 0, utc),
		},
		{
			name:  "weekly sunday",
			expr:  "0 3 * * 0",
			after: time.Date(2026, 8, 25, 0, 0, 0, 0, utc), // Tuesday
			want:  time.Date(2026, 8, 30, 3, 0, 0, 0, utc), // Sunday
		},
		{
			name:  "every 15m",
			expr:  "@every 15m",
			after: time.Date(2026, 8, 25, 1, 3, 0, 0, utc),
			want:  time.Date(2026, 8, 25, 1, 18, 0, 0, utc),
		},
		{
			name:  "hourly fires on the hour, not process start + 1h",
			expr:  "@hourly",
			after: time.Date(2026, 8, 25, 1, 3, 0, 0, utc),
			want:  time.Date(2026, 8, 25, 2, 0, 0, 0, utc),
		},
		{
			name:  "weekly descriptor is sunday midnight",
			expr:  "@weekly",
			after: time.Date(2026, 8, 25, 1, 3, 0, 0, utc), // Tuesday
			want:  time.Date(2026, 8, 30, 0, 0, 0, 0, utc), // Sunday
		},
		{
			name:  "monthly descriptor is first of month midnight",
			expr:  "@monthly",
			after: time.Date(2026, 8, 25, 1, 3, 0, 0, utc),
			want:  time.Date(2026, 9, 1, 0, 0, 0, 0, utc),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := ParseSchedule(tc.expr)
			if err != nil {
				t.Fatalf("ParseSchedule: %v", err)
			}
			got, err := s.Next(tc.after, utc)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("Next(%s) = %s, want %s", tc.after, got, tc.want)
			}
		})
	}
}

func TestDowMatchesSundayAliases(t *testing.T) {
	m, err := parseField("0", 0, 7)
	if err != nil {
		t.Fatalf("parseField: %v", err)
	}
	if !dowMatches(m, time.Sunday) {
		t.Fatalf("dow field 0 should match Sunday")
	}

	m7, err := parseField("7", 0, 7)
	if err != nil {
		t.Fatalf("parseField: %v", err)
	}
	if !dowMatches(m7, time.Sunday) {
		t.Fatalf("dow field 7 should also match Sunday")
	}
	if dowMatches(m7, time.Monday) {
		t.Fatalf("dow field 7 should not match Monday")
	}
}
