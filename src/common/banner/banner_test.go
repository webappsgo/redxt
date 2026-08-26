package banner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/common/terminal"
)

// testConfig returns a banner config wired to an in-memory writer.
func testConfig(buf *bytes.Buffer) BannerConfig {
	return BannerConfig{
		AppName: "redxt",
		Version: "1.2.3",
		AppMode: "production",
		URLs:    []string{"http://127.0.0.1:8053/", "https://dns.example.com/admin"},
		Out:     buf,
	}
}

func TestPrintStartupBannerPlainHasNoEmoji(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(&buf)
	off := false
	cfg.ForceEmoji = &off
	cfg.ForceColor = &off
	PrintStartupBanner(cfg)

	out := buf.String()
	for _, icon := range []string{iconApp, iconProduction, iconURL, iconSetup} {
		if strings.Contains(out, icon) {
			t.Fatalf("plain banner contains %q:\n%s", icon, out)
		}
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("plain banner contains an ANSI escape:\n%s", out)
	}
	if !strings.Contains(out, "redxt v1.2.3") {
		t.Fatalf("plain banner missing name and version:\n%s", out)
	}
	for _, url := range cfg.URLs {
		if !strings.Contains(out, url) {
			t.Fatalf("plain banner missing %q:\n%s", url, out)
		}
	}
}

func TestPlainBannerShowsSetupTokenOnlyWhenRequested(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(&buf)
	cfg.SetupToken = "setup-token-value"
	printStartupBannerPlain(cfg)
	if strings.Contains(buf.String(), cfg.SetupToken) {
		t.Fatal("setup token printed with ShowSetup false")
	}

	buf.Reset()
	cfg.ShowSetup = true
	printStartupBannerPlain(cfg)
	if !strings.Contains(buf.String(), cfg.SetupToken) {
		t.Fatal("setup token missing with ShowSetup true")
	}
}

func TestBannerVariantsShrinkWithTerminal(t *testing.T) {
	var full, compact, minimal, micro bytes.Buffer

	printStartupBannerFull(testConfig(&full), terminal.TerminalSize{Cols: 100, Rows: 30, Mode: terminal.SizeStandard})
	printStartupBannerCompact(testConfig(&compact))
	printStartupBannerMinimal(testConfig(&minimal))
	printStartupBannerMicro(testConfig(&micro))

	if !strings.Contains(full.String(), "#") {
		t.Fatalf("full banner lost its ASCII art:\n%s", full.String())
	}
	if strings.Contains(compact.String(), "#") {
		t.Fatalf("compact banner must drop the ASCII art:\n%s", compact.String())
	}
	if !strings.Contains(compact.String(), iconApp) {
		t.Fatalf("compact banner must keep icons:\n%s", compact.String())
	}
	if strings.Contains(minimal.String(), iconApp) {
		t.Fatalf("minimal banner must drop icons:\n%s", minimal.String())
	}
	if !strings.Contains(minimal.String(), "127.0.0.1:8053") || strings.Contains(minimal.String(), "http://") {
		t.Fatalf("minimal banner must abbreviate URLs:\n%s", minimal.String())
	}
	if got := strings.TrimSpace(micro.String()); got != "redxt 127.0.0.1:8053" {
		t.Fatalf("micro banner = %q, want a single line", got)
	}
}

func TestMicroBannerWithoutURLs(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(&buf)
	cfg.URLs = nil
	printStartupBannerMicro(cfg)
	if got := strings.TrimSpace(buf.String()); got != "redxt" {
		t.Fatalf("micro banner = %q, want just the app name", got)
	}
}

func TestModeIcon(t *testing.T) {
	tests := []struct {
		name  string
		cfg   BannerConfig
		want  string
		label string
	}{
		{name: "production", cfg: BannerConfig{AppMode: "production"}, want: iconProduction},
		{name: "development", cfg: BannerConfig{AppMode: "development"}, want: iconDevelopment},
		{name: "case insensitive", cfg: BannerConfig{AppMode: "Development"}, want: iconDevelopment},
		{name: "debug wins", cfg: BannerConfig{AppMode: "production", Debug: true}, want: iconDebug},
		{name: "unknown mode is treated as production", cfg: BannerConfig{AppMode: "staging"}, want: iconProduction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.modeIcon(); got != tt.want {
				t.Fatalf("modeIcon() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "http with path", in: "http://127.0.0.1:8053/admin", want: "127.0.0.1:8053"},
		{name: "https root", in: "https://dns.example.com/", want: "dns.example.com"},
		{name: "no scheme", in: "dns.example.com:8053", want: "dns.example.com:8053"},
		{name: "query stripped", in: "http://a.example.com?x=1", want: "a.example.com"},
		{name: "fragment stripped", in: "http://a.example.com#top", want: "a.example.com"},
		{name: "ipv6", in: "http://[::1]:8053/", want: "[::1]:8053"},
		{name: "scheme only", in: "http://", want: "http://"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractHostPort(tt.in); got != tt.want {
				t.Fatalf("ExtractHostPort(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestBannerStripsControlCharacters guards the white-label path: an
// application name is operator-supplied and must not be able to inject
// escape sequences or extra lines into the console.
func TestBannerStripsControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	cfg := testConfig(&buf)
	cfg.AppName = "red\x1b[31mxt\nfake"
	cfg.Version = "1.0\x00"
	cfg.URLs = []string{"http://a.example.com\nhttp://evil.example.com"}
	off := false
	cfg.ForceEmoji = &off
	cfg.ForceColor = &off
	PrintStartupBanner(cfg)

	out := buf.String()
	if strings.ContainsAny(out, "\x1b\x00") {
		t.Fatalf("control characters survived:\n%q", out)
	}
	urlLines := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "http://") {
			urlLines++
		}
	}
	if urlLines != 1 {
		t.Fatalf("URL was split across %d lines:\n%q", urlLines, out)
	}
}

func TestASCIIArt(t *testing.T) {
	art := ASCIIArt("redxt")
	lines := strings.Split(art, "\n")
	if len(lines) != glyphRows {
		t.Fatalf("art has %d rows, want %d:\n%s", len(lines), glyphRows, art)
	}
	for _, l := range lines {
		if len(l) > maxArtColumns {
			t.Fatalf("art row is %d columns, over the %d limit", len(l), maxArtColumns)
		}
	}
	if strings.TrimSpace(art) == "" {
		t.Fatal("art is blank")
	}
}

func TestASCIIArtFallsBackToPlainText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "unsupported rune", in: "réd", want: "RÉD"},
		{name: "too wide", in: "a-very-long-white-label-application-name", want: "A-VERY-LONG-WHITE-LABEL-APPLICATION-NAME"},
		{name: "control character", in: "red\x1bxt-with-a-long-enough-name-to-be-plain", want: "REDXT-WITH-A-LONG-ENOUGH-NAME-TO-BE-PLAIN"},
		{name: "empty", in: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ASCIIArt(tt.in); got != tt.want {
				t.Fatalf("ASCIIArt(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestBlockFontIsRectangular catches a mistyped glyph, which would
// otherwise shear every row of the banner.
func TestBlockFontIsRectangular(t *testing.T) {
	for r, g := range blockFont {
		for i, row := range g {
			if len(row) != glyphWidth {
				t.Fatalf("glyph %q row %d is %d wide, want %d", string(r), i, len(row), glyphWidth)
			}
		}
	}
}
