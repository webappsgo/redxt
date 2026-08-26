package cli

import (
	"fmt"
	"strings"

	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/config"
)

// Help renders the server --help output defined in AI.md PART 8
// "Server --help Output". name is the actual binary name, so a renamed
// binary documents itself correctly.
func Help(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s - %s\n\n", name, version.Version(), config.DefaultApplicationTagline)
	b.WriteString("Usage:\n")
	fmt.Fprintf(&b, "  %s [flags]\n\n", name)

	b.WriteString("Information:\n")
	b.WriteString("-h, --help                             - Show help (--help for any command shows its help)\n")
	b.WriteString("-v, --version                          - Show version\n")
	b.WriteString("--status                               - Show server status and health\n\n")

	b.WriteString("Shell Integration:\n")
	b.WriteString("--shell completions [SHELL]            - Print shell completions\n")
	b.WriteString("--shell init [SHELL]                   - Print shell init command\n")
	b.WriteString("--shell help                           - Show shell help\n\n")

	b.WriteString("Server Configuration:\n")
	b.WriteString("--mode {production|development|debug}  - Application mode (default: production)\n")
	b.WriteString("--config DIR                           - Config directory\n")
	b.WriteString("--data DIR                             - Data directory\n")
	b.WriteString("--cache DIR                            - Cache directory\n")
	b.WriteString("--log DIR                              - Log directory\n")
	b.WriteString("--backup DIR                           - Backup directory\n")
	b.WriteString("--pid FILE                             - PID file path\n")
	b.WriteString("--address ADDR                         - Listen address (default: 0.0.0.0)\n")
	b.WriteString("--port PORT                            - Listen port (default: random 64xxx, 80 in container)\n")
	b.WriteString("--baseurl PATH                         - URL path prefix (default: /)\n")
	b.WriteString("--daemon                               - Run as daemon (detach from terminal)\n")
	b.WriteString("--debug                                - Enable debug mode\n")
	b.WriteString("--color {auto|yes|no}                  - Color output (default: auto)\n")
	b.WriteString("--lang CODE                            - Language for output (default: auto)\n\n")

	fmt.Fprintf(&b, "Run '%s <command> help' for detailed help on any command.\n", name)
	return b.String()
}

// ShellHelp renders the --shell help output. The shell list matches the
// completion generators this binary actually ships.
func ShellHelp(name string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s --shell - shell integration\n\n", name)
	b.WriteString("Usage:\n")
	fmt.Fprintf(&b, "  %s --shell completions [SHELL]        - Print the completion script to stdout\n", name)
	fmt.Fprintf(&b, "  %s --shell init [SHELL]               - Print the line to add to your shell rc file\n", name)
	fmt.Fprintf(&b, "  %s --shell help                       - Show this help\n\n", name)
	b.WriteString("SHELL is optional and is auto-detected from $SHELL when omitted.\n\n")
	fmt.Fprintf(&b, "Shells: %s\n\n", strings.Join(SupportedShells(), ", "))
	b.WriteString("Examples:\n")
	fmt.Fprintf(&b, "  %s --shell completions bash > /etc/bash_completion.d/%s\n", name, name)
	fmt.Fprintf(&b, "  eval \"$(%s --shell init)\"\n", name)
	return b.String()
}
