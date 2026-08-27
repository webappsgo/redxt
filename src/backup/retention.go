package backup

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/webappsgo/redxt/src/config"
)

// ParseSizeCap resolves server.backup.retention.max_total_size against
// volumeTotalBytes (the backup filesystem's total capacity, needed only for
// a "%" value). It returns capBytes=0, enabled=false for any of the AI.md
// PART 22 falsey values (0, false, no, none, disable, disabled, off).
func ParseSizeCap(raw string, volumeTotalBytes uint64) (capBytes uint64, enabled bool, err error) {
	trimmed := strings.TrimSpace(raw)
	if isFalsey(trimmed) {
		return 0, false, nil
	}
	if strings.HasSuffix(trimmed, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil || pct <= 0 {
			return 0, false, fmt.Errorf("backup: invalid max_total_size percent %q", raw)
		}
		return uint64(pct / 100 * float64(volumeTotalBytes)), true, nil
	}
	bytes, err := humanize.ParseBytes(trimmed)
	if err != nil {
		return 0, false, fmt.Errorf("backup: invalid max_total_size %q: %w", raw, err)
	}
	if bytes == 0 {
		return 0, false, nil
	}
	return bytes, true, nil
}

// ScanDir lists every File in dir that ClassifyFile recognizes as belonging
// to project. Files with a name the app never creates are ignored — never
// touched by retention, per AI.md PART 22.
func ScanDir(dir, project string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("backup: read backup dir: %w", err)
	}

	files := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("backup: stat %s: %w", e.Name(), err)
		}
		f, ok := ClassifyFile(project, e.Name(), info.Size(), info.ModTime())
		if !ok {
			continue
		}
		f.Path = dir + string(os.PathSeparator) + e.Name()
		files = append(files, f)
	}
	return files, nil
}

// SelectForDeletion implements AI.md PART 22's "Backup Cleanup Logic"
// exactly: each file is claimed by the highest-priority tier it
// structurally qualifies for (yearly > monthly > weekly > daily); within a
// tier, the newest r.KeepYearly/KeepMonthly/KeepWeekly/r.MaxBackups files
// are kept and the rest — oldest first — are marked for deletion.
// Incremental files (daily/hourly) are never selected here; they are
// replaced in place by the next create, not pruned by this sweep.
//
// After the count-based pass, if capBytes > 0 and the surviving set's total
// size still exceeds capBytes, additional files are deleted oldest-first
// (across all tiers — the size cap overrides every count limit) until the
// total is at or under the cap.
func SelectForDeletion(files []File, r config.BackupRetention, capBytes uint64, capEnabled bool) []File {
	var yearly, monthly, weekly, daily []File
	for _, f := range files {
		switch f.Class {
		case classYearly:
			yearly = append(yearly, f)
		case classMonthly:
			monthly = append(monthly, f)
		case classWeekly:
			weekly = append(weekly, f)
		case classDaily:
			daily = append(daily, f)
		}
		// classIncremental is intentionally excluded from every tier.
	}

	maxBackups := r.MaxBackups
	if maxBackups <= 0 {
		maxBackups = 1
	}

	var toDelete []File
	kept := make([]File, 0, len(files))
	kept = append(kept, tierPrune(yearly, r.KeepYearly, &toDelete)...)
	kept = append(kept, tierPrune(monthly, r.KeepMonthly, &toDelete)...)
	kept = append(kept, tierPrune(weekly, r.KeepWeekly, &toDelete)...)
	kept = append(kept, tierPrune(daily, maxBackups, &toDelete)...)

	if capEnabled && capBytes > 0 {
		toDelete = append(toDelete, sizeCapPrune(kept, capBytes)...)
	}

	return toDelete
}

// tierPrune sorts a tier's files newest-first, keeps up to keepN of them,
// and appends the rest to deleted. It returns the kept subset.
func tierPrune(tier []File, keepN int, deleted *[]File) []File {
	if keepN < 0 {
		keepN = 0
	}
	sorted := make([]File, len(tier))
	copy(sorted, tier)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.After(sorted[j].Date) })

	if keepN >= len(sorted) {
		return sorted
	}
	*deleted = append(*deleted, sorted[keepN:]...)
	return sorted[:keepN]
}

// sizeCapPrune deletes the oldest of the kept files, one at a time, until
// their total size is at or under capBytes. It returns the additional
// files it selected for deletion.
func sizeCapPrune(kept []File, capBytes uint64) []File {
	var total uint64
	for _, f := range kept {
		total += uint64(f.Size)
	}
	if total <= capBytes {
		return nil
	}

	sorted := make([]File, len(kept))
	copy(sorted, kept)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	var extra []File
	for _, f := range sorted {
		if total <= capBytes {
			break
		}
		extra = append(extra, f)
		total -= uint64(f.Size)
	}
	return extra
}
