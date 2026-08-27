package config

import "testing"

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"true", "true", true},
		{"yes", "yes", true},
		{"on", "on", true},
		{"one", "1", true},
		{"enable", "enable", true},
		{"enabled", "enabled", true},
		{"mixed case", "YES", true},
		{"padded", "  yes  ", true},
		{"false", "false", false},
		{"no", "no", false},
		{"off", "off", false},
		{"zero", "0", false},
		{"disable", "disable", false},
		{"disabled", "disabled", false},
		{"none", "none", false},
		{"empty", "", false},
		{"garbage", "maybe", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTruthy(tt.in); got != tt.want {
				t.Errorf("IsTruthy(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsFalsey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"false", "false", true},
		{"no", "no", true},
		{"off", "off", true},
		{"zero", "0", true},
		{"disable", "disable", true},
		{"disabled", "disabled", true},
		{"none", "none", true},
		{"mixed case", "NO", true},
		{"true", "true", false},
		{"yes", "yes", false},
		{"garbage", "maybe", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFalsey(tt.in); got != tt.want {
				t.Errorf("IsFalsey(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantValue bool
		wantOK    bool
	}{
		{"truthy", "enabled", true, true},
		{"falsey", "disabled", false, true},
		{"unrecognized", "sometimes", false, false},
		{"empty", "", false, false},
		{"numeric truthy", "1", true, true},
		{"numeric falsey", "0", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := ParseBool(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ParseBool(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if ok && value != tt.wantValue {
				t.Errorf("ParseBool(%q) value = %v, want %v", tt.in, value, tt.wantValue)
			}
		})
	}
}
