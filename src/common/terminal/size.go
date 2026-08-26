// Package terminal implements AI.md PART 7 "Terminal Size Modes": terminal
// dimension detection and the size-mode thresholds that drive layout
// decisions for every binary (server, CLI, agent).
package terminal

import (
	"os"
	"strconv"
)

// SizeMode classifies the terminal by its smallest usable dimension.
type SizeMode int

const (
	// SizeMicro is under 40 columns or under 10 rows.
	SizeMicro SizeMode = iota
	// SizeMinimal is 40-59 columns or 10-15 rows.
	SizeMinimal
	// SizeCompact is 60-79 columns or 16-23 rows.
	SizeCompact
	// SizeStandard is 80-119 columns and 24-39 rows.
	SizeStandard
	// SizeWide is 120-199 columns and 40-59 rows.
	SizeWide
	// SizeUltrawide is 200-399 columns and 60-79 rows.
	SizeUltrawide
	// SizeMassive is 400+ columns and 80+ rows.
	SizeMassive
)

// AI.md spells these constants SizeModeXxx in its code samples; the aliases
// keep both spellings valid so downstream parts compile against either name.
const (
	// SizeModeMicro is an alias for SizeMicro.
	SizeModeMicro = SizeMicro
	// SizeModeMinimal is an alias for SizeMinimal.
	SizeModeMinimal = SizeMinimal
	// SizeModeCompact is an alias for SizeCompact.
	SizeModeCompact = SizeCompact
	// SizeModeStandard is an alias for SizeStandard.
	SizeModeStandard = SizeStandard
	// SizeModeWide is an alias for SizeWide.
	SizeModeWide = SizeWide
	// SizeModeUltrawide is an alias for SizeUltrawide.
	SizeModeUltrawide = SizeUltrawide
	// SizeModeMassive is an alias for SizeMassive.
	SizeModeMassive = SizeMassive
)

// Default terminal dimensions used when neither the terminal nor the
// environment reports a usable size.
const (
	// DefaultCols is the fallback column count.
	DefaultCols = 80
	// DefaultRows is the fallback row count.
	DefaultRows = 24
)

// String returns the lowercase mode name.
func (m SizeMode) String() string {
	switch m {
	case SizeMicro:
		return "micro"
	case SizeMinimal:
		return "minimal"
	case SizeCompact:
		return "compact"
	case SizeStandard:
		return "standard"
	case SizeWide:
		return "wide"
	case SizeUltrawide:
		return "ultrawide"
	case SizeMassive:
		return "massive"
	}
	return "unknown"
}

// ShowASCIIArt reports whether ASCII art banners fit this mode.
func (m SizeMode) ShowASCIIArt() bool {
	return m >= SizeStandard
}

// ShowBorders reports whether box-drawing borders fit this mode.
func (m SizeMode) ShowBorders() bool {
	return m >= SizeCompact
}

// ShowSidebar reports whether a sidebar column fits this mode.
func (m SizeMode) ShowSidebar() bool {
	return m >= SizeWide
}

// ShowIcons reports whether inline icons fit this mode.
func (m SizeMode) ShowIcons() bool {
	return m >= SizeMinimal
}

// TerminalSize is the detected terminal geometry and its derived size mode.
type TerminalSize struct {
	Cols int
	Rows int
	Mode SizeMode
}

// ShowASCIIArt reports whether ASCII art banners fit this terminal.
func (t TerminalSize) ShowASCIIArt() bool {
	return t.Mode.ShowASCIIArt()
}

// ShowBorders reports whether box-drawing borders fit this terminal.
func (t TerminalSize) ShowBorders() bool {
	return t.Mode.ShowBorders()
}

// ShowSidebar reports whether a sidebar column fits this terminal.
func (t TerminalSize) ShowSidebar() bool {
	return t.Mode.ShowSidebar()
}

// ShowIcons reports whether inline icons fit this terminal.
func (t TerminalSize) ShowIcons() bool {
	return t.Mode.ShowIcons()
}

// GetTerminalSize returns the current terminal geometry. It queries the real
// terminal first, then falls back to the COLUMNS and LINES environment
// variables, then to the 80x24 default.
func GetTerminalSize() TerminalSize {
	cols, rows, ok := querySize()
	if !ok {
		cols, rows = 0, 0
	}
	if cols <= 0 {
		cols = envDimension("COLUMNS")
	}
	if rows <= 0 {
		rows = envDimension("LINES")
	}
	if cols <= 0 {
		cols = DefaultCols
	}
	if rows <= 0 {
		rows = DefaultRows
	}
	return TerminalSize{
		Cols: cols,
		Rows: rows,
		Mode: calculateMode(cols, rows),
	}
}

// envDimension reads a positive integer dimension from an environment variable, 0 when unusable.
func envDimension(name string) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// calculateMode maps dimensions to a size mode, smallest bucket first so the
// more constrained of the two dimensions decides the result.
func calculateMode(cols, rows int) SizeMode {
	switch {
	case cols < 40 || rows < 10:
		return SizeMicro
	case cols < 60 || rows < 16:
		return SizeMinimal
	case cols < 80 || rows < 24:
		return SizeCompact
	case cols < 120 || rows < 40:
		return SizeStandard
	case cols < 200 || rows < 60:
		return SizeWide
	case cols < 400 || rows < 80:
		return SizeUltrawide
	default:
		return SizeMassive
	}
}
