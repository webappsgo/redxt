package logging

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// stampLayout is the timestamp appended to a rotated file name. It is
// UTC and sorts lexicographically, which keeps directory listings in
// chronological order.
const stampLayout = "20060102-150405"

// period identifies the calendar boundary a time-based rotation policy
// rotates on.
type period int

// The calendar boundaries the PART 11 rotation options describe.
const (
	periodNone period = iota
	periodDaily
	periodWeekly
	periodMonthly
	periodYearly
)

// Retention units accepted by a "Nd", "Nw", or "Nm" keep policy.
const (
	day   = 24 * time.Hour
	week  = 7 * day
	month = 30 * day
)

// policy is a parsed rotation policy: a calendar boundary, a size
// ceiling, or both. A combined policy such as "weekly,50MB" rotates on
// whichever fires first.
type policy struct {
	// period is the calendar boundary to rotate on, periodNone when
	// the policy is size-only or "never".
	period period
	// interval is the nominal duration of period, kept so the pure
	// decision helper can be called with a duration.
	interval time.Duration
	// maxSize is the size ceiling in bytes, zero when the policy is
	// time-only or "never".
	maxSize int64
}

// parseRotate turns a configured rotation policy into its decision
// form. The time and size components are read with the config
// helpers, so this package and the config validator always agree on
// what a policy means. An unrecognized policy never rotates, matching
// "never".
func parseRotate(raw string) policy {
	var p policy
	if interval, ok := config.LogRotationInterval(raw); ok {
		p.interval = interval
		p.period = periodFor(interval)
	}
	if size, ok := config.LogRotationSize(raw); ok {
		p.maxSize = size
	}
	return p
}

// periodFor maps the nominal duration config reports for a named
// rotation period back onto the calendar boundary it stands for. The
// durations are the fixed constants config.LogRotationInterval
// returns, never arbitrary user input.
func periodFor(interval time.Duration) period {
	switch {
	case interval <= 0:
		return periodNone
	case interval < week:
		return periodDaily
	case interval < 30*day:
		return periodWeekly
	case interval < 365*day:
		return periodMonthly
	default:
		return periodYearly
	}
}

// shouldRotateTime reports whether a file opened at openedAt must be
// rotated at now under a time-based policy of the given interval.
//
// The comparison is by calendar period rather than by elapsed time: a
// daily policy rotates when the UTC date changes, not when 24 hours
// have passed since the file was opened, so a server restarted at
// 23:59 still rotates at midnight. An interval of zero never rotates.
func shouldRotateTime(openedAt, now time.Time, interval time.Duration) bool {
	p := periodFor(interval)
	if p == periodNone {
		return false
	}
	if now.Before(openedAt) {
		return false
	}
	return periodKey(p, openedAt) != periodKey(p, now)
}

// periodKey returns the identifier of the calendar period a time falls
// in. Two times in the same period share a key, so a differing key is
// exactly a crossed rotation boundary. All keys are computed in UTC,
// per the PART 11 "Daily rotation at midnight UTC" rule.
func periodKey(p period, t time.Time) string {
	t = t.UTC()
	switch p {
	case periodDaily:
		return t.Format("2006-01-02")
	case periodWeekly:
		year, week := t.ISOWeek()
		return strconv.Itoa(year) + "-W" + strconv.Itoa(week)
	case periodMonthly:
		return t.Format("2006-01")
	case periodYearly:
		return t.Format("2006")
	default:
		return ""
	}
}

// rotatedFile is one already-rotated log file found beside the live
// one.
type rotatedFile struct {
	// Name is the base file name.
	Name string
	// Path is the full path to the file.
	Path string
	// Stamp is the rotation time parsed out of the file name.
	Stamp time.Time
}

// pruneList returns the rotated files that the retention policy says
// to delete, given the current time.
//
// The policies are the PART 11 retention options: "none" (and an empty
// policy) deletes every rotated file immediately, "forever" deletes
// nothing, a bare integer N keeps the N newest files, and "Nd", "Nw",
// or "Nm" keep files younger than that age. A policy that parses as
// none of these deletes nothing, so an unexpected value can never
// destroy log history.
func pruneList(files []rotatedFile, keep string, now time.Time) []rotatedFile {
	keepPolicy := strings.ToLower(strings.TrimSpace(keep))

	sorted := make([]rotatedFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Stamp.Equal(sorted[j].Stamp) {
			return sorted[i].Name > sorted[j].Name
		}
		return sorted[i].Stamp.After(sorted[j].Stamp)
	})

	switch keepPolicy {
	case "", "none":
		return sorted
	case "forever":
		return nil
	}

	if unit, count, ok := parseKeepAge(keepPolicy); ok {
		cutoff := now.Add(-time.Duration(count) * unit)
		var out []rotatedFile
		for _, f := range sorted {
			if f.Stamp.Before(cutoff) {
				out = append(out, f)
			}
		}
		return out
	}

	if n, err := strconv.Atoi(keepPolicy); err == nil && n >= 0 {
		if n >= len(sorted) {
			return nil
		}
		return sorted[n:]
	}

	return nil
}

// parseKeepAge splits an age-based retention policy such as "30d" into
// its unit and count.
func parseKeepAge(policy string) (time.Duration, int, bool) {
	if len(policy) < 2 {
		return 0, 0, false
	}
	var unit time.Duration
	switch policy[len(policy)-1] {
	case 'd':
		unit = day
	case 'w':
		unit = week
	case 'm':
		unit = month
	default:
		return 0, 0, false
	}
	count, err := strconv.Atoi(policy[:len(policy)-1])
	if err != nil || count < 0 {
		return 0, 0, false
	}
	return unit, count, true
}
