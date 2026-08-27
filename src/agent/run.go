package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/webappsgo/redxt/src/common/shellcomp"
)

// IO groups the streams Run reads from and writes to, so tests never
// touch the real stdio.
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Run parses args and executes the corresponding redxt-agent behavior,
// returning the process exit code.
//
// The foreground data-plane runtime ("run agent" with no flags, per
// AI.md PART 33 "Agent Commands") and the status/test/register
// subcommands all depend on a server-side agent-registration API that
// does not exist yet (server.db agents/tokens CRUD is schema-only) —
// they are reported clearly rather than silently faked. See
// TODO.AI.md.
func Run(args []string, io IO) int {
	name := BinaryName()

	opts, err := Parse(name, args, io.Err)
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if opts.Help {
		fmt.Fprint(io.Out, Help(name))
		return 0
	}
	if opts.Version {
		fmt.Fprint(io.Out, VersionLine(name))
		return 0
	}
	if opts.Shell != "" {
		if opts.Shell == "help" {
			fmt.Fprint(io.Out, ShellHelp(name))
			return 0
		}
		return runShell(opts, name, io)
	}

	cfg, cfgErr := LoadConfig(ConfigPath(opts.Config))
	if cfgErr != nil {
		fmt.Fprintf(io.Err, "load config: %s\n", cfgErr)
		return 1
	}

	token, tokenErr := ResolveToken(opts.Token, os.Getenv("REDXT_AGENT_TOKEN"), cfg)
	if tokenErr != nil {
		fmt.Fprintf(io.Err, "read token file: %s\n", tokenErr)
		return 1
	}

	server := opts.Server
	if server == "" {
		server = cfg.Server.Primary
	}

	client := NewHTTPClient(server, token)

	if opts.Status {
		return RunStatus(client, io.Out, io.Err)
	}

	if len(opts.Args) > 0 {
		fmt.Fprintf(io.Err, "%q is not yet available: agent registration is not implemented on the server yet (see TODO.AI.md)\n", opts.Args[0])
		return 2
	}

	fmt.Fprintln(io.Err, "the agent foreground runtime is not yet available in this build (see TODO.AI.md); use --status to check server connectivity")
	return 2
}

// runShell executes the --shell completions/init subcommand.
func runShell(opts *Options, name string, io IO) int {
	shellName := opts.ShellName
	if shellName == "" {
		shellName = shellcomp.DetectShell()
	}

	switch opts.Shell {
	case "completions":
		script, err := shellcomp.Completions(shellSpec, shellName, name)
		if err != nil {
			fmt.Fprintf(io.Err, "%s\n", err)
			return 1
		}
		fmt.Fprint(io.Out, script)
		return 0
	case "init":
		line, err := shellcomp.Init(shellName, name)
		if err != nil {
			fmt.Fprintf(io.Err, "%s\n", err)
			return 1
		}
		fmt.Fprint(io.Out, line)
		return 0
	default:
		fmt.Fprintf(io.Err, "unknown --shell command %q (use completions, init, or help)\n", opts.Shell)
		return 1
	}
}
