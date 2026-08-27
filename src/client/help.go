package main

import (
	"fmt"
	"strings"

	"github.com/webappsgo/redxt/src/common/shellcomp"
	"github.com/webappsgo/redxt/src/common/version"
)

// shellSpec is the completion surface for redxt-cli, shared by the
// --shell completions/init subcommands and the --help renderer below,
// so the two can never drift apart.
var shellSpec = shellcomp.Spec{
	Flags: []string{
		"--help", "--version", "--shell", "--server", "--token",
		"--token-file", "--user", "--config", "--debug", "--color", "--lang",
	},
	FileFlags: []string{"--token-file"},
	EnumFlags: map[string][]string{
		"--shell": {"completions", "init", "help"},
		"--color": {"auto", "yes", "no"},
	},
}

// Help renders the redxt-cli --help output defined in AI.md PART 33
// "--help Output". name is the actual binary name, so a renamed binary
// documents itself correctly.
func Help(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s - CLI for redxt\n\n", name, version.Version())
	b.WriteString("Usage:\n")
	fmt.Fprintf(&b, "  %s [args] [flags]\n", name)
	b.WriteString("  # TUI mode (no args)\n")
	fmt.Fprintf(&b, "  %s\n\n", name)

	b.WriteString("Flags:\n")
	b.WriteString("-h, --help                             - Show help\n")
	b.WriteString("-v, --version                          - Show version\n")
	b.WriteString("--shell completions [SHELL]            - Print shell completions (auto-detect if SHELL omitted)\n")
	b.WriteString("--shell init [SHELL]                   - Print shell init command (auto-detect if SHELL omitted)\n")
	b.WriteString("--shell help                           - Show shell integration help\n\n")

	b.WriteString("--server URL                           - Server URL (default: from config)\n")
	b.WriteString("--token TOKEN                          - API token for authentication\n")
	b.WriteString("--token-file FILE                      - Read token from file\n")
	b.WriteString("--user NAME                            - Target user or org (auto-detect, @user, +org)\n")
	b.WriteString("--config NAME                          - Config profile name (default: cli.yml)\n")
	b.WriteString("--debug                                - Debug output\n")
	b.WriteString("--color {auto|yes|no}                  - Color output (default: auto)\n")
	b.WriteString("--lang CODE                            - Language for output (default: auto)\n\n")

	b.WriteString("Commands:\n")
	b.WriteString("health                                  - Show server health and exit non-zero if unhealthy\n\n")

	fmt.Fprintf(&b, "Shells: %s\n\n", strings.Join(shellcomp.Shells(), ", "))
	b.WriteString("Run without arguments for interactive TUI mode.\n")
	fmt.Fprintf(&b, "Run '%s <command> help' for detailed help on any command.\n", name)
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

// VersionLine renders the redxt-cli --version output defined in AI.md
// PART 33: "{project_name}-cli {project_version} ({commit_sha}) built
// {build_date}", using the actual (possibly renamed) binary name.
func VersionLine(name string) string {
	return fmt.Sprintf("%s %s (%s) built %s\n", name, version.Version(), version.Commit(), version.BuildDate())
}

// UserAgent returns the outbound HTTP User-Agent. Per AI.md PART 33 the
// internal project name is hardcoded even though the binary is
// renameable, so clients can be identified server-side regardless of
// what the user named the file.
func UserAgent() string {
	return "redxt-cli/" + version.Version()
}
