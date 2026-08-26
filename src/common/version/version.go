// Package version is the single source of truth for build and version
// information, implementing AI.md PART 13 "--version Output" and the
// PART 27 build-variable contract (main.Version, main.CommitID,
// main.BuildEpoch, main.OfficialSite stamped via -ldflags).
package version

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
)

// Default values used when the binary was not stamped by the build system.
var (
	version      = "dev"
	commit       = "N/A"
	buildEpoch   = "0"
	officialSite = "redxt.us"
)

// Set pushes the values stamped into package main by -ldflags into this
// package. Empty arguments are ignored so each field keeps its default.
// Call once from main at startup.
func Set(v, c, epoch, site string) {
	if v != "" {
		version = v
	}
	if c != "" {
		commit = c
	}
	if epoch != "" {
		buildEpoch = epoch
	}
	if site != "" {
		officialSite = site
	}
}

// Version returns the project version string.
func Version() string {
	return version
}

// Commit returns the short git commit hash the binary was built from.
func Commit() string {
	return commit
}

// OfficialSite returns the default server URL embedded at build time.
func OfficialSite() string {
	return officialSite
}

// BuildEpoch returns the build timestamp as Unix seconds, 0 when unset or unparsable.
func BuildEpoch() int64 {
	n, err := strconv.ParseInt(buildEpoch, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// BuildDate returns the build timestamp as an ISO 8601 (RFC 3339) UTC string,
// or "unknown" when no build epoch was embedded.
func BuildDate() string {
	epoch := BuildEpoch()
	if epoch == 0 {
		return "unknown"
	}
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

// GoVersion returns the Go runtime version the binary was compiled with.
func GoVersion() string {
	return runtime.Version()
}

// Platform returns the compile-time target as "{GOOS}/{GOARCH}".
func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// AssetStamp returns "{version}-{short_commit}", the cache-busting stamp
// required by AI.md PART 9 for static assets and ETags.
func AssetStamp() string {
	return Version() + "-" + Commit()
}

// UserAgent returns the outbound HTTP User-Agent. Per AI.md PART 8 the
// internal project name is hardcoded even though the binary is renameable.
func UserAgent() string {
	return "redxt/" + Version()
}

// String renders the exact --version output block defined in AI.md PART 13,
// using binaryName as the (renameable) program name.
func String(binaryName string) string {
	return fmt.Sprintf("%s %s\nBuilt: %s\nGo: %s\nOS/Arch: %s\n",
		binaryName, Version(), BuildDate(), GoVersion(), Platform())
}
