// Command redxt-cli is the client binary for redxt (see IDEA.md),
// implementing the fixed flag set, help/version output, shell
// completions, and config handling defined in AI.md PART 33
// "Client (CLI) Binary".
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options holds every value redxt-cli accepts on the command line.
type Options struct {
	Help    bool
	Version bool

	// Shell is the --shell subcommand: completions, init, or help.
	Shell string
	// ShellName is the optional [SHELL] positional argument.
	ShellName string

	Server    string
	Token     string
	TokenFile string
	User      string
	Config    string

	Debug    bool
	DebugSet bool
	Color    string
	Lang     string

	// Args holds the non-flag positional arguments (the command and
	// its own arguments), so a project-specific command line such as
	// "redxt-cli health" still parses.
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
	fs.StringVar(&o.Server, "server", "", "Server URL")
	fs.StringVar(&o.Token, "token", "", "API token for authentication")
	fs.StringVar(&o.TokenFile, "token-file", "", "Read token from file")
	fs.StringVar(&o.User, "user", "", "Target user or org (@user, +org)")
	fs.StringVar(&o.Config, "config", "", "Config profile name (default: cli.yml)")
	fs.BoolVar(&o.Debug, "debug", false, "Debug output")
	fs.StringVar(&o.Color, "color", "auto", "Color output: auto, yes, no")
	fs.StringVar(&o.Lang, "lang", "", "Language for output")

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
// {PROJECT_NAME}_DEBUG environment variable still applies, and a
// pointer to the parsed value otherwise.
func (o *Options) DebugFlag() *bool {
	if !o.DebugSet {
		return nil
	}
	debug := o.Debug
	return &debug
}
