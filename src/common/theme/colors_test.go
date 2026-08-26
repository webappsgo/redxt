package theme

import "testing"

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "dark", in: "dark", want: ModeDark},
		{name: "light", in: "light", want: ModeLight},
		{name: "auto", in: "auto", want: ModeAuto},
		{name: "uppercase", in: "LIGHT", want: ModeLight},
		{name: "padded", in: "  auto  ", want: ModeAuto},
		{name: "empty falls back to dark", in: "", want: ModeDark},
		{name: "unknown falls back to dark", in: "solarized", want: ModeDark},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeMode(tt.in); got != tt.want {
				t.Fatalf("NormalizeMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetThemePalette(t *testing.T) {
	if got := GetThemePalette("light"); got != ThemePaletteLight {
		t.Fatalf("light palette = %+v", got)
	}
	if got := GetThemePalette("dark"); got != ThemePaletteDark {
		t.Fatalf("dark palette = %+v", got)
	}
	if got := GetThemePalette("nonsense"); got != ThemePaletteDark {
		t.Fatalf("unknown mode must fall back to dark, got %+v", got)
	}
}

func TestGetThemePaletteAutoFollowsBackground(t *testing.T) {
	t.Setenv("COLORFGBG", "0;15")
	if got := GetThemePalette("auto"); got != ThemePaletteLight {
		t.Fatal("auto on a light background must select the light palette")
	}
	t.Setenv("COLORFGBG", "15;0")
	if got := GetThemePalette("auto"); got != ThemePaletteDark {
		t.Fatal("auto on a dark background must select the dark palette")
	}
}

func TestGetTerminalPalette(t *testing.T) {
	if got := GetTerminalPalette("light"); got != TerminalPaletteLight {
		t.Fatalf("light terminal palette = %+v", got)
	}
	if got := GetTerminalPalette("dark"); got != TerminalPaletteDark {
		t.Fatalf("dark terminal palette = %+v", got)
	}
	t.Setenv("COLORFGBG", "0;7")
	if got := GetTerminalPalette("auto"); got != TerminalPaletteLight {
		t.Fatalf("auto terminal palette = %+v, want light", got)
	}
}

func TestColorFGBGIsDark(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "classic dark", in: "15;0", want: true},
		{name: "classic light", in: "0;15", want: false},
		{name: "bright black background", in: "7;8", want: true},
		{name: "three fields uses the last", in: "15;default;0", want: true},
		{name: "unset falls back to dark", in: "", want: true},
		{name: "unparseable falls back to dark", in: "15;default", want: true},
		{name: "light gray background", in: "0;7", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colorFGBGIsDark(tt.in); got != tt.want {
				t.Fatalf("colorFGBGIsDark(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestPalettesAreFullyPopulated guards against a role being added to the
// struct but left empty in one of the two palettes, which would render
// as an invalid CSS custom property.
func TestPalettesAreFullyPopulated(t *testing.T) {
	for _, p := range []ThemePalette{ThemePaletteDark, ThemePaletteLight} {
		for name, v := range map[string]string{
			"background": p.Background, "foreground": p.Foreground,
			"primary": p.Primary, "secondary": p.Secondary, "accent": p.Accent,
			"success": p.Success, "warning": p.Warning, "error": p.Error,
			"info": p.Info, "surface": p.Surface, "surface_alt": p.SurfaceAlt,
			"border": p.Border, "muted": p.Muted,
		} {
			if len(v) != 7 || v[0] != '#' {
				t.Fatalf("%s = %q, want a 7-character hex value", name, v)
			}
		}
	}
	for _, p := range []TerminalPalette{TerminalPaletteDark, TerminalPaletteLight} {
		for name, v := range map[string]string{
			"foreground": p.Foreground, "muted": p.Muted, "primary": p.Primary,
			"success": p.Success, "warning": p.Warning, "error": p.Error,
			"info": p.Info, "border": p.Border,
		} {
			if v == "" {
				t.Fatalf("terminal role %s is empty", name)
			}
		}
	}
}
