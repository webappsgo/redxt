// Package banner prints the responsive startup banner shared by the
// server and agent binaries. Both show a status banner only; the CLI is
// the only binary with an interactive TUI. The banner adapts to the
// terminal width because phone SSH sessions are common, and it degrades
// to plain text when color and emoji are disabled. See AI.md PART 7
// "Banner Package" and PART 15 "Responsive Startup Banner".
package banner

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/webappsgo/redxt/src/common/color"
	"github.com/webappsgo/redxt/src/common/terminal"
)

// Banner icons, matching the icon table in AI.md PART 15. Log files are
// always raw text, so these never reach a log writer.
const (
	iconApp         = "🚀"
	iconProduction  = "🔒"
	iconDevelopment = "🔧"
	iconDebug       = "🐛"
	iconURL         = "🌐"
	iconSetup       = "🔑"
)

// BannerConfig describes everything the startup banner can display.
type BannerConfig struct {
	AppName string
	Version string
	// AppMode is the application mode, production or development.
	AppMode string
	Debug   bool
	URLs    []string
	// ShowSetup enables the first-run setup token line. Server only.
	ShowSetup bool
	// SetupToken is the one-time setup token. It is only ever written to
	// the console, never to a log file.
	SetupToken string
	// ForceColor overrides color auto-detection when non-nil.
	ForceColor *bool
	// ForceEmoji overrides emoji auto-detection when non-nil.
	ForceEmoji *bool
	// Out receives the banner. It defaults to os.Stdout.
	Out io.Writer
}

// clean strips control characters from a value before it reaches the
// console. Application name, version, mode, and URLs can all come from
// white-label branding or a reverse proxy header, so none of them may
// carry an escape sequence into the terminal.
func clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// cleanAll applies clean to every element of a slice.
func cleanAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, clean(s))
	}
	return out
}

// sanitized returns a copy of the config with every console-bound field
// stripped of control characters.
func (c BannerConfig) sanitized() BannerConfig {
	c.AppName = clean(c.AppName)
	c.Version = clean(c.Version)
	c.AppMode = clean(c.AppMode)
	c.SetupToken = clean(c.SetupToken)
	c.URLs = cleanAll(c.URLs)
	return c
}

// writer returns the configured output stream, defaulting to stdout.
func (c BannerConfig) writer() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return os.Stdout
}

// modeIcon returns the icon for the configured application mode.
func (c BannerConfig) modeIcon() string {
	if c.Debug {
		return iconDebug
	}
	if strings.EqualFold(c.AppMode, "development") {
		return iconDevelopment
	}
	return iconProduction
}

// PrintStartupBanner writes the banner sized for the current terminal.
// When emoji output is disabled — NO_COLOR, TERM=dumb, a non-terminal
// stdout, or an explicit override — the plain variant is used at every
// width.
func PrintStartupBanner(cfg BannerConfig) {
	cfg = cfg.sanitized()
	if !color.EmojiEnabled(cfg.ForceEmoji) {
		printStartupBannerPlain(cfg)
		return
	}
	size := terminal.GetTerminalSize()
	switch {
	case size.Mode >= terminal.SizeModeStandard:
		printStartupBannerFull(cfg, size)
	case size.Mode >= terminal.SizeModeCompact:
		printStartupBannerCompact(cfg)
	case size.Mode >= terminal.SizeModeMinimal:
		printStartupBannerMinimal(cfg)
	default:
		printStartupBannerMicro(cfg)
	}
}

// printStartupBannerPlain writes text with no emoji and no ASCII art.
func printStartupBannerPlain(cfg BannerConfig) {
	w := cfg.writer()
	fmt.Fprintf(w, "%s v%s\n", cfg.AppName, cfg.Version)
	fmt.Fprintf(w, "Running in mode: %s\n", cfg.AppMode)
	for _, url := range cfg.URLs {
		fmt.Fprintf(w, "  %s\n", url)
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Fprintf(w, "  Setup token: %s\n", cfg.SetupToken)
	}
	fmt.Fprintln(w)
}

// printStartupBannerFull writes the ASCII logo followed by the full
// icon-and-URL banner.
func printStartupBannerFull(cfg BannerConfig, size terminal.TerminalSize) {
	w := cfg.writer()
	enabled := color.ColorEnabled(cfg.ForceColor)
	if size.ShowASCIIArt() {
		fmt.Fprintln(w, color.Magenta(enabled, ASCIIArt(cfg.AppName)))
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%s %s v%s\n", iconApp, color.Bold(cfg.AppName), cfg.Version)
	fmt.Fprintf(w, "%s Running in mode: %s\n", cfg.modeIcon(), cfg.AppMode)
	fmt.Fprintln(w)
	for _, url := range cfg.URLs {
		fmt.Fprintf(w, "  %s %s\n", iconURL, color.Cyan(enabled, url))
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Fprintf(w, "  %s Setup token: %s\n", iconSetup, color.Yellow(enabled, cfg.SetupToken))
	}
	fmt.Fprintln(w)
}

// printStartupBannerCompact drops the ASCII art but keeps icons.
func printStartupBannerCompact(cfg BannerConfig) {
	w := cfg.writer()
	fmt.Fprintf(w, "%s %s v%s\n", iconApp, cfg.AppName, cfg.Version)
	fmt.Fprintf(w, "%s Running in mode: %s\n", cfg.modeIcon(), cfg.AppMode)
	for _, url := range cfg.URLs {
		fmt.Fprintf(w, "%s %s\n", iconURL, url)
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Fprintf(w, "%s %s\n", iconSetup, cfg.SetupToken)
	}
}

// printStartupBannerMinimal abbreviates URLs to host:port and drops
// icons entirely.
func printStartupBannerMinimal(cfg BannerConfig) {
	w := cfg.writer()
	fmt.Fprintf(w, "%s %s\n", cfg.AppName, cfg.Version)
	for _, url := range cfg.URLs {
		fmt.Fprintln(w, ExtractHostPort(url))
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Fprintln(w, cfg.SetupToken)
	}
}

// printStartupBannerMicro writes a single line for very narrow
// terminals.
func printStartupBannerMicro(cfg BannerConfig) {
	w := cfg.writer()
	if len(cfg.URLs) > 0 {
		fmt.Fprintf(w, "%s %s\n", cfg.AppName, ExtractHostPort(cfg.URLs[0]))
		return
	}
	fmt.Fprintln(w, cfg.AppName)
}

// ExtractHostPort reduces a URL to its host and port, which is all a
// narrow terminal has room for. Input that is not a URL is returned
// unchanged apart from any trailing slash.
func ExtractHostPort(url string) string {
	s := url
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+len("://"):]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return url
	}
	return s
}
