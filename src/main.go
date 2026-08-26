// Command redxt is the server binary for redxt, the complete DNS
// server (see IDEA.md). This is the PART 0-6 foundation entry point:
// flag parsing, mode/debug resolution, config load, and path
// resolution. Feature commands (zones, DNSSEC, clustering, etc.) are
// wired in by their owning PART implementations per TODO.AI.md.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/mode"
	"github.com/webappsgo/redxt/src/paths"
)

// Version, CommitID, BuildDate, and OfficialSite are stamped at build
// time via -ldflags (see Makefile).
var (
	Version      = "devel"
	CommitID     = "N/A"
	BuildDate    = "unknown"
	OfficialSite = "redxt.us"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("redxt", flag.ContinueOnError)
	fs.Usage = func() { printHelp(fs) }

	help := fs.Bool("help", false, "Show this help message and exit")
	fs.BoolVar(help, "h", false, "Show this help message and exit")
	version := fs.Bool("version", false, "Show version and exit")
	fs.BoolVar(version, "v", false, "Show version and exit")

	debugSet := false
	debug := fs.Bool("debug", false, "Enable debug output")
	modeFlag := fs.String("mode", "", "Application mode: production, development, debug")
	colorFlag := fs.String("color", "auto", "Color output: auto, yes, no")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "debug" {
			debugSet = true
		}
	})

	if len(fs.Args()) > 0 && fs.Args()[0] == "help" {
		printHelp(fs)
		return 0
	}
	if *help {
		printHelp(fs)
		return 0
	}
	if *version {
		printVersion()
		return 0
	}

	_ = colorFlag

	var debugPtr *bool
	if debugSet {
		debugPtr = debug
	}
	state := mode.Resolve(*modeFlag, debugPtr)

	p := paths.Resolve()

	cfg, err := config.Load(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "redxt: %v\n", err)
		return 1
	}

	fmt.Printf("%s Running in mode: %s", state.Icon(), state.Mode)
	if state.Debug {
		fmt.Print(" [debugging]")
	}
	fmt.Println()
	fmt.Printf("redxt %s listening on %s:%d (config: %s)\n",
		Version, cfg.Server.Listen, cfg.Server.Port, cfg.Path())

	return 0
}

func printVersion() {
	fmt.Printf("redxt %s (commit %s, built %s)\n%s\n", Version, CommitID, BuildDate, OfficialSite)
}

func printHelp(fs *flag.FlagSet) {
	fmt.Println("redxt - The Complete DNS Server")
	fmt.Println()
	fmt.Println("Usage: redxt [flags]")
	fmt.Println()
	printItem("--help, -h", "Show this help message and exit")
	printItem("--version, -v", "Show version and exit")
	printItem("--debug", "Enable debug output")
	printItem("--mode", "Application mode: production, development, debug")
	printItem("--color", "Color output: auto, yes, no")
	printItem("help", "Show this help message and exit")
}

func printItem(name, desc string) {
	fmt.Printf("%-38s- %s\n", name, desc)
}
