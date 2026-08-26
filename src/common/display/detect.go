// Package display implements AI.md PART 7 "Display Environment Detection":
// it classifies the runtime UI environment (headless, cli, tui, gui) and
// reports whether ANSI escapes may be used.
package display

import (
	"os"
	"strings"

	"github.com/webappsgo/redxt/src/common/terminal"
)

// DisplayMode is the detected UI display mode, not the application mode.
type DisplayMode int

const (
	// DisplayModeHeadless means no display and no TTY (daemon, service, cron).
	DisplayModeHeadless DisplayMode = iota
	// DisplayModeCLI means command-line only output (piped, dumb terminal, or a command was given).
	DisplayModeCLI
	// DisplayModeTUI means an interactive terminal is available.
	DisplayModeTUI
	// DisplayModeGUI means a native graphical display is available.
	DisplayModeGUI
)

// String returns the lowercase mode name.
func (m DisplayMode) String() string {
	switch m {
	case DisplayModeHeadless:
		return "headless"
	case DisplayModeCLI:
		return "cli"
	case DisplayModeTUI:
		return "tui"
	case DisplayModeGUI:
		return "gui"
	}
	return "unknown"
}

// DisplayEnv is the detected display environment.
type DisplayEnv struct {
	Mode DisplayMode
	// HasDisplay reports whether a native X11, Wayland, Quartz or Win32 display exists.
	HasDisplay bool
	// DisplayType names the native display ("x11", "wayland", "quartz", "win32", "rdp", "session0" or empty).
	DisplayType string
	// IsTerminal reports whether stdout is a character device.
	IsTerminal bool
	// IsSSH reports whether the process runs inside an SSH session.
	IsSSH bool
	// IsMosh reports whether the process runs inside a mosh session.
	IsMosh bool
	// IsScreen reports whether the process runs inside screen or tmux.
	IsScreen bool
	// TerminalType is the TERM environment value.
	TerminalType string
	// Cols is the detected terminal column count.
	Cols int
	// Rows is the detected terminal row count.
	Rows int
}

// DetectDisplayEnv auto-detects the current display environment.
func DetectDisplayEnv() DisplayEnv {
	env := DisplayEnv{}

	env.TerminalType = os.Getenv("TERM")
	env.IsTerminal = stdoutIsTerminal()

	env.IsSSH = os.Getenv("SSH_CLIENT") != "" ||
		os.Getenv("SSH_TTY") != "" ||
		os.Getenv("SSH_CONNECTION") != ""
	env.IsMosh = os.Getenv("MOSH_CONNECTION") != "" || strings.Contains(env.TerminalType, "mosh")
	env.IsScreen = os.Getenv("STY") != "" ||
		os.Getenv("TMUX") != "" ||
		strings.HasPrefix(env.TerminalType, "screen") ||
		strings.HasPrefix(env.TerminalType, "tmux")

	env.HasDisplay, env.DisplayType = detectDisplay()

	size := terminal.GetTerminalSize()
	env.Cols = size.Cols
	env.Rows = size.Rows

	env.Mode = autoDetectDisplayMode(env)

	return env
}

// stdoutIsTerminal reports whether stdout is a character device.
func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// autoDetectDisplayMode determines the display mode from a detected environment.
func autoDetectDisplayMode(env DisplayEnv) DisplayMode {
	if env.TerminalType == "dumb" {
		return DisplayModeCLI
	}
	if !env.IsTerminal && !env.HasDisplay {
		return DisplayModeHeadless
	}
	if env.HasDisplay && !env.IsSSH && !env.IsMosh {
		return DisplayModeGUI
	}
	if env.IsTerminal {
		return DisplayModeTUI
	}
	return DisplayModeCLI
}

// IsDumbTerminal reports whether TERM is "dumb", meaning no ANSI capabilities at all.
func IsDumbTerminal() bool {
	return os.Getenv("TERM") == "dumb"
}

// IsDumbTerminal reports whether this environment was detected as a dumb terminal.
func (e DisplayEnv) IsDumbTerminal() bool {
	return e.TerminalType == "dumb"
}

// CanUseANSI reports whether ANSI escape sequences (colors, cursor control,
// screen clearing) may be emitted. NO_COLOR is honored here because users who
// set it want plain output.
func CanUseANSI(env DisplayEnv) bool {
	if IsDumbTerminal() || env.IsDumbTerminal() {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return env.IsTerminal
}

// IsAutoDetectDisplayModeGUI reports whether this environment resolved to GUI mode.
func (e DisplayEnv) IsAutoDetectDisplayModeGUI() bool {
	return e.Mode == DisplayModeGUI
}

// IsAutoDetectDisplayModeTUI reports whether this environment resolved to TUI mode.
func (e DisplayEnv) IsAutoDetectDisplayModeTUI() bool {
	return e.Mode == DisplayModeTUI
}

// IsAutoDetectDisplayModeCLI reports whether this environment resolved to CLI mode.
func (e DisplayEnv) IsAutoDetectDisplayModeCLI() bool {
	return e.Mode == DisplayModeCLI
}

// IsAutoDetectDisplayModeHeadless reports whether this environment resolved to headless mode.
func (e DisplayEnv) IsAutoDetectDisplayModeHeadless() bool {
	return e.Mode == DisplayModeHeadless
}

// IsAutoDetectDisplayModeGUI detects the environment and reports whether it is GUI mode.
func IsAutoDetectDisplayModeGUI() bool {
	return DetectDisplayEnv().Mode == DisplayModeGUI
}

// IsAutoDetectDisplayModeTUI detects the environment and reports whether it is TUI mode.
func IsAutoDetectDisplayModeTUI() bool {
	return DetectDisplayEnv().Mode == DisplayModeTUI
}

// IsAutoDetectDisplayModeCLI detects the environment and reports whether it is CLI mode.
func IsAutoDetectDisplayModeCLI() bool {
	return DetectDisplayEnv().Mode == DisplayModeCLI
}

// IsAutoDetectDisplayModeHeadless detects the environment and reports whether it is headless mode.
func IsAutoDetectDisplayModeHeadless() bool {
	return DetectDisplayEnv().Mode == DisplayModeHeadless
}
