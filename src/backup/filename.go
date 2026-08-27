package backup

import (
	"regexp"
	"time"
)

// dateLayout is the YYYY-MM-DD stamp used in full/manual filenames.
const dateLayout = "2006-01-02"

// timeLayout is the HHMMSS stamp appended to manual filenames.
const timeLayout = "150405"

// ext returns the ".tar.gz" or ".tar.gz.enc" suffix per AI.md PART 22.
func ext(encrypted bool) string {
	if encrypted {
		return ".tar.gz.enc"
	}
	return ".tar.gz"
}

// FullFilename returns "{project}_backup_YYYY-MM-DD.tar.gz[.enc]", the
// backup_daily task's full-backup name.
func FullFilename(project string, t time.Time, encrypted bool) string {
	return project + "_backup_" + t.Format(dateLayout) + ext(encrypted)
}

// ManualFilename returns "{project}_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]",
// the timestamped name a `--maintenance backup` with no explicit filename
// uses.
func ManualFilename(project string, t time.Time, encrypted bool) string {
	return project + "_backup_" + t.Format(dateLayout) + "_" + t.Format(timeLayout) + ext(encrypted)
}

// DailyIncrementalFilename returns "{project}-daily.tar.gz[.enc]", which is
// always replaced in place — there is never more than one on disk.
func DailyIncrementalFilename(project string, encrypted bool) string {
	return project + "-daily" + ext(encrypted)
}

// HourlyIncrementalFilename returns "{project}-hourly.tar.gz[.enc]", which
// is always replaced in place — there is never more than one on disk.
func HourlyIncrementalFilename(project string, encrypted bool) string {
	return project + "-hourly" + ext(encrypted)
}

// class is a retention tier a backup file has been assigned to. Order below
// matches the priority in AI.md PART 22 "Retention Priority Order".
type class int

const (
	classDaily class = iota
	classWeekly
	classMonthly
	classYearly
	// classIncremental marks the daily/hourly incrementals, which the
	// count-based retention algorithm never selects — they are replaced by
	// the next create, not pruned by the retention sweep.
	classIncremental
)

// File describes one file on disk in the backup directory, classified for
// retention purposes.
type File struct {
	Name  string
	Path  string
	Date  time.Time
	Size  int64
	Class class
}

// filenamePatterns compiles the four naming shapes AI.md PART 22 "Backup
// Cleanup Logic" lists, anchored to a specific project name so a file from
// an unrelated app is never matched.
type filenamePatterns struct {
	full        *regexp.Regexp
	manual      *regexp.Regexp
	dailyIncr   *regexp.Regexp
	hourlyIncr  *regexp.Regexp
	genericFull *regexp.Regexp
}

func newFilenamePatterns(project string) filenamePatterns {
	q := regexp.QuoteMeta(project)
	return filenamePatterns{
		full:        regexp.MustCompile(`^` + q + `_backup_(\d{4}-\d{2}-\d{2})\.tar\.gz(\.enc)?$`),
		manual:      regexp.MustCompile(`^` + q + `_backup_(\d{4}-\d{2}-\d{2})_(\d{6})\.tar\.gz(\.enc)?$`),
		dailyIncr:   regexp.MustCompile(`^` + q + `-daily\.tar\.gz(\.enc)?$`),
		hourlyIncr:  regexp.MustCompile(`^` + q + `-hourly\.tar\.gz(\.enc)?$`),
		genericFull: regexp.MustCompile(`^` + q + `(_backup_|-)\S*\.tar\.gz(\.enc)?$`),
	}
}

// ClassifyFile parses name against the app's naming conventions and returns
// the File it describes, or ok=false if name does not belong to this app at
// all (a foreign file in the backup directory is never touched).
//
// modTime is used as the effective date for any file the app's naming
// recognizes as a backup but cannot extract a date from — the "not
// otherwise classified" fallback in AI.md PART 22.
func ClassifyFile(project, name string, size int64, modTime time.Time) (File, bool) {
	p := newFilenamePatterns(project)

	if m := p.full.FindStringSubmatch(name); m != nil {
		d, err := time.Parse(dateLayout, m[1])
		if err != nil {
			d = modTime
		}
		return File{Name: name, Date: d, Size: size, Class: classify(d)}, true
	}
	if m := p.manual.FindStringSubmatch(name); m != nil {
		d, err := time.Parse(dateLayout+"_"+timeLayout, m[1]+"_"+m[2])
		if err != nil {
			d = modTime
		}
		return File{Name: name, Date: d, Size: size, Class: classify(d)}, true
	}
	if p.dailyIncr.MatchString(name) || p.hourlyIncr.MatchString(name) {
		return File{Name: name, Date: modTime, Size: size, Class: classIncremental}, true
	}
	if p.genericFull.MatchString(name) {
		return File{Name: name, Date: modTime, Size: size, Class: classify(modTime)}, true
	}
	return File{}, false
}

// classify assigns a date to the highest-priority tier it structurally
// qualifies for: yearly (Jan 1st) > monthly (1st of month) > weekly
// (Sunday) > daily, exactly as AI.md PART 22 "Retention Priority Order"
// specifies. A file is claimed by exactly one tier.
func classify(d time.Time) class {
	switch {
	case d.Month() == time.January && d.Day() == 1:
		return classYearly
	case d.Day() == 1:
		return classMonthly
	case d.Weekday() == time.Sunday:
		return classWeekly
	default:
		return classDaily
	}
}

// isFalsey reports whether s is one of the retention "all disabled" values
// from AI.md PART 22: 0, false, no, none, disable, disabled, off (case and
// surrounding whitespace insensitive), or empty.
func isFalsey(s string) bool {
	switch normalizeFalsey(s) {
	case "", "0", "false", "no", "none", "disable", "disabled", "off":
		return true
	default:
		return false
	}
}

func normalizeFalsey(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
