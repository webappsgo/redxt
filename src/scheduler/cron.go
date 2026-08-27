// Package scheduler implements AI.md PART 19: the always-running,
// database-backed, cluster-aware internal task scheduler that
// replaces every external cron mechanism in the product.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// fieldMatcher reports whether a single cron field value matches.
type fieldMatcher func(v int) bool

// schedule is a parsed cron-style expression: either five
// minute/hour/day-of-month/month/day-of-week fields, or an "@every"
// fixed interval.
type schedule struct {
	every time.Duration // non-zero for "@every"/"@hourly"/"@daily"/"@midnight"

	minute fieldMatcher
	hour   fieldMatcher
	dom    fieldMatcher
	month  fieldMatcher
	dow    fieldMatcher
}

// ParseSchedule parses a schedule string in either the standard
// 5-field cron format ("0 3 * * *") or the "@every <duration>",
// "@hourly", "@daily"/"@midnight" shorthand AI.md PART 19 uses.
func ParseSchedule(expr string) (*schedule, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case strings.HasPrefix(expr, "@every "):
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(expr, "@every ")))
		if err != nil {
			return nil, fmt.Errorf("invalid @every duration %q: %w", expr, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("invalid @every duration %q: must be positive", expr)
		}
		return &schedule{every: d}, nil
	case expr == "@hourly":
		return &schedule{every: time.Hour}, nil
	case expr == "@daily", expr == "@midnight":
		return parseCron("0 0 * * *")
	default:
		return parseCron(expr)
	}
}

func parseCron(expr string) (*schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression %q: want 5 fields, got %d", expr, len(fields))
	}
	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	dow, err := parseField(fields[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}
	return &schedule{minute: minute, hour: hour, dom: dom, month: month, dow: dow}, nil
}

// parseField parses one cron field: "*", "N", "N,M,...", or "*/N".
func parseField(field string, min, max int) (fieldMatcher, error) {
	if field == "*" {
		return func(int) bool { return true }, nil
	}
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(strings.TrimPrefix(field, "*/"))
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step value %q", field)
		}
		return func(v int) bool { return (v-min)%step == 0 }, nil
	}
	parts := strings.Split(field, ",")
	values := make(map[int]bool, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < min || n > max {
			return nil, fmt.Errorf("invalid value %q", p)
		}
		values[n] = true
	}
	return func(v int) bool { return values[v] }, nil
}

// maxLookahead bounds how far into the future NextRun searches before
// giving up, so a contradictory field combination (e.g. Feb 30) fails
// fast instead of looping forever.
const maxLookahead = 4 * 366 * 24 * time.Hour

// Next returns the first minute-aligned instant strictly after
// `after` that this schedule matches, in loc's timezone. Seconds and
// smaller are truncated away since cron granularity is one minute.
func (s *schedule) Next(after time.Time, loc *time.Location) (time.Time, error) {
	if s.every > 0 {
		return after.Add(s.every), nil
	}

	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	deadline := after.Add(maxLookahead)
	for t.Before(deadline) {
		if s.month(int(t.Month())) && s.dom(t.Day()) && dowMatches(s.dow, t.Weekday()) &&
			s.hour(t.Hour()) && s.minute(t.Minute()) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching run time found within %s", maxLookahead)
}

// dowMatches treats cron day-of-week 0 and 7 as equivalent (both
// Sunday), matching standard cron semantics.
func dowMatches(m fieldMatcher, wd time.Weekday) bool {
	d := int(wd)
	if m(d) {
		return true
	}
	if d == 0 {
		return m(7)
	}
	return false
}
