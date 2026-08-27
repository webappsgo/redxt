package main

import (
	"os"

	"github.com/webappsgo/redxt/src/common/version"
)

// Version, CommitID, BuildEpoch, and OfficialSite are stamped at build
// time via -ldflags (see Makefile), matching the root server binary's
// contract so a single LDFLAGS variable stamps every binary.
var (
	Version      = "devel"
	CommitID     = "N/A"
	BuildEpoch   = "0"
	OfficialSite = "redxt.us"
)

func main() {
	version.Set(Version, CommitID, BuildEpoch, OfficialSite)
	os.Exit(Run(os.Args[1:], IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}
