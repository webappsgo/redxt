package logging

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseRotate(t *testing.T) {
	tests := []struct {
		name       string
		policy     string
		wantPeriod period
		wantSize   int64
	}{
		{name: "never", policy: "never", wantPeriod: periodNone, wantSize: 0},
		{name: "daily", policy: "daily", wantPeriod: periodDaily},
		{name: "weekly", policy: "weekly", wantPeriod: periodWeekly},
		{name: "monthly", policy: "monthly", wantPeriod: periodMonthly},
		{name: "yearly", policy: "yearly", wantPeriod: periodYearly},
		{name: "size only", policy: "50MB", wantPeriod: periodNone, wantSize: 50 * 1024 * 1024},
		{name: "combined", policy: "weekly,50MB", wantPeriod: periodWeekly, wantSize: 50 * 1024 * 1024},
		{name: "unknown never rotates", policy: "sometimes", wantPeriod: periodNone},
		{name: "empty never rotates", policy: "", wantPeriod: periodNone},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRotate(tc.policy)
			if got.period != tc.wantPeriod {
				t.Errorf("period = %v, want %v", got.period, tc.wantPeriod)
			}
			if got.maxSize != tc.wantSize {
				t.Errorf("maxSize = %d, want %d", got.maxSize, tc.wantSize)
			}
		})
	}
}

func TestShouldRotateTime(t *testing.T) {
	const (
		daily   = 24 * time.Hour
		weekly  = 7 * 24 * time.Hour
		monthly = 30 * 24 * time.Hour
		yearly  = 365 * 24 * time.Hour
	)

	tests := []struct {
		name     string
		opened   time.Time
		now      time.Time
		interval time.Duration
		want     bool
	}{
		{
			name:     "never rotates without an interval",
			opened:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			interval: 0,
			want:     false,
		},
		{
			name:     "daily within the same day",
			opened:   time.Date(2025, 1, 15, 0, 0, 1, 0, time.UTC),
			now:      time.Date(2025, 1, 15, 23, 59, 59, 0, time.UTC),
			interval: daily,
			want:     false,
		},
		{
			name:     "daily across midnight after one minute",
			opened:   time.Date(2025, 1, 15, 23, 59, 0, 0, time.UTC),
			now:      time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
			interval: daily,
			want:     true,
		},
		{
			name:     "weekly within the same iso week",
			opened:   time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 1, 19, 23, 0, 0, 0, time.UTC),
			interval: weekly,
			want:     false,
		},
		{
			name:     "weekly across the iso week boundary",
			opened:   time.Date(2025, 1, 19, 23, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
			interval: weekly,
			want:     true,
		},
		{
			name:     "monthly within the same month",
			opened:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 1, 31, 23, 0, 0, 0, time.UTC),
			interval: monthly,
			want:     false,
		},
		{
			name:     "monthly across the month boundary",
			opened:   time.Date(2025, 1, 31, 23, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			interval: monthly,
			want:     true,
		},
		{
			name:     "monthly does not rotate at 30 elapsed days inside one month",
			opened:   time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 3, 31, 12, 0, 0, 0, time.UTC),
			interval: monthly,
			want:     false,
		},
		{
			name:     "yearly across new year",
			opened:   time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			interval: yearly,
			want:     true,
		},
		{
			name:     "clock moving backwards never rotates",
			opened:   time.Date(2025, 1, 16, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			interval: daily,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRotateTime(tc.opened, tc.now, tc.interval); got != tc.want {
				t.Errorf("shouldRotateTime() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPruneList(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	files := []rotatedFile{
		{Name: "a.20250101-000000.log", Path: "a1", Stamp: now.AddDate(0, 0, -151)},
		{Name: "b.20250515-000000.log", Path: "b", Stamp: now.AddDate(0, 0, -17)},
		{Name: "c.20250530-000000.log", Path: "c", Stamp: now.AddDate(0, 0, -2)},
		{Name: "d.20250531-000000.log", Path: "d", Stamp: now.AddDate(0, 0, -1)},
	}

	tests := []struct {
		name       string
		keep       string
		wantDelete []string
	}{
		{name: "none deletes everything", keep: "none", wantDelete: []string{"d", "c", "b", "a1"}},
		{name: "empty policy behaves like none", keep: "", wantDelete: []string{"d", "c", "b", "a1"}},
		{name: "forever deletes nothing", keep: "forever"},
		{name: "count keeps the newest", keep: "2", wantDelete: []string{"b", "a1"}},
		{name: "count larger than the set keeps everything", keep: "10"},
		{name: "zero count deletes everything", keep: "0", wantDelete: []string{"d", "c", "b", "a1"}},
		{name: "days", keep: "7d", wantDelete: []string{"b", "a1"}},
		{name: "weeks", keep: "3w", wantDelete: []string{"a1"}},
		{name: "months", keep: "1m", wantDelete: []string{"a1"}},
		{name: "unparseable policy deletes nothing", keep: "sometimes"},
		{name: "case is ignored", keep: "NONE", wantDelete: []string{"d", "c", "b", "a1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pruneList(files, tc.keep, now)
			if len(got) != len(tc.wantDelete) {
				t.Fatalf("pruneList() deleted %d files (%v), want %d (%v)", len(got), names(got), len(tc.wantDelete), tc.wantDelete)
			}
			for i, f := range got {
				if f.Path != tc.wantDelete[i] {
					t.Errorf("pruneList()[%d] = %s, want %s", i, f.Path, tc.wantDelete[i])
				}
			}
		})
	}
}

// names extracts the paths of a rotated file list for test output.
func names(files []rotatedFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestParseStamp(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantOK  bool
		wantUTC string
	}{
		{name: "plain rotated file", file: "access.20250115-010203.log", wantOK: true, wantUTC: "2025-01-15T01:02:03Z"},
		{name: "compressed rotated file", file: "access.20250115-010203.log.gz", wantOK: true, wantUTC: "2025-01-15T01:02:03Z"},
		{name: "same second collision is stamped one second later", file: "access.20250115-010204.log", wantOK: true, wantUTC: "2025-01-15T01:02:04Z"},
		{name: "unrelated file", file: "access.notes.log", wantOK: false},
		{name: "too short", file: "access.2025.log", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stamp, ok := parseStamp(tc.file, "access.", ".log")
			if ok != tc.wantOK {
				t.Fatalf("parseStamp(%q) ok = %v, want %v", tc.file, ok, tc.wantOK)
			}
			if ok && stamp.UTC().Format(time.RFC3339) != tc.wantUTC {
				t.Errorf("parseStamp(%q) = %s, want %s", tc.file, stamp.UTC().Format(time.RFC3339), tc.wantUTC)
			}
		})
	}
}

func TestFileRotatesOnSize(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir, "test.log", "64", "forever")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	line := strings.Repeat("x", 32) + "\n"
	for i := 0; i < 4; i++ {
		if _, err := f.Write([]byte(line)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	rotated, err := f.rotatedFiles()
	if err != nil {
		t.Fatalf("rotatedFiles() error = %v", err)
	}
	if len(rotated) == 0 {
		t.Fatalf("expected at least one rotated file, found none")
	}
	if _, err := os.Stat(filepath.Join(dir, "test.log")); err != nil {
		t.Fatalf("live log missing after rotation: %v", err)
	}
}

func TestFileRotateAppliesRetention(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir, "test.log", "never", "1")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	for i := 0; i < 3; i++ {
		if _, err := f.Write([]byte("entry\n")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := f.Rotate(); err != nil {
			t.Fatalf("Rotate() error = %v", err)
		}
	}

	rotated, err := f.rotatedFiles()
	if err != nil {
		t.Fatalf("rotatedFiles() error = %v", err)
	}
	if len(rotated) != 1 {
		t.Fatalf("retention kept %d rotated files, want 1", len(rotated))
	}
}

func TestFileRotateNoneDeletesEverything(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir, "test.log", "never", "none")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write([]byte("entry\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := f.Rotate(); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	rotated, err := f.rotatedFiles()
	if err != nil {
		t.Fatalf("rotatedFiles() error = %v", err)
	}
	if len(rotated) != 0 {
		t.Fatalf("keep=none left %d rotated files, want 0", len(rotated))
	}
}

func TestFileRotateCompresses(t *testing.T) {
	dir := t.TempDir()
	f, err := OpenCompressed(dir, "audit.log", "never", "forever", true)
	if err != nil {
		t.Fatalf("OpenCompressed() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	payload := "audited\n"
	if _, err := f.Write([]byte(payload)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := f.Rotate(); err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	rotated, err := f.rotatedFiles()
	if err != nil {
		t.Fatalf("rotatedFiles() error = %v", err)
	}
	if len(rotated) != 1 {
		t.Fatalf("found %d rotated files, want 1", len(rotated))
	}
	if !strings.HasSuffix(rotated[0].Name, ".gz") {
		t.Fatalf("rotated file %q is not compressed", rotated[0].Name)
	}

	handle, err := os.Open(rotated[0].Path)
	if err != nil {
		t.Fatalf("open rotated file: %v", err)
	}
	defer func() {
		_ = handle.Close()
	}()
	zr, err := gzip.NewReader(handle)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	content, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	if string(content) != payload {
		t.Errorf("compressed content = %q, want %q", content, payload)
	}
}

func TestFilePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	f, err := Open(dir, "test.log", "never", "forever")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	info, err := os.Stat(f.Path())
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if info.Mode().Perm() != logFileMode {
		t.Errorf("log file mode = %v, want %v", info.Mode().Perm(), logFileMode)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat log dir: %v", err)
	}
	if dirInfo.Mode().Perm() != logDirMode {
		t.Errorf("log dir mode = %v, want %v", dirInfo.Mode().Perm(), logDirMode)
	}
}

func TestFileCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir, "test.log", "never", "forever")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := f.Write([]byte("late\n")); err != ErrClosed {
		t.Errorf("Write() after Close error = %v, want %v", err, ErrClosed)
	}
}

func TestFileConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	f, err := Open(dir, "test.log", "512", "forever")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	const writers = 8
	const perWriter = 20
	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				if _, err := f.Write([]byte("concurrent line\n")); err != nil {
					t.Errorf("Write() error = %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	total := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			if line != "concurrent line" {
				t.Fatalf("interleaved write produced %q", line)
			}
			total++
		}
	}
	if total != writers*perWriter {
		t.Errorf("wrote %d lines, want %d", total, writers*perWriter)
	}
}
