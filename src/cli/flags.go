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

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "debug" {
			o.DebugSet = true
		}
	})

	// --shell takes an optional [SHELL] positional argument, and
	// "--shell --help" is spelled "--shell help" once the flag
	// package has consumed the value.
	if rest := fs.Args(); o.Shell != "" && len(rest) > 0 {
		o.ShellName = rest[0]
	}

	return &o, nil
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
