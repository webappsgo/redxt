// Package format implements AI.md PART 8 "Human-Readable Values": the single
// set of helpers every user-facing surface (web pages, healthz HTML, admin
// panel, error pages, TUI/GUI, pretty console output) uses to render
// durations, sizes, counts and timestamps.
//
// JSON API responses, Prometheus metrics and log files keep raw base units
// (seconds, bytes) and must never call these helpers.
package format

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// durationUnit is one entry of the largest-to-smallest duration unit ladder.
type durationUnit struct {
	name    string
	seconds int64
}

// durationUnits is ordered largest first; years are 365 days and months are 30 days.
var durationUnits = []durationUnit{
	{name: "year", seconds: 365 * 24 * 60 * 60},
	{name: "month", seconds: 30 * 24 * 60 * 60},
	{name: "week", seconds: 7 * 24 * 60 * 60},
	{name: "day", seconds: 24 * 60 * 60},
	{name: "hour", seconds: 60 * 60},
	{name: "minute", seconds: 60},
	{name: "second", seconds: 1},
}

// sizeUnits is ordered smallest first, each step a 1024 multiple of the previous.
var sizeUnits = []string{"byte", "kilobyte", "megabyte", "gigabyte", "terabyte", "petabyte"}

// TimestampLayout is the display layout for timestamps, matching the
// `%B %d, %Y at %H:%M:%S %Z` format required by AI.md.
const TimestampLayout = "January 02, 2006 at 15:04:05 MST"

// Duration renders d in English words using the largest fitting unit and at
// most two units, e.g. "1 minute 30 seconds" or "2 days 5 hours". Negative
// durations are rendered from their absolute value with a "-" prefix, and
// anything under one second renders as "0 seconds".
func Duration(d time.Duration) string {
	total := int64(d / time.Second)
	negative := false
	if total < 0 {
		negative = true
		total = -total
	}
	if total == 0 {
		return "0 seconds"
	}

	var parts []string
	for i, unit := range durationUnits {
		value := total / unit.seconds
		if value == 0 {
			continue
		}
		parts = append(parts, pluralize(value, unit.name))
		if i+1 < len(durationUnits) {
			remainder := total % unit.seconds
			if next := remainder / durationUnits[i+1].seconds; next > 0 {
				parts = append(parts, pluralize(next, durationUnits[i+1].name))
			}
		}
		break
	}

	out := strings.Join(parts, " ")
	if negative {
		return "-" + out
	}
	return out
}

// Size renders a byte count with full unit names on 1024 boundaries, using at
// most one decimal place and dropping a trailing ".0".
func Size(bytes int64) string {
	if bytes < 0 {
		return "-" + Size(-bytes)
	}
	if bytes < 1024 {
		return pluralize(bytes, sizeUnits[0])
	}

	value := float64(bytes)
	index := 0
	for value >= 1024 && index < len(sizeUnits)-1 {
		value /= 1024
		index++
	}

	rounded := math.Round(value*10) / 10
	if rounded == math.Trunc(rounded) {
		return pluralize(int64(rounded), sizeUnits[index])
	}
	return strconv.FormatFloat(rounded, 'f', 1, 64) + " " + sizeUnits[index] + "s"
}

// Count renders n with "," thousands separators, e.g. "1,234,567".
func Count(n int64) string {
	digits := strconv.FormatInt(n, 10)
	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = digits[1:]
	}

	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if negative {
		return "-" + b.String()
	}
	return b.String()
}

// Timestamp renders t in the display layout required by AI.md.
func Timestamp(t time.Time) string {
	return t.Format(TimestampLayout)
}

// Uptime renders the elapsed time since start, used by the health endpoint.
func Uptime(start time.Time) string {
	return Duration(time.Since(start))
}

// pluralize joins a value with its unit name, appending "s" for any value other than one.
func pluralize(value int64, unit string) string {
	if value == 1 {
		return "1 " + unit
	}
	return strconv.FormatInt(value, 10) + " " + unit + "s"
}
