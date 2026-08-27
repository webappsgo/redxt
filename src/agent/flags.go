package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options holds every value redxt-agent accepts on the command line.
// Per AI.md PART 33 "Agent Flags", the agent has no --port/--address
// (it never serves HTTP). --service and --update are documented but
// depend on PART 24/25 (service install) and PART 23 (self-update),
// neither of which is built anywhere in this codebase yet (confirmed:
// no other binary implements them either) — they are intentionally
// omitted from this pass rather than parsed and left non-functional.
// See TODO.AI.md.
type Options struct {
	Help    bool
	Version bool

	// Shell is the --shell subcommand: completions, init, or help.
	Shell string
	// ShellName is the optional [SHELL] positional argument.
	ShellName string

	// Config, Data, and Log are directory overrides (unlike redxt-cli's
	// --config, which names a profile file).
	Config string
	Data   string
	Log    string

	Server string
	Token  string

	Mode     string
	Debug    bool
	DebugSet bool
	Color    string
	Lang     string

	// Status requests a one-shot connectivity health check
	// (exit 0=healthy, 1=unhealthy) instead of starting the agent.
	Status bool

	// Args holds any leftover positional arguments (e.g. the
	// documented but not-yet-implemented status/test/register
	// subcommands), so Run can report them clearly rather than
	// silently ignore them.
	Args []string
}

// BinaryName returns the name the binary was invoked as, so a renamed
// binary documents itself correctly in --help and --version output.
func BinaryName() string {
	name := filepath.Base(os.Args[0])
	return strings.TrimSuffix(name, ".exe")
}

// Parse builds the fixed flag set and parses args (which must not
// include the program name).
func Parse(name string, args []string, errOut io.Writer) (*Options, error) {
	var o Options

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { fmt.Fprint(errOut, Help(name)) }

	fs.BoolVar(&o.Help, "help", false, "Show help")
	fs.BoolVar(&o.Help, "h", false, "Show help")
	fs.BoolVar(&o.Version, "version", false, "Show version")
	fs.BoolVar(&o.Version, "v", false, "Show version")
	fs.StringVar(&o.Shell, "shell", "", "Shell integration: completions, init, help")
	fs.StringVar(&o.Config, "config", "", "Config directory (default: platform config dir)")
	fs.StringVar(&o.Data, "data", "", "Data directory override")
	fs.StringVar(&o.Log, "log", "", "Log directory override")
	fs.StringVar(&o.Server, "server", "", "Server URL to connect to")
	fs.StringVar(&o.Token, "token", "", "Authentication token")
	fs.StringVar(&o.Mode, "mode", "", "Application mode: production, development, debug")
	fs.BoolVar(&o.Debug, "debug", false, "Enable debug logging")
	fs.StringVar(&o.Color, "color", "auto", "Color output: auto, yes, no")
	fs.StringVar(&o.Lang, "lang", "", "Language for output")
	fs.BoolVar(&o.Status, "status", false, "Show agent health (exit 0=healthy, 1=unhealthy)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "debug" {
			o.DebugSet = true
		}
	})

	rest := fs.Args()
	// "--shell completions bash" leaves "bash" as a positional argument
	// once the flag package has consumed --shell's own value.
	if o.Shell != "" && len(rest) > 0 {
		o.ShellName = rest[0]
		rest = rest[1:]
	}
	o.Args = rest

	return &o, nil
}

// DebugFlag returns nil when --debug was never given, so the
// {PROJECT_NAME}_AGENT_DEBUG environment variable still applies, and a
// pointer to the parsed value otherwise.
func (o *Options) DebugFlag() *bool {
	if !o.DebugSet {
		return nil
	}
	debug := o.Debug
	return &debug
}
