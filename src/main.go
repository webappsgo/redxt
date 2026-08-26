// Command redxt is the server binary for redxt, the complete DNS
// server (see IDEA.md). It stamps the build metadata into the version
// package and hands the whole startup sequence to src/startup, which
// owns the ordering defined in AI.md PART 8.
package main

import (
	"context"
	"os"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/startup"
)

// Version, CommitID, BuildEpoch, and OfficialSite are stamped at build
// time via -ldflags (see Makefile).
var (
	Version      = "devel"
	CommitID     = "N/A"
	BuildEpoch   = "0"
	OfficialSite = "redxt.us"
)

func main() {
	version.Set(Version, CommitID, BuildEpoch, OfficialSite)

	// Signal handling belongs to src/signals, which registers the full
	// platform signal table in startup step 19; the context here exists
	// only so the sequence has one to carry.
	os.Exit(startup.Run(context.Background(), os.Args[1:], startup.IO{Out: os.Stdout, Err: os.Stderr}))
}
