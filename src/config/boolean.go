package config

import "strings"

// truthyValues and falseyValues implement the PART 5 "Boolean Handling"
// table shared by the server, redxt-cli, and redxt-agent: every flag,
// environment variable, and config file value that accepts a boolean
// goes through this table instead of strconv.ParseBool, so "yes"/"on"/
// "enabled" and their opposites are always understood.
var (
	truthyValues = map[string]bool{
		"true": true, "yes": true, "on": true, "1": true,
		"enable": true, "enabled": true,
	}
	falseyValues = map[string]bool{
		"false": true, "no": true, "off": true, "0": true,
		"disable": true, "disabled": true, "none": true,
	}
)

// IsTruthy reports whether s is one of the recognized truthy spellings,
// case-insensitively and ignoring surrounding whitespace.
func IsTruthy(s string) bool {
	return truthyValues[strings.ToLower(strings.TrimSpace(s))]
}

// IsFalsey reports whether s is one of the recognized falsey spellings,
// case-insensitively and ignoring surrounding whitespace.
func IsFalsey(s string) bool {
	return falseyValues[strings.ToLower(strings.TrimSpace(s))]
}

// ParseBool resolves s against the truthy/falsey table. ok is false when
// s matches neither list, in which case value is meaningless and the
// caller should fall back to its own default.
func ParseBool(s string) (value bool, ok bool) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if truthyValues[trimmed] {
		return true, true
	}
	if falseyValues[trimmed] {
		return false, true
	}
	return false, false
}
