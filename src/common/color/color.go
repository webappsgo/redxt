// Package color implements AI.md PART 8 "NO_COLOR Support" and the PART 11
// emoji fallback table. Resolution priority is: CLI flag, config value,
// NO_COLOR environment variable, then TTY and TERM auto-detection.
package color

import (
	"fmt"
	"os"
	"strings"

	"github.com/webappsgo/redxt/src/common/display"
)

// ANSI SGR codes used by the color wrappers.
const (
	// CodeRed is the ANSI code for red foreground text.
	CodeRed = "31"
	// CodeGreen is the ANSI code for green foreground text.
	CodeGreen = "32"
	// CodeYellow is the ANSI code for yellow foreground text.
	CodeYellow = "33"
	// CodeBlue is the ANSI code for blue foreground text.
	CodeBlue = "34"
	// CodeMagenta is the ANSI code for magenta foreground text.
	CodeMagenta = "35"
	// CodeCyan is the ANSI code for cyan foreground text.
	CodeCyan = "36"
	// CodeGray is the ANSI code for bright black (gray) foreground text.
	CodeGray = "90"
	// CodeBold is the ANSI code for bold text.
	CodeBold = "1"
	// CodeReset is the ANSI code that clears all attributes.
	CodeReset = "0"
)

// ColorEnabled resolves whether ANSI colors should be emitted. A non-nil
// forceColor is the resolved CLI flag or config value and wins over
// everything else; otherwise NO_COLOR, then auto-detection, decide.
func ColorEnabled(forceColor *bool) bool {
	if forceColor != nil {
		return *forceColor
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return display.CanUseANSI(display.DetectDisplayEnv())
}

// EmojiEnabled resolves whether emojis should be emitted. A non-nil
// forceEmoji is the `output.emoji` config override, which keeps emojis on
// even when NO_COLOR is set; otherwise the color rules apply unchanged.
func EmojiEnabled(forceEmoji *bool) bool {
	if forceEmoji != nil {
		return *forceEmoji
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return display.CanUseANSI(display.DetectDisplayEnv())
}

// ParseColorFlag maps a --color {auto|yes|no} flag value to an override
// pointer. "auto" and the empty value yield nil, meaning "decide later".
func ParseColorFlag(v string) (*bool, error) {
	yes, no := true, false
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto":
		return nil, nil
	case "yes", "true", "always", "on", "1":
		return &yes, nil
	case "no", "false", "never", "off", "0":
		return &no, nil
	}
	return nil, fmt.Errorf("invalid --color value %q (want auto, yes, or no)", v)
}

// StatusOK returns the success prefix, falling back to plain text when emojis are off.
func StatusOK(emoji bool) string {
	if emoji {
		return "✅"
	}
	return "[OK]"
}

// StatusError returns the error prefix, falling back to plain text when emojis are off.
func StatusError(emoji bool) string {
	if emoji {
		return "❌"
	}
	return "[ERROR]"
}

// StatusWarn returns the warning prefix, falling back to plain text when emojis are off.
func StatusWarn(emoji bool) string {
	if emoji {
		return "⚠️"
	}
	return "[WARN]"
}

// StatusInfo returns the informational prefix, falling back to plain text when emojis are off.
func StatusInfo(emoji bool) string {
	if emoji {
		return "ℹ️"
	}
	return "[INFO]"
}

// Colorize wraps s in an ANSI SGR sequence when enabled, otherwise returns s unchanged.
func Colorize(enabled bool, code, s string) string {
	if !enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[" + CodeReset + "m"
}

// Red renders s in red when enabled.
func Red(enabled bool, s string) string {
	return Colorize(enabled, CodeRed, s)
}

// Green renders s in green when enabled.
func Green(enabled bool, s string) string {
	return Colorize(enabled, CodeGreen, s)
}

// Yellow renders s in yellow when enabled.
func Yellow(enabled bool, s string) string {
	return Colorize(enabled, CodeYellow, s)
}

// Blue renders s in blue when enabled.
func Blue(enabled bool, s string) string {
	return Colorize(enabled, CodeBlue, s)
}

// Cyan renders s in cyan when enabled.
func Cyan(enabled bool, s string) string {
	return Colorize(enabled, CodeCyan, s)
}

// Magenta renders s in magenta when enabled.
func Magenta(enabled bool, s string) string {
	return Colorize(enabled, CodeMagenta, s)
}

// Gray renders s in gray when enabled.
func Gray(enabled bool, s string) string {
	return Colorize(enabled, CodeGray, s)
}

// Bold renders s in bold and takes no enabled flag: NO_COLOR disables colors
// and emojis but never bold, underline or italic text styling (AI.md PART 8).
func Bold(s string) string {
	return "\x1b[" + CodeBold + "m" + s + "\x1b[" + CodeReset + "m"
}
