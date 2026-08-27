package main

import (
	"fmt"
	"strings"

	"github.com/webappsgo/redxt/src/common/shellcomp"
	"github.com/webappsgo/redxt/src/common/version"
)

// shellSpec is the completion surface for redxt-agent, shared by the
// --shell completions/init subcommands and the --help renderer below,
// so the two can never drift apart. --service/--update are documented
// in AI.md PART 33 but not yet implemented anywhere in this codebase
// (see flags.go) and are intentionally excluded.
var shellSpec = shellcomp.Spec{
	Flags: []string{
		"--help", "--version", "--shell", "--config", "--data", "--log",
		"--server", "--token", "--mode", "--debug", "--color", "--lang",
		"--status",
	},
	DirFlags: []string{"--config", "--data", "--log"},
	EnumFlags: map[string][]string{
		"--shell": {"completions", "init", "help"},
		"--mode":  {"production", "development", "debug"},
		"--color": {"auto", "yes", "no"},
	},
}

// Help renders the redxt-agent --help output defined in AI.md PART 33
// "Agent --help Output", limited to the flags actually implemented this
// pass. name is the actual binary name, so a renamed binary documents
// itself correctly.
func Help(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s - Agent for redxt\n\n", name, version.Version())
	b.WriteString("Usage:\n")
	fmt.Fprintf(&b, "  %s [flags]\n\n", name)

	b.WriteString("Flags:\n")
	b.WriteString("-h, --help                             - Show help\n")
	b.WriteString("-v, --version                          - Show version\n")
	b.WriteString("--shell completions [SHELL]            - Print shell completions (auto-detect if SHELL omitted)\n")
	b.WriteString("--shell init [SHELL]                   - Print shell init command (auto-detect if SHELL omitted)\n")
	b.WriteString("--shell help                           - Show shell integration help\n\n")

	b.WriteString("--config DIR                           - Config directory\n")
	b.WriteString("--data DIR                              - Data directory\n")
	b.WriteString("--log DIR                               - Log directory\n")
	b.WriteString("--server URL                            - Server URL to connect to\n")
	b.WriteString("--token TOKEN                           - Authentication token\n\n")

	b.WriteString("--mode {production|development|debug}  - Application mode\n")
	b.WriteString("--debug                                - Enable debug mode\n")
	b.WriteString("--color {auto|yes|no}                  - Color output (default: auto)\n")
	b.WriteString("--lang CODE                             - Language for output (default: auto)\n")
	b.WriteString("--status                                - Show agent health (exit 0=healthy, 1=unhealthy)\n\n")

	fmt.Fprintf(&b, "Shells: %s\n", strings.Join(shellcomp.Shells(), ", "))
	return b.String()
}

// ShellHelp renders the --shell help output.
func ShellHelp(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s --shell - shell integration\n\n", name)
	b.WriteString("Usage:\n")
	fmt.Fprintf(&b, "  %s --shell completions [SHELL]        - Print the completion script to stdout\n", name)
	fmt.Fprintf(&b, "  %s --shell init [SHELL]               - Print the line to add to your shell rc file\n", name)
	fmt.Fprintf(&b, "  %s --shell help                       - Show this help\n\n", name)
	b.WriteString("SHELL is optional and is auto-detected from $SHELL when omitted.\n\n")
	fmt.Fprintf(&b, "Shells: %s\n\n", strings.Join(shellcomp.Shells(), ", "))
	b.WriteString("Examples:\n")
	fmt.Fprintf(&b, "  %s --shell completions bash > ~/.local/share/bash-completion/completions/%s\n", name, name)
	fmt.Fprintf(&b, "  eval \"$(%s --shell init)\"\n", name)
	return b.String()
}

// VersionLine renders the redxt-agent --version output, matching the
// server and CLI binaries' shared format: "{name} {version} ({commit})
// built {build_date}".
func VersionLine(name string) string {
	return fmt.Sprintf("%s %s (%s) built %s\n", name, version.Version(), version.Commit(), version.BuildDate())
}

// UserAgent returns the outbound HTTP User-Agent. Per AI.md PART 33 the
// internal project name is hardcoded even though the binary is
// renameable, so the server can identify agents regardless of what the
// user named the file.
func UserAgent() string {
	return "redxt-agent/" + version.Version()
}
