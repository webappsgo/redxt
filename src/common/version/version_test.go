package version

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// reset restores the package defaults so subtests are independent.
func reset() {
	version = "dev"
	commit = "N/A"
	buildEpoch = "0"
	officialSite = "redxt.us"
}

func TestSet(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		commit       string
		epoch        string
		site         string
		wantVersion  string
		wantCommit   string
		wantEpoch    int64
		wantSiteName string
	}{
		{
			name:         "all values set",
			version:      "1.2.3",
			commit:       "abc1234",
			epoch:        "1765112713",
			site:         "https://redxt.us",
			wantVersion:  "1.2.3",
			wantCommit:   "abc1234",
			wantEpoch:    1765112713,
			wantSiteName: "https://redxt.us",
		},
		{
			name:         "all empty keeps defaults",
			wantVersion:  "dev",
			wantCommit:   "N/A",
			wantEpoch:    0,
			wantSiteName: "redxt.us",
		},
		{
			name:         "partial values keep remaining defaults",
			version:      "2.0.0",
			epoch:        "42",
			wantVersion:  "2.0.0",
			wantCommit:   "N/A",
			wantEpoch:    42,
			wantSiteName: "redxt.us",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset()
			Set(tt.version, tt.commit, tt.epoch, tt.site)
			if got := Version(); got != tt.wantVersion {
				t.Errorf("Version() = %q, want %q", got, tt.wantVersion)
			}
			if got := Commit(); got != tt.wantCommit {
				t.Errorf("Commit() = %q, want %q", got, tt.wantCommit)
			}
			if got := BuildEpoch(); got != tt.wantEpoch {
				t.Errorf("BuildEpoch() = %d, want %d", got, tt.wantEpoch)
			}
			if got := OfficialSite(); got != tt.wantSiteName {
				t.Errorf("OfficialSite() = %q, want %q", got, tt.wantSiteName)
			}
		})
	}
	reset()
}

func TestBuildDate(t *testing.T) {
	tests := []struct {
		name  string
		epoch string
		want  string
	}{
		{name: "valid epoch", epoch: "1765112713", want: time.Unix(1765112713, 0).UTC().Format(time.RFC3339)},
		{name: "zero epoch", epoch: "0", want: "unknown"},
		{name: "garbage epoch", epoch: "not-a-number", want: "unknown"},
		{name: "negative epoch", epoch: "-1", want: time.Unix(-1, 0).UTC().Format(time.RFC3339)},
		{name: "unix zero day", epoch: "1", want: "1970-01-01T00:00:01Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset()
			Set("", "", tt.epoch, "")
			if got := BuildDate(); got != tt.want {
				t.Errorf("BuildDate() = %q, want %q", got, tt.want)
			}
		})
	}
	reset()
}

func TestAssetStamp(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{name: "defaults", want: "dev-N/A"},
		{name: "stamped", version: "1.0.0", commit: "deadbee", want: "1.0.0-deadbee"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset()
			Set(tt.version, tt.commit, "", "")
			if got := AssetStamp(); got != tt.want {
				t.Errorf("AssetStamp() = %q, want %q", got, tt.want)
			}
		})
	}
	reset()
}

func TestUserAgent(t *testing.T) {
	reset()
	Set("1.4.0", "", "", "")
	if got, want := UserAgent(), "redxt/1.4.0"; got != want {
		t.Errorf("UserAgent() = %q, want %q", got, want)
	}
	reset()
}

func TestString(t *testing.T) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	goVersion := runtime.Version()

	tests := []struct {
		name       string
		binaryName string
		version    string
		epoch      string
		want       string
	}{
		{
			name:       "unstamped build",
			binaryName: "redxt",
			version:    "",
			epoch:      "",
			want:       fmt.Sprintf("redxt dev\nBuilt: unknown\nGo: %s\nOS/Arch: %s\n", goVersion, platform),
		},
		{
			name:       "renamed binary",
			binaryName: "myserver",
			version:    "9.9.9",
			epoch:      "1",
			want: fmt.Sprintf("myserver 9.9.9\nBuilt: 1970-01-01T00:00:01Z\nGo: %s\nOS/Arch: %s\n",
				goVersion, platform),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset()
			Set(tt.version, "", tt.epoch, "")
			if got := String(tt.binaryName); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
	reset()
}

func TestPlatformAndGoVersion(t *testing.T) {
	if got, want := Platform(), runtime.GOOS+"/"+runtime.GOARCH; got != want {
		t.Errorf("Platform() = %q, want %q", got, want)
	}
	if got, want := GoVersion(), runtime.Version(); got != want {
		t.Errorf("GoVersion() = %q, want %q", got, want)
	}
}
