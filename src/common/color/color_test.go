package color

import "testing"

// boolPtr returns a pointer to b for the force-override table cases.
func boolPtr(b bool) *bool {
	return &b
}

func TestParseColorFlag(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    *bool
		wantErr bool
	}{
		{name: "empty is auto", value: "", want: nil},
		{name: "auto", value: "auto", want: nil},
		{name: "auto uppercase", value: "AUTO", want: nil},
		{name: "auto padded", value: "  auto  ", want: nil},
		{name: "yes", value: "yes", want: boolPtr(true)},
		{name: "true", value: "true", want: boolPtr(true)},
		{name: "always", value: "always", want: boolPtr(true)},
		{name: "on", value: "on", want: boolPtr(true)},
		{name: "one", value: "1", want: boolPtr(true)},
		{name: "yes mixed case", value: "YeS", want: boolPtr(true)},
		{name: "no", value: "no", want: boolPtr(false)},
		{name: "false", value: "false", want: boolPtr(false)},
		{name: "never", value: "never", want: boolPtr(false)},
		{name: "off", value: "off", want: boolPtr(false)},
		{name: "zero", value: "0", want: boolPtr(false)},
		{name: "invalid", value: "maybe", wantErr: true},
		{name: "invalid number", value: "2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseColorFlag(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseColorFlag(%q) error = nil, want an error", tt.value)
				}
				if got != nil {
					t.Errorf("ParseColorFlag(%q) = %v, want nil on error", tt.value, *got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseColorFlag(%q) error = %v, want nil", tt.value, err)
			}
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("ParseColorFlag(%q) = %v, want nil", tt.value, *got)
			case tt.want != nil && got == nil:
				t.Errorf("ParseColorFlag(%q) = nil, want %v", tt.value, *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Errorf("ParseColorFlag(%q) = %v, want %v", tt.value, *got, *tt.want)
			}
		})
	}
}

func TestColorEnabled(t *testing.T) {
	tests := []struct {
		name    string
		force   *bool
		noColor string
		term    string
		want    bool
	}{
		{name: "force on beats no color", force: boolPtr(true), noColor: "1", term: "dumb", want: true},
		{name: "force off", force: boolPtr(false), noColor: "", term: "xterm-256color", want: false},
		{name: "no color set", noColor: "1", term: "xterm-256color", want: false},
		{name: "dumb terminal", noColor: "", term: "dumb", want: false},
		{name: "auto without tty", noColor: "", term: "xterm-256color", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got := ColorEnabled(tt.force); got != tt.want {
				t.Errorf("ColorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmojiEnabled(t *testing.T) {
	tests := []struct {
		name    string
		force   *bool
		noColor string
		term    string
		want    bool
	}{
		{name: "config forces emojis on despite no color", force: boolPtr(true), noColor: "1", want: true},
		{name: "config forces emojis off", force: boolPtr(false), noColor: "", want: false},
		{name: "no color disables emojis", noColor: "yes", term: "xterm-256color", want: false},
		{name: "auto without tty", noColor: "", term: "xterm-256color", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("TERM", tt.term)
			if got := EmojiEnabled(tt.force); got != tt.want {
				t.Errorf("EmojiEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColorize(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		code    string
		input   string
		want    string
	}{
		{name: "disabled passes through", enabled: false, code: CodeRed, input: "boom", want: "boom"},
		{name: "enabled wraps", enabled: true, code: CodeRed, input: "boom", want: "\x1b[31mboom\x1b[0m"},
		{name: "enabled empty string", enabled: true, code: CodeGreen, input: "", want: "\x1b[32m\x1b[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Colorize(tt.enabled, tt.code, tt.input); got != tt.want {
				t.Errorf("Colorize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColorWrappers(t *testing.T) {
	tests := []struct {
		name string
		fn   func(bool, string) string
		want string
	}{
		{name: "red", fn: Red, want: "\x1b[31mx\x1b[0m"},
		{name: "green", fn: Green, want: "\x1b[32mx\x1b[0m"},
		{name: "yellow", fn: Yellow, want: "\x1b[33mx\x1b[0m"},
		{name: "blue", fn: Blue, want: "\x1b[34mx\x1b[0m"},
		{name: "magenta", fn: Magenta, want: "\x1b[35mx\x1b[0m"},
		{name: "cyan", fn: Cyan, want: "\x1b[36mx\x1b[0m"},
		{name: "gray", fn: Gray, want: "\x1b[90mx\x1b[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(true, "x"); got != tt.want {
				t.Errorf("%s(true, \"x\") = %q, want %q", tt.name, got, tt.want)
			}
			if got := tt.fn(false, "x"); got != "x" {
				t.Errorf("%s(false, \"x\") = %q, want %q", tt.name, got, "x")
			}
		})
	}
}

func TestBoldIgnoresColorSetting(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if got, want := Bold("x"), "\x1b[1mx\x1b[0m"; got != want {
		t.Errorf("Bold() = %q, want %q", got, want)
	}
}

func TestStatusPrefixes(t *testing.T) {
	tests := []struct {
		name      string
		fn        func(bool) string
		wantEmoji string
		wantPlain string
	}{
		{name: "ok", fn: StatusOK, wantEmoji: "✅", wantPlain: "[OK]"},
		{name: "error", fn: StatusError, wantEmoji: "❌", wantPlain: "[ERROR]"},
		{name: "warn", fn: StatusWarn, wantEmoji: "⚠️", wantPlain: "[WARN]"},
		{name: "info", fn: StatusInfo, wantEmoji: "ℹ️", wantPlain: "[INFO]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(true); got != tt.wantEmoji {
				t.Errorf("%s(true) = %q, want %q", tt.name, got, tt.wantEmoji)
			}
			if got := tt.fn(false); got != tt.wantPlain {
				t.Errorf("%s(false) = %q, want %q", tt.name, got, tt.wantPlain)
			}
		})
	}
}
