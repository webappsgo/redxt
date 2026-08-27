// Package update implements AI.md PART 23 "Update Command" — the
// self-update mechanism for the server binary: checking the GitHub
// Releases API on a configured channel, verifying the SHA-256
// checksum of a downloaded release asset, replacing the running
// binary, and restarting the service.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
	"github.com/webappsgo/redxt/src/retry"
)

// repoOwner and repoName are the fixed GitHub coordinates this build
// checks for updates, per IDEA.md {project_org}/{project_name}.
const (
	repoOwner      = "webappsgo"
	repoName       = "redxt"
	defaultAPIBase = "https://api.github.com"
)

// Logger is the narrow logging surface this package needs, matching
// the shape used by src/scheduler so callers can pass their existing
// logger without an adapter.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Release is one GitHub Releases API entry.
type Release struct {
	TagName     string    `json:"tag_name"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset is one downloadable file attached to a Release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Client queries the GitHub Releases API and performs the update flow.
// The zero value is usable; APIBase and HTTPClient exist so tests can
// point at an httptest.Server instead of the real network.
type Client struct {
	// APIBase overrides the GitHub API origin. Empty uses defaultAPIBase.
	APIBase string
	// HTTPClient overrides the client used for API and asset requests.
	// Nil uses http.DefaultClient.
	HTTPClient *http.Client
}

// NewClient returns a Client configured against the real GitHub API.
func NewClient() *Client {
	return &Client{}
}

func (c *Client) apiBase() string {
	if c.APIBase == "" {
		return defaultAPIBase
	}
	return c.APIBase
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// listReleases fetches every release from the GitHub Releases API. A 404
// response (no releases exist yet) returns (nil, nil), per AI.md PART 23
// "HTTP 404 from GitHub API means no updates available".
func (c *Client) listReleases(ctx context.Context) ([]Release, error) {
	url := c.apiBase() + "/repos/" + repoOwner + "/" + repoName + "/releases"

	var body []byte
	var status int
	err := retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("User-Agent", version.UserAgent())

		resp, err := c.httpClient().Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		status = resp.StatusCode
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return err
		}
		body = b
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update: list releases: %w", err)
	}

	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("update: GitHub API error: %d", status)
	}

	var releases []Release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("update: decode releases: %w", err)
	}
	return releases, nil
}

// matchesBranch implements the cumulative channel rule from AI.md PART 23
// "Channel Semantics": stable considers full releases only, beta adds
// pre-releases tagged "*-beta", daily adds the rolling "daily" tag too.
func matchesBranch(r Release, branch string) bool {
	isDaily := r.TagName == "daily"
	isBeta := strings.HasSuffix(r.TagName, "-beta")
	isStable := !r.Prerelease && !isDaily && !isBeta
	switch branch {
	case "beta":
		return isBeta || isStable
	case "daily":
		return isDaily || isBeta || isStable
	default:
		return isStable
	}
}

// selectLatest picks the newest release on branch that is actually newer
// than the running version, per AI.md PART 23's CheckForUpdate algorithm.
// The rolling "daily" tag never matches currentVersion, so its eligibility
// is decided by comparing its publish time against this binary's own build
// time instead of by tag/publish comparison against the current release.
func selectLatest(releases []Release, currentVersion, branch string, buildEpoch int64) *Release {
	var newest *Release
	var currentPublished time.Time
	haveCurrent := false

	for i := range releases {
		r := &releases[i]
		if r.TagName == currentVersion {
			currentPublished = r.PublishedAt
			haveCurrent = true
		}
		if matchesBranch(*r, branch) && (newest == nil || r.PublishedAt.After(newest.PublishedAt)) {
			newest = r
		}
	}
	if newest == nil {
		return nil
	}
	if newest.TagName == "daily" {
		if newest.PublishedAt.Unix() <= buildEpoch {
			return nil
		}
		return newest
	}
	if newest.TagName == currentVersion {
		return nil
	}
	if haveCurrent && !newest.PublishedAt.After(currentPublished) {
		return nil
	}
	return newest
}

// filterEligible drops releases that have not yet aged past deferDays as
// of now, per AI.md PART 23 "Defer Semantics (defer_days)". deferDays <= 0
// means every release is immediately eligible.
func filterEligible(releases []Release, deferDays int, now time.Time) []Release {
	if deferDays <= 0 {
		return releases
	}
	threshold := time.Duration(deferDays) * 24 * time.Hour
	eligible := make([]Release, 0, len(releases))
	for _, r := range releases {
		if now.UTC().Sub(r.PublishedAt.UTC()) >= threshold {
			eligible = append(eligible, r)
		}
	}
	return eligible
}

// CheckLatest returns the newest release on branch, ignoring defer_days.
// This is what a manual "--update check"/"--update yes" always sees, per
// AI.md PART 23 "an explicit operator action overrides the defer window".
// A nil Release with a nil error means the running version is current.
func (c *Client) CheckLatest(ctx context.Context, currentVersion, branch string) (*Release, error) {
	releases, err := c.listReleases(ctx)
	if err != nil {
		return nil, err
	}
	if releases == nil {
		return nil, nil
	}
	return selectLatest(releases, currentVersion, branch, version.BuildEpoch()), nil
}

// CheckEligible returns the newest release on branch that is both newer
// than currentVersion and has aged past deferDays as of now. This is what
// the scheduled update_check task sees, per AI.md PART 23 "Defer Semantics".
func (c *Client) CheckEligible(ctx context.Context, currentVersion, branch string, deferDays int, now time.Time) (*Release, error) {
	releases, err := c.listReleases(ctx)
	if err != nil {
		return nil, err
	}
	if releases == nil {
		return nil, nil
	}
	eligible := filterEligible(releases, deferDays, now)
	return selectLatest(eligible, currentVersion, branch, version.BuildEpoch()), nil
}

// getBinaryName returns the release asset name expected for this
// platform: "{project_name}-{GOOS}-{GOARCH}", ".exe" on Windows.
func getBinaryName() string {
	name := paths.ProjectName() + "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// fetchChecksum downloads a release's sha256.txt (standard "sha256sum"
// output: "{hash}  {filename}" per line) and returns the hex digest for
// the line matching assetName.
func (c *Client) fetchChecksum(ctx context.Context, url, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update: checksum download failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return parseChecksum(body, assetName)
}

// parseChecksum scans standard "sha256sum" output for the hex digest of
// assetName. A malformed line (wrong field count) is skipped, not fatal.
func parseChecksum(body []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("update: no checksum entry for %s in sha256.txt", assetName)
}

// verifyChecksum confirms filePath's SHA-256 digest matches expectedHash.
func verifyChecksum(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("update: checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	return nil
}

// DoUpdate downloads release's binary asset for this platform, verifies
// its SHA-256 checksum against the release's mandatory sha256.txt asset,
// and replaces the binary at currentPath, per AI.md PART 23 "Update Flow"
// steps 2-4. Checksum verification is mandatory: a release with no
// sha256.txt asset is refused rather than installed unverified.
func (c *Client) DoUpdate(ctx context.Context, release *Release, currentPath string) error {
	assetName := getBinaryName()
	var downloadURL, checksumURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			downloadURL = asset.BrowserDownloadURL
		case "sha256.txt":
			checksumURL = asset.BrowserDownloadURL
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("update: no binary asset %q in release %s", assetName, release.TagName)
	}
	if checksumURL == "" {
		return fmt.Errorf("update: no sha256.txt asset found in release %s - refusing unverified update", release.TagName)
	}

	tmpFile, err := os.CreateTemp("", paths.ProjectName()+"-update-*")
	if err != nil {
		return fmt.Errorf("update: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		tmpFile.Close()
		return err
	}
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		tmpFile.Close()
		return err
	}
	_, copyErr := io.Copy(tmpFile, resp.Body)
	resp.Body.Close()
	tmpFile.Close()
	if copyErr != nil {
		return fmt.Errorf("update: download: %w", copyErr)
	}

	expectedHash, err := c.fetchChecksum(ctx, checksumURL, assetName)
	if err != nil {
		return fmt.Errorf("update: fetch checksum: %w", err)
	}
	if err := verifyChecksum(tmpPath, expectedHash); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return fmt.Errorf("update: set permissions: %w", err)
		}
	}

	resolvedPath, err := filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("update: resolve symlinks: %w", err)
	}

	return replaceBinary(resolvedPath, tmpPath)
}

// Update performs the full "--update yes" flow: it checks branch for the
// true latest release (ignoring defer_days), and if newer than
// currentVersion, downloads, verifies, installs it at binaryPath, and
// restarts serviceName's service. A nil Release with a nil error means
// the running version is already current.
func (c *Client) Update(ctx context.Context, currentVersion, branch, binaryPath, serviceName string) (*Release, error) {
	release, err := c.CheckLatest(ctx, currentVersion, branch)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, nil
	}
	if err := c.DoUpdate(ctx, release, binaryPath); err != nil {
		return nil, err
	}
	if err := restartService(serviceName); err != nil {
		return release, err
	}
	return release, nil
}

// SetBranch validates and persists update.branch to the config file, per
// AI.md PART 23 "--update branch {name} writes update.branch to the
// config file — the config is the single source of truth". branch is
// matched case-insensitively against stable/beta/daily.
func SetBranch(cfg *config.Config, branch string) error {
	branch = strings.ToLower(branch)
	switch branch {
	case "stable", "beta", "daily":
	default:
		return fmt.Errorf("update: invalid branch %q: must be stable, beta, or daily", branch)
	}
	cfg.Server.Update.Branch = branch
	return cfg.Save()
}

// CheckForUpdate is the update_check scheduler task entry point, per
// AI.md PART 23 "Scheduled Check (update_check Task)". It looks for the
// newest release on branch that is eligible under deferDays as of now
// and, when found, calls notify (which may be nil). When autoInstall is
// true it additionally downloads, verifies, installs the release at
// binaryPath, and restarts serviceName's service; when false it only
// notifies, per "auto_install: false ... never touches the binary".
func (c *Client) CheckForUpdate(ctx context.Context, currentVersion, branch string, deferDays int, autoInstall bool, binaryPath, serviceName string, notify func(*Release), logger Logger) error {
	release, err := c.CheckEligible(ctx, currentVersion, branch, deferDays, time.Now())
	if err != nil {
		return err
	}
	if release == nil {
		return nil
	}
	if notify != nil {
		notify(release)
	}
	if !autoInstall {
		return nil
	}
	if err := c.DoUpdate(ctx, release, binaryPath); err != nil {
		return err
	}
	if logger != nil {
		logger.Infof("update: installed %s, restarting", release.TagName)
	}
	return restartService(serviceName)
}
