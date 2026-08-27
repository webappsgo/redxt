// Package cli implements the server binary's fixed command-line
// surface: the flag set defined in AI.md PART 8 "Server Binary
// Commands", its --help and --version output, the built-in shell
// completions, and the --status client.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Options holds every value the server binary accepts on the command
// line. A zero value means "not supplied", which lets the configuration
// precedence in PART 12 (CLI flag > env var > database > file >
// default) be applied by the caller.
type Options struct {
	// Help and Version are the immediate-exit information flags.
	Help    bool
	Version bool
	// Status queries a running server and exits 0 (healthy) or 1.
	Status bool

	// Shell is the --shell subcommand: completions, init, or help.
	Shell string
	// ShellName is the optional [SHELL] positional argument.
	ShellName string

	// Service is the --service subcommand: start, stop, restart,
	// reload, --install, --disable, --uninstall, or help.
	Service string
	// ServiceArgs holds any positional arguments that followed the
	// --service subcommand.
	ServiceArgs []string

	// Maintenance is the --maintenance subcommand: backup, restore,
	// update, mode, setup, or help.
	Maintenance string
	// MaintenanceArgs holds the optional file or setting argument that
	// followed the --maintenance subcommand.
	MaintenanceArgs []string

	// Update is the --update subcommand: check, yes, branch, or help.
	Update string
	// UpdateArgs holds the branch name that follows "--update branch".
	UpdateArgs []string

	// Mode is production, development, or debug.
	Mode string

	// Directory and file locations.
	Config  string
	Data    string
	Cache   string
	Log     string
	Backup  string
	PIDFile string

	// Listen configuration.
	Address string
	Port    string
	BaseURL string

	// Daemon detaches the process from the terminal.
	Daemon bool
	// Debug enables verbose logging and the debug endpoints.
	Debug bool
	// DebugSet records whether --debug appeared at all, so an unset
	// flag falls through to the DEBUG environment variable.
	DebugSet bool
	// Color is auto, yes, or no.
	Color string
	// Lang is the output language code.
	Lang string
}

// BinaryName returns the name the binary was invoked as. All
// user-facing output uses it, because every binary may be renamed;
// internal identifiers such as the User-Agent and the default paths
// keep the hardcoded project name instead.
func BinaryName() string {
	name := filepath.Base(os.Args[0])
	return strings.TrimSuffix(name, ".exe")
}

// Parse builds the fixed flag set and parses args (which must not
// include the program name). Parse errors are returned rather than
// exiting, so the caller controls the exit path.
func Parse(name string, args []string, errOut io.Writer) (*Options, error) {
	var o Options

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	// The help layout is fixed by the spec, so the default flag
	// listing is replaced wholesale.
	fs.Usage = func() { fmt.Fprint(errOut, Help(name)) }

	fs.BoolVar(&o.Help, "help", false, "Show help")
	fs.BoolVar(&o.Help, "h", false, "Show help")
	fs.BoolVar(&o.Version, "version", false, "Show version")
	fs.BoolVar(&o.Version, "v", false, "Show version")
	fs.BoolVar(&o.Status, "status", false, "Show server status and health")
	fs.StringVar(&o.Shell, "shell", "", "Shell integration: completions, init, help")
	fs.StringVar(&o.Service, "service", "", "Service management: start, stop, restart, reload, --install, --disable, --uninstall, help")
	fs.StringVar(&o.Maintenance, "maintenance", "", "Maintenance: backup, restore, update, mode, setup, help")
	fs.StringVar(&o.Update, "update", "", "Updates: check, yes, branch, help")
	fs.StringVar(&o.Mode, "mode", "", "Application mode: production, development, debug")
	fs.StringVar(&o.Config, "config", "", "Config directory")
	fs.StringVar(&o.Data, "data", "", "Data directory")
	fs.StringVar(&o.Cache, "cache", "", "Cache directory")
	fs.StringVar(&o.Log, "log", "", "Log directory")
	fs.StringVar(&o.Backup, "backup", "", "Backup directory")
	fs.StringVar(&o.PIDFile, "pid", "", "PID file path")
	fs.StringVar(&o.Address, "address", "", "Listen address")
	fs.StringVar(&o.Port, "port", "", "Listen port, or http,https for a dual-port setup")
	fs.StringVar(&o.BaseURL, "baseurl", "", "URL path prefix")
	fs.BoolVar(&o.Daemon, "daemon", false, "Run as daemon")
	fs.BoolVar(&o.Debug, "debug", false, "Enable debug mode")
	fs.StringVar(&o.Color, "color", "auto", "Color output: auto, yes, no")
	fs.StringVar(&o.Lang, "lang", "", "Language for output")

	if err := fs.Parse(normalizeSubcommands(args)); err != nil {
		return nil, err
	}

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "debug" {
			o.DebugSet = true
		}
	})

	// The subcommand flags take optional positional arguments, and
	// "--shell --help" is spelled "--shell help" once the flag package
	// has consumed the value.
	rest := fs.Args()
	if o.Shell != "" && len(rest) > 0 {
		o.ShellName = rest[0]
	}
	if o.Service != "" {
		o.ServiceArgs = rest
	}
	if o.Maintenance != "" {
		o.MaintenanceArgs = rest
	}
	if o.Update != "" {
		o.UpdateArgs = rest
	}

	return &o, nil
}

// subcommandDefaults maps each subcommand flag to the value a bare
// invocation means. --service and --maintenance always need a
// subcommand, so a bare flag prints their help; --update is documented
// as "check/perform updates", and check is its read-only form.
var subcommandDefaults = map[string]string{
	"service":     "help",
	"maintenance": "help",
	"update":      "check",
}

// subcommandDashValues lists the dash-prefixed values the subcommand
// flags legitimately take. Anything else that looks like a flag belongs
// to the main flag set, not to the subcommand.
var subcommandDashValues = map[string]bool{
	"--install":   true,
	"--uninstall": true,
	"--disable":   true,
	"--help":      true,
	"-h":          true,
}

// normalizeSubcommands rewrites a bare --service, --maintenance, or
// --update into its explicit default form. The flag package would
// otherwise fail with "flag needs an argument", and it would also try to
// parse a following --install as a flag of its own.
func normalizeSubcommands(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, ok := bareSubcommandName(arg)
		if !ok {
			out = append(out, arg)
			continue
		}
		if i+1 < len(args) && !isSubcommandBreak(args[i+1]) {
			// The next token is this subcommand's name. Everything after
			// it belongs to the subcommand (a backup path, a branch name,
			// its own --password), so flag parsing stops right here.
			out = append(out, arg, args[i+1], "--")
			return append(out, args[i+2:]...)
		}
		out = append(out, arg+"="+subcommandDefaults[name])
	}
	return out
}

// bareSubcommandName reports whether arg is one of the subcommand flags
// written without an inline "=value".
func bareSubcommandName(arg string) (string, bool) {
	name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	if name == arg || strings.Contains(name, "=") {
		return "", false
	}
	if _, ok := subcommandDefaults[name]; !ok {
		return "", false
	}
	return name, true
}

// isSubcommandBreak reports whether next cannot serve as the value of a
// subcommand flag, which is true for every flag except the dash-prefixed
// values the subcommands themselves define.
func isSubcommandBreak(next string) bool {
	return strings.HasPrefix(next, "-") && !subcommandDashValues[next]
}

// DebugFlag returns the pointer form mode.Resolve expects: nil when
// --debug was never given, so the DEBUG environment variable still
// applies, and a pointer to the parsed value otherwise.
func (o *Options) DebugFlag() *bool {
	if !o.DebugSet {
		return nil
	}
	debug := o.Debug
	return &debug
}

// Ports parses the --port value into its HTTP and, when a dual-port
// value was given, HTTPS components. An empty value yields no ports,
// which lets the environment variable, the config file, and finally the
// random 64000-64999 default apply in turn.
func (o *Options) Ports() ([]int, error) {
	return ParsePorts(o.Port)
}

// ParsePorts parses the "{http}" or "{http},{https}" port format from
// AI.md PART 5. It rejects anything that is not one or two valid TCP
// port numbers.
func ParsePorts(raw string) ([]int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	fields := strings.Split(trimmed, ",")
	if len(fields) > 2 {
		return nil, fmt.Errorf("port %q: expected PORT or HTTP_PORT,HTTPS_PORT", raw)
	}

	ports := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			return nil, fmt.Errorf("port %q: not a number", strings.TrimSpace(field))
		}
		if n < 1 || n > 65535 {
			return nil, fmt.Errorf("port %d: out of range 1-65535", n)
		}
		ports = append(ports, n)
	}
	return ports, nil
}
