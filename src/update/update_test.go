package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// release builds a minimal Release for table-driven tests.
func release(tag string, prerelease bool, publishedAt time.Time) Release {
	return Release{TagName: tag, Prerelease: prerelease, PublishedAt: publishedAt}
}

func TestMatchesBranch(t *testing.T) {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		r      Release
		branch string
		want   bool
	}{
		{name: "stable release on stable channel", r: release("v1.0.0", false, base), branch: "stable", want: true},
		{name: "beta release on stable channel", r: release("202512051430-beta", true, base), branch: "stable", want: false},
		{name: "daily release on stable channel", r: release("daily", true, base), branch: "stable", want: false},
		{name: "stable release on beta channel", r: release("v1.0.0", false, base), branch: "beta", want: true},
		{name: "beta release on beta channel", r: release("202512051430-beta", true, base), branch: "beta", want: true},
		{name: "daily release on beta channel", r: release("daily", true, base), branch: "beta", want: false},
		{name: "stable release on daily channel", r: release("v1.0.0", false, base), branch: "daily", want: true},
		{name: "beta release on daily channel", r: release("202512051430-beta", true, base), branch: "daily", want: true},
		{name: "daily release on daily channel", r: release("daily", true, base), branch: "daily", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesBranch(tt.r, tt.branch); got != tt.want {
				t.Fatalf("matchesBranch(%q, %q) = %v, want %v", tt.r.TagName, tt.branch, got, tt.want)
			}
		})
	}
}

func TestSelectLatest(t *testing.T) {
	t1 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		releases       []Release
		currentVersion string
		branch         string
		buildEpoch     int64
		wantTag        string
	}{
		{
			name: "picks newer stable over current",
			releases: []Release{
				release("v1.0.0", false, t1),
				release("v1.1.0", false, t2),
			},
			currentVersion: "v1.0.0", branch: "stable", wantTag: "v1.1.0",
		},
		{
			name: "already current returns nil",
			releases: []Release{
				release("v1.0.0", false, t1),
			},
			currentVersion: "v1.0.0", branch: "stable", wantTag: "",
		},
		{
			name: "beta never stuck behind stable",
			releases: []Release{
				release("v1.0.0", false, t1),
				release("v1.1.0-beta", true, t2),
			},
			currentVersion: "v1.0.0", branch: "beta", wantTag: "v1.1.0-beta",
		},
		{
			name: "cumulative daily picks newest of all three",
			releases: []Release{
				release("v1.0.0", false, t1),
				release("v1.1.0-beta", true, t2),
				release("daily", true, t3),
			},
			currentVersion: "v1.0.0", branch: "daily", buildEpoch: t1.Unix(), wantTag: "daily",
		},
		{
			name: "daily not newer than build epoch returns nil",
			releases: []Release{
				release("daily", true, t2),
			},
			currentVersion: "v1.0.0", branch: "daily", buildEpoch: t3.Unix(), wantTag: "",
		},
		{
			name: "no matching release on branch returns nil",
			releases: []Release{
				release("v1.1.0-beta", true, t2),
			},
			currentVersion: "v1.0.0", branch: "stable", wantTag: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectLatest(tt.releases, tt.currentVersion, tt.branch, tt.buildEpoch)
			gotTag := ""
			if got != nil {
				gotTag = got.TagName
			}
			if gotTag != tt.wantTag {
				t.Fatalf("selectLatest() tag = %q, want %q", gotTag, tt.wantTag)
			}
		})
	}
}

func TestFilterEligible(t *testing.T) {
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		publishedAt time.Time
		deferDays   int
		wantCount   int
	}{
		{name: "defer_days zero is immediately eligible", publishedAt: now, deferDays: 0, wantCount: 1},
		{name: "just before threshold is not eligible", publishedAt: now.Add(-30*24*time.Hour + time.Minute), deferDays: 30, wantCount: 0},
		{name: "exactly at threshold is eligible", publishedAt: now.Add(-30 * 24 * time.Hour), deferDays: 30, wantCount: 1},
		{name: "just after threshold is eligible", publishedAt: now.Add(-30*24*time.Hour - time.Minute), deferDays: 30, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			releases := []Release{release("v1.0.0", false, tt.publishedAt)}
			got := filterEligible(releases, tt.deferDays, now)
			if len(got) != tt.wantCount {
				t.Fatalf("filterEligible() len = %d, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// ghHandler serves a fixed JSON body for /repos/{owner}/{repo}/releases,
// or a fixed status code when body is nil.
func ghHandler(t *testing.T, status int, releases []Release) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if releases == nil {
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(releases); err != nil {
			t.Fatalf("encode releases: %v", err)
		}
	}
}

func TestCheckLatestNotFoundIsNoUpdate(t *testing.T) {
	srv := httptest.NewServer(ghHandler(t, http.StatusNotFound, nil))
	defer srv.Close()

	c := &Client{APIBase: srv.URL}
	release, err := c.CheckLatest(context.Background(), "v1.0.0", "stable")
	if err != nil {
		t.Fatalf("CheckLatest() error = %v, want nil", err)
	}
	if release != nil {
		t.Fatalf("CheckLatest() = %+v, want nil (no update available)", release)
	}
}

func TestCheckLatestServerErrorIsError(t *testing.T) {
	srv := httptest.NewServer(ghHandler(t, http.StatusInternalServerError, nil))
	defer srv.Close()

	c := &Client{APIBase: srv.URL}
	if _, err := c.CheckLatest(context.Background(), "v1.0.0", "stable"); err == nil {
		t.Fatal("CheckLatest() error = nil, want non-nil for a 500 response")
	}
}

func TestCheckLatestIgnoresDefer(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		release("v1.0.0", false, now.Add(-48*time.Hour)),
		release("v1.1.0", false, now),
	}
	srv := httptest.NewServer(ghHandler(t, http.StatusOK, releases))
	defer srv.Close()

	c := &Client{APIBase: srv.URL}
	got, err := c.CheckLatest(context.Background(), "v1.0.0", "stable")
	if err != nil {
		t.Fatalf("CheckLatest() error = %v", err)
	}
	if got == nil || got.TagName != "v1.1.0" {
		t.Fatalf("CheckLatest() = %+v, want v1.1.0 (manual checks ignore defer_days)", got)
	}
}

func TestCheckEligibleAppliesDefer(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		release("v1.0.0", false, now.Add(-60*24*time.Hour)),
		release("v1.1.0", false, now.Add(-5*24*time.Hour)),
	}
	srv := httptest.NewServer(ghHandler(t, http.StatusOK, releases))
	defer srv.Close()

	c := &Client{APIBase: srv.URL}
	got, err := c.CheckEligible(context.Background(), "v1.0.0", "stable", 30, now)
	if err != nil {
		t.Fatalf("CheckEligible() error = %v", err)
	}
	if got != nil {
		t.Fatalf("CheckEligible() = %+v, want nil (v1.1.0 has not aged past defer_days yet)", got)
	}
}

func TestParseChecksum(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		assetName string
		wantHash  string
		wantErr   bool
	}{
		{
			name:      "matching line",
			body:      "abc123  redxt-linux-amd64\ndef456  redxt-darwin-amd64\n",
			assetName: "redxt-linux-amd64",
			wantHash:  "abc123",
		},
		{
			name:      "no matching line",
			body:      "abc123  redxt-linux-amd64\n",
			assetName: "redxt-windows-amd64.exe",
			wantErr:   true,
		},
		{
			name:      "malformed line skipped",
			body:      "not-a-valid-line\nabc123  redxt-linux-amd64\n",
			assetName: "redxt-linux-amd64",
			wantHash:  "abc123",
		},
		{
			name:      "empty body",
			body:      "",
			assetName: "redxt-linux-amd64",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChecksum([]byte(tt.body), tt.assetName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseChecksum() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseChecksum() error = %v", err)
			}
			if got != tt.wantHash {
				t.Fatalf("parseChecksum() = %q, want %q", got, tt.wantHash)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	writeFile(t, path, []byte("hello world"))
	// sha256("hello world")
	const wantHash = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	if err := verifyChecksum(path, wantHash); err != nil {
		t.Fatalf("verifyChecksum() with correct hash: %v", err)
	}
	if err := verifyChecksum(path, "0000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("verifyChecksum() with wrong hash: want error, got nil")
	}
}

func TestDoUpdateRefusesWithoutChecksumAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("new binary bytes"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	currentPath := filepath.Join(dir, "redxt")
	writeFile(t, currentPath, []byte("old binary bytes"))

	r := &Release{
		TagName: "v1.1.0",
		Assets: []Asset{
			{Name: getBinaryName(), BrowserDownloadURL: srv.URL},
		},
	}

	c := &Client{}
	err := c.DoUpdate(context.Background(), r, currentPath)
	if err == nil {
		t.Fatal("DoUpdate() error = nil, want a refusal error when sha256.txt is missing")
	}
}

func TestDoUpdateRejectsChecksumMismatch(t *testing.T) {
	binaryBody := []byte("new binary bytes")
	mux := http.NewServeMux()
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryBody)
	})
	mux.HandleFunc("/sha256.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000  " + getBinaryName() + "\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	currentPath := filepath.Join(dir, "redxt")
	writeFile(t, currentPath, []byte("old binary bytes"))

	r := &Release{
		TagName: "v1.1.0",
		Assets: []Asset{
			{Name: getBinaryName(), BrowserDownloadURL: srv.URL + "/binary"},
			{Name: "sha256.txt", BrowserDownloadURL: srv.URL + "/sha256.txt"},
		},
	}

	c := &Client{}
	if err := c.DoUpdate(context.Background(), r, currentPath); err == nil {
		t.Fatal("DoUpdate() error = nil, want checksum mismatch error")
	}
	got, err := readFile(t, currentPath)
	if err != nil {
		t.Fatalf("read current binary after failed update: %v", err)
	}
	if string(got) != "old binary bytes" {
		t.Fatalf("current binary = %q, want untouched original after a rejected update", got)
	}
}

func TestSetBranch(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		want    string
		wantErr bool
	}{
		{name: "stable", branch: "stable", want: "stable"},
		{name: "beta", branch: "beta", want: "beta"},
		{name: "daily", branch: "daily", want: "daily"},
		{name: "case insensitive", branch: "STABLE", want: "stable"},
		{name: "invalid branch", branch: "nightly", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			c := config.DefaultConfig()
			c.SetPath(filepath.Join(dir, "server.yml"))

			err := SetBranch(c, tt.branch)
			if tt.wantErr {
				if err == nil {
					t.Fatal("SetBranch() error = nil, want non-nil for an invalid branch")
				}
				return
			}
			if err != nil {
				t.Fatalf("SetBranch() error = %v", err)
			}
			if c.Server.Update.Branch != tt.want {
				t.Fatalf("Server.Update.Branch = %q, want %q", c.Server.Update.Branch, tt.want)
			}
		})
	}
}

func TestCheckForUpdateNotifyOnlyWhenAutoInstallOff(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		release("v1.0.0", false, now.Add(-48*time.Hour)),
		release("v1.1.0", false, now),
	}
	srv := httptest.NewServer(ghHandler(t, http.StatusOK, releases))
	defer srv.Close()

	c := &Client{APIBase: srv.URL}
	var notified *Release
	err := c.CheckForUpdate(context.Background(), "v1.0.0", "stable", 0, false, "", "", func(r *Release) {
		notified = r
	}, nil)
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	if notified == nil || notified.TagName != "v1.1.0" {
		t.Fatalf("notify callback got %+v, want v1.1.0", notified)
	}
}

func TestCheckForUpdateNoEligibleReleaseSkipsNotify(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		release("v1.0.0", false, now.Add(-60*24*time.Hour)),
		release("v1.1.0", false, now.Add(-5*24*time.Hour)),
	}
	srv := httptest.NewServer(ghHandler(t, http.StatusOK, releases))
	defer srv.Close()

	c := &Client{APIBase: srv.URL}
	called := false
	err := c.CheckForUpdate(context.Background(), "v1.0.0", "stable", 30, false, "", "", func(r *Release) {
		called = true
	}, nil)
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	if called {
		t.Fatal("notify callback was called, want no call while defer_days is not satisfied")
	}
}
