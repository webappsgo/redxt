package backup

import (
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

func TestFilenameFormats(t *testing.T) {
	stamp := time.Date(2026, time.March, 4, 13, 5, 9, 0, time.UTC)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"full unencrypted", FullFilename("redxt", stamp, false), "redxt_backup_2026-03-04.tar.gz"},
		{"full encrypted", FullFilename("redxt", stamp, true), "redxt_backup_2026-03-04.tar.gz.enc"},
		{"manual unencrypted", ManualFilename("redxt", stamp, false), "redxt_backup_2026-03-04_130509.tar.gz"},
		{"manual encrypted", ManualFilename("redxt", stamp, true), "redxt_backup_2026-03-04_130509.tar.gz.enc"},
		{"daily incremental", DailyIncrementalFilename("redxt", false), "redxt-daily.tar.gz"},
		{"daily incremental encrypted", DailyIncrementalFilename("redxt", true), "redxt-daily.tar.gz.enc"},
		{"hourly incremental", HourlyIncrementalFilename("redxt", false), "redxt-hourly.tar.gz"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("got %q, want %q", c.got, c.want)
			}
		})
	}
}

func TestClassifyFile(t *testing.T) {
	cases := []struct {
		name      string
		filename  string
		wantClass class
		wantOK    bool
	}{
		{"yearly", "redxt_backup_2026-01-01.tar.gz", classYearly, true},
		{"monthly", "redxt_backup_2026-03-01.tar.gz", classMonthly, true},
		{"weekly sunday", "redxt_backup_2026-03-08.tar.gz", classWeekly, true},
		{"daily", "redxt_backup_2026-03-04.tar.gz", classDaily, true},
		{"daily incremental", "redxt-daily.tar.gz", classIncremental, true},
		{"hourly incremental", "redxt-hourly.tar.gz.enc", classIncremental, true},
		{"foreign file", "unrelated.txt", 0, false},
		{"other project", "otherapp_backup_2026-03-04.tar.gz", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, ok := ClassifyFile("redxt", c.filename, 100, time.Now())
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if f.Class != c.wantClass {
				t.Errorf("class = %v, want %v", f.Class, c.wantClass)
			}
		})
	}
}

func TestIsFalsey(t *testing.T) {
	falsey := []string{"", "0", "false", "No", " NONE ", "Disable", "disabled", "OFF"}
	for _, v := range falsey {
		if !isFalsey(v) {
			t.Errorf("isFalsey(%q) = false, want true", v)
		}
	}
	truthy := []string{"10%", "50G", "1"}
	for _, v := range truthy {
		if isFalsey(v) {
			t.Errorf("isFalsey(%q) = true, want false", v)
		}
	}
}

func TestParseSizeCap(t *testing.T) {
	cap10pct, enabled, err := ParseSizeCap("10%", 1000)
	if err != nil {
		t.Fatalf("ParseSizeCap percent: %v", err)
	}
	if !enabled || cap10pct != 100 {
		t.Errorf("cap = %d enabled = %v, want 100 true", cap10pct, enabled)
	}

	capAbs, enabled, err := ParseSizeCap("50MB", 0)
	if err != nil {
		t.Fatalf("ParseSizeCap absolute: %v", err)
	}
	if !enabled || capAbs != 50_000_000 {
		t.Errorf("cap = %d enabled = %v, want 50000000 true", capAbs, enabled)
	}

	_, enabled, err = ParseSizeCap("off", 1000)
	if err != nil {
		t.Fatalf("ParseSizeCap falsey: %v", err)
	}
	if enabled {
		t.Errorf("expected disabled for falsey value")
	}
}

// TestSelectForDeletionAllTiers builds a synthetic set of backup files
// spanning every retention tier plus a size cap, verifying the priority
// order (yearly > monthly > weekly > daily), the keep-newest-N-per-tier
// rule, and that the size cap prunes oldest-first across the survivors.
func TestSelectForDeletionAllTiers(t *testing.T) {
	mk := func(name string, date time.Time, size int64, cls class) File {
		return File{Name: name, Date: date, Size: size, Class: cls}
	}

	files := []File{
		// Two yearly candidates, keep 1.
		mk("y2025", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), 10, classYearly),
		mk("y2024", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), 10, classYearly),
		// Two monthly candidates, keep 1.
		mk("m-feb", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 10, classMonthly),
		mk("m-jan", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 10, classMonthly),
		// Two weekly candidates, keep 1.
		mk("w-mar8", time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), 10, classWeekly),
		mk("w-mar1", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 10, classWeekly),
		// Three daily candidates, keep 2 (maxBackups=2).
		mk("d-mar4", time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), 10, classDaily),
		mk("d-mar3", time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), 10, classDaily),
		mk("d-mar2", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), 10, classDaily),
		// Incremental files are never selected by this algorithm.
		mk("redxt-daily", time.Now(), 10, classIncremental),
	}

	r := config.BackupRetention{MaxBackups: 2, KeepWeekly: 1, KeepMonthly: 1, KeepYearly: 1}
	deleted := SelectForDeletion(files, r, 0, false)

	deletedNames := map[string]bool{}
	for _, f := range deleted {
		deletedNames[f.Name] = true
	}

	wantDeleted := []string{"y2024", "m-jan", "w-mar1", "d-mar2"}
	for _, name := range wantDeleted {
		if !deletedNames[name] {
			t.Errorf("expected %s to be deleted, deleted set = %v", name, deletedNames)
		}
	}
	wantKept := []string{"y2025", "m-feb", "w-mar8", "d-mar4", "d-mar3", "redxt-daily"}
	for _, name := range wantKept {
		if deletedNames[name] {
			t.Errorf("expected %s to be kept, deleted set = %v", name, deletedNames)
		}
	}

	// Now apply a tight size cap on top: 6 survivors * 10 bytes = 60, cap
	// at 25 forces additional oldest-first deletion among the survivors.
	deletedWithCap := SelectForDeletion(files, r, 25, true)
	if len(deletedWithCap) <= len(deleted) {
		t.Fatalf("expected size cap to delete more files than the tier-only pass: %d vs %d", len(deletedWithCap), len(deleted))
	}

	deletedWithCapNames := map[string]bool{}
	for _, f := range deletedWithCap {
		deletedWithCapNames[f.Name] = true
	}

	var keptTotal int64
	for _, name := range wantKept {
		if name == "redxt-daily" || deletedWithCapNames[name] {
			continue
		}
		for _, f := range files {
			if f.Name == name {
				keptTotal += f.Size
			}
		}
	}
	if keptTotal > 25 {
		t.Errorf("kept non-incremental total = %d, want <= 25", keptTotal)
	}
}
