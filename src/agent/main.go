// Command redxt-agent is the agent binary for redxt (see IDEA.md),
// implementing the flag set, help/version output, shell completions,
// and config handling defined in AI.md PART 33 "Agent Binary". The
// foreground data-plane runtime (the agent's actual collect/sync loop)
// and the server-side registration it depends on are not yet built;
// see TODO.AI.md.
package main

import (
	"os"

	"github.com/webappsgo/redxt/src/common/version"
)

// Version, CommitID, BuildEpoch, and OfficialSite are stamped at build
// time via -ldflags (see Makefile), matching the root server binary's
// and redxt-cli's contract so a single LDFLAGS variable stamps every
// binary.
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
