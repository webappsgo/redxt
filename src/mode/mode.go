// Package mode implements application mode and debug detection per
// AI.md PART 6 (Application Modes): six operational states derived
// from a mode value (production/development/debug) crossed with a
// boolean debug flag, with strict CLI-flag > env-var > default
// priority and debug mode's opt-in-only semantics.
package mode

import (
	"os"
	"strconv"
	"strings"
)

// Mode is one of the three application modes.
type Mode string

const (
	// Production is the default mode: minimal logging, no debug
	// endpoints, full security hardening.
	Production Mode = "production"
	// Development enables verbose logging and relaxed caching but
	// keeps debug endpoints disabled unless --debug is also set.
	Development Mode = "development"
	// Debug is an explicit opt-in mode that defaults the debug flag
	// on; it is NEVER implied or auto-enabled.
	Debug Mode = "debug"
)

// State holds the fully resolved mode + debug pair for the process.
type State struct {
	Mode  Mode
	Debug bool
}

// Resolve determines the operational state from CLI flags (highest
// priority), environment variables, then defaults, per PART 6.
//
// modeFlag/debugFlag are the raw values passed via --mode/--debug;
// pass "" / nil when the flag was not set on the command line.
func Resolve(modeFlag string, debugFlag *bool) State {
	m := resolveMode(modeFlag)

	debug := false
	switch {
	case debugFlag != nil:
		// --debug flag always wins, in either direction.
		debug = *debugFlag
	case envSet("DEBUG"):
		debug = truthy(os.Getenv("DEBUG"))
	case m == Debug:
		// MODE=debug defaults the debug flag to on.
		debug = true
	default:
		debug = false
	}

	return State{Mode: m, Debug: debug}
}

func resolveMode(modeFlag string) Mode {
	if modeFlag != "" {
		return normalize(modeFlag)
	}
	if v := os.Getenv("MODE"); v != "" {
		return normalize(v)
	}
	return Production
}

func normalize(raw string) Mode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "dev", "devel", "development":
		return Development
	case "prod", "production":
		return Production
	case "debug":
		return Debug
	default:
		return Production
	}
}

func envSet(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

func truthy(v string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false
	}
	return b
}

// DebugEndpointsEnabled reports whether /debug/* routes (including
// pprof and expvar) should be registered. Debug endpoints are gated
// solely on the Debug flag, independent of Mode.
func (s State) DebugEndpointsEnabled() bool {
	return s.Debug
}

// Icon returns the console-output icon used when printing the running
// mode banner (🔒 production, 🔧 development/debug).
func (s State) Icon() string {
	if s.Mode == Production {
		return "🔒"
	}
	return "🔧"
}
