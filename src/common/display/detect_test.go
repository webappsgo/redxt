package display

import "testing"

func TestAutoDetectDisplayMode(t *testing.T) {
	tests := []struct {
		name string
		env  DisplayEnv
		want DisplayMode
	}{
		{
			name: "dumb terminal forces cli",
			env:  DisplayEnv{TerminalType: "dumb", IsTerminal: true, HasDisplay: true},
			want: DisplayModeCLI,
		},
		{
			name: "no terminal and no display is headless",
			env:  DisplayEnv{TerminalType: "xterm-256color"},
			want: DisplayModeHeadless,
		},
		{
			name: "local display is gui",
			env:  DisplayEnv{TerminalType: "xterm-256color", HasDisplay: true, DisplayType: "wayland"},
			want: DisplayModeGUI,
		},
		{
			name: "local display with terminal is gui",
			env:  DisplayEnv{TerminalType: "xterm", HasDisplay: true, DisplayType: "x11", IsTerminal: true},
			want: DisplayModeGUI,
		},
		{
			name: "ssh with forwarded display is tui",
			env: DisplayEnv{
				TerminalType: "xterm",
				HasDisplay:   true,
				DisplayType:  "x11",
				IsTerminal:   true,
				IsSSH:        true,
			},
			want: DisplayModeTUI,
		},
		{
			name: "mosh with display is tui",
			env: DisplayEnv{
				TerminalType: "xterm",
				HasDisplay:   true,
				DisplayType:  "x11",
				IsTerminal:   true,
				IsMosh:       true,
			},
			want: DisplayModeTUI,
		},
		{
			name: "ssh without terminal but with display is cli",
			env:  DisplayEnv{TerminalType: "xterm", HasDisplay: true, DisplayType: "x11", IsSSH: true},
			want: DisplayModeCLI,
		},
		{
			name: "plain terminal is tui",
			env:  DisplayEnv{TerminalType: "screen-256color", IsTerminal: true, IsScreen: true},
			want: DisplayModeTUI,
		},
		{
			name: "empty environment is headless",
			env:  DisplayEnv{},
			want: DisplayModeHeadless,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoDetectDisplayMode(tt.env); got != tt.want {
				t.Errorf("autoDetectDisplayMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanUseANSI(t *testing.T) {
	tests := []struct {
		name    string
		term    string
		noColor string
		env     DisplayEnv
		want    bool
	}{
		{
			name: "interactive terminal",
			term: "xterm-256color",
			env:  DisplayEnv{TerminalType: "xterm-256color", IsTerminal: true},
			want: true,
		},
		{
			name: "dumb terminal",
			term: "dumb",
			env:  DisplayEnv{TerminalType: "dumb", IsTerminal: true},
			want: false,
		},
		{
			name:    "no color set",
			term:    "xterm-256color",
			noColor: "1",
			env:     DisplayEnv{TerminalType: "xterm-256color", IsTerminal: true},
			want:    false,
		},
		{
			name:    "no color empty is ignored",
			term:    "xterm-256color",
			noColor: "",
			env:     DisplayEnv{TerminalType: "xterm-256color", IsTerminal: true},
			want:    true,
		},
		{
			name: "piped output",
			term: "xterm-256color",
			env:  DisplayEnv{TerminalType: "xterm-256color"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM", tt.term)
			t.Setenv("NO_COLOR", tt.noColor)
			if got := CanUseANSI(tt.env); got != tt.want {
				t.Errorf("CanUseANSI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDumbTerminal(t *testing.T) {
	tests := []struct {
		name string
		term string
		want bool
	}{
		{name: "dumb", term: "dumb", want: true},
		{name: "xterm", term: "xterm-256color", want: false},
		{name: "unset", term: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM", tt.term)
			if got := IsDumbTerminal(); got != tt.want {
				t.Errorf("IsDumbTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDisplayModeString(t *testing.T) {
	tests := []struct {
		name string
		mode DisplayMode
		want string
	}{
		{name: "headless", mode: DisplayModeHeadless, want: "headless"},
		{name: "cli", mode: DisplayModeCLI, want: "cli"},
		{name: "tui", mode: DisplayModeTUI, want: "tui"},
		{name: "gui", mode: DisplayModeGUI, want: "gui"},
		{name: "out of range", mode: DisplayMode(42), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("DisplayMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectDisplayEnvIsConsistent(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	env := DetectDisplayEnv()
	if env.TerminalType != "xterm-256color" {
		t.Errorf("TerminalType = %q, want %q", env.TerminalType, "xterm-256color")
	}
	if env.Cols <= 0 || env.Rows <= 0 {
		t.Errorf("Cols/Rows = %dx%d, want positive dimensions", env.Cols, env.Rows)
	}
	if got := autoDetectDisplayMode(env); got != env.Mode {
		t.Errorf("Mode = %v, want %v", env.Mode, got)
	}
	if env.Mode.String() == "unknown" {
		t.Errorf("Mode.String() = %q, want a known mode name", env.Mode.String())
	}
}
