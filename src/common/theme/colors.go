// Package theme holds the single source of truth for redxt's color
// palette. The hex ThemePalette is consumed literally by the web CSS,
// Swagger, and GraphiQL; CLI and TUI output map the same semantic roles
// onto ANSI indices through TerminalPalette instead, because terminals
// render a user-configured 16/256-color set. See AI.md PART 16
// "Unified Color Palette".
package theme

import (
	"os"
	"strconv"
	"strings"
)

// Theme mode names accepted by GetThemePalette and GetTerminalPalette.
const (
	// ModeDark forces the dark palette.
	ModeDark = "dark"
	// ModeLight forces the light palette.
	ModeLight = "light"
	// ModeAuto resolves the palette from the environment.
	ModeAuto = "auto"
)

// ThemePalette is the literal hex palette used by every HTML surface.
type ThemePalette struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Surface    string `json:"surface"`
	SurfaceAlt string `json:"surface_alt"`
	Border     string `json:"border"`
	Muted      string `json:"muted"`
}

// ThemePaletteDark is the default palette; dark mode is the project default.
var ThemePaletteDark = ThemePalette{
	Background: "#282a36", Foreground: "#f8f8f2",
	Primary: "#bd93f9", Secondary: "#50fa7b", Accent: "#ff79c6",
	Success: "#50fa7b", Warning: "#ffb86c", Error: "#ff5555", Info: "#8be9fd",
	Surface: "#2b2d3a", SurfaceAlt: "#21222c", Border: "#44475a", Muted: "#6272a4",
}

// ThemePaletteLight is the light-mode palette.
var ThemePaletteLight = ThemePalette{
	Background: "#ffffff", Foreground: "#1f2328",
	Primary: "#0969da", Secondary: "#1a7f37", Accent: "#8250df",
	Success: "#1a7f37", Warning: "#9a6700", Error: "#d1242f", Info: "#0969da",
	Surface: "#f6f8fa", SurfaceAlt: "#eff2f5", Border: "#d1d9e0", Muted: "#59636e",
}

// TerminalPalette holds ANSI 16-color indices (0-15) for CLI and TUI
// output. It is never the literal hex palette: both lipgloss.Color and
// the ESC[38;5;{n}m escape accept these indices directly.
type TerminalPalette struct {
	Foreground string `json:"foreground"`
	Muted      string `json:"muted"`
	Primary    string `json:"primary"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Border     string `json:"border"`
}

// TerminalPaletteDark maps the semantic roles onto bright ANSI colors.
var TerminalPaletteDark = TerminalPalette{
	Foreground: "15", Muted: "7", Primary: "13",
	Success: "10", Warning: "11", Error: "9", Info: "12", Border: "13",
}

// TerminalPaletteLight maps the semantic roles onto standard ANSI colors.
var TerminalPaletteLight = TerminalPalette{
	Foreground: "0", Muted: "8", Primary: "4",
	Success: "2", Warning: "3", Error: "1", Info: "4", Border: "4",
}

// NormalizeMode maps any user-supplied theme name onto one of the three
// supported modes. Anything unrecognized resolves to dark, which is the
// documented fallback when detection fails.
func NormalizeMode(themeMode string) string {
	switch strings.ToLower(strings.TrimSpace(themeMode)) {
	case ModeLight:
		return ModeLight
	case ModeAuto:
		return ModeAuto
	default:
		return ModeDark
	}
}

// GetThemePalette returns the hex palette for a theme mode. "auto"
// resolves through IsSystemDarkTheme.
func GetThemePalette(themeMode string) ThemePalette {
	switch NormalizeMode(themeMode) {
	case ModeLight:
		return ThemePaletteLight
	case ModeAuto:
		if IsSystemDarkTheme() {
			return ThemePaletteDark
		}
		return ThemePaletteLight
	default:
		return ThemePaletteDark
	}
}

// GetTerminalPalette returns the ANSI palette for a theme mode, using
// the same resolution rules as GetThemePalette.
func GetTerminalPalette(themeMode string) TerminalPalette {
	switch NormalizeMode(themeMode) {
	case ModeLight:
		return TerminalPaletteLight
	case ModeAuto:
		if IsSystemDarkTheme() {
			return TerminalPaletteDark
		}
		return TerminalPaletteLight
	default:
		return TerminalPaletteDark
	}
}

// IsSystemDarkTheme reports whether the surrounding terminal appears to
// be dark. Detection is COLORFGBG based (the only signal available to a
// headless server process) and falls back to dark when the variable is
// absent or unparseable.
func IsSystemDarkTheme() bool {
	return colorFGBGIsDark(os.Getenv("COLORFGBG"))
}

// colorFGBGIsDark interprets a COLORFGBG value such as "15;0". The last
// field is the background color index; indices 0-6 and 8 are the dark
// ANSI backgrounds.
func colorFGBGIsDark(v string) bool {
	fields := strings.Split(strings.TrimSpace(v), ";")
	last := strings.TrimSpace(fields[len(fields)-1])
	if last == "" {
		return true
	}
	n, err := strconv.Atoi(last)
	if err != nil {
		return true
	}
	return n <= 6 || n == 8
}
