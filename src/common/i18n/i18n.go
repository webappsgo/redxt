// Package i18n implements the string-table translation engine required
// by AI.md PART 31. It is shared by every binary (redxt, redxt-cli,
// redxt-agent) so all three present the same translations for the same
// language, per the spec's "no partial support" rule.
//
// Translation is literal `{variable}` substring replacement, never
// fmt.Sprintf — locale files must never contain printf-style
// placeholders. A missing key or an unsupported language always falls
// back to English rather than erroring, so a bad --lang flag or
// Accept-Language header can never crash a binary.
package i18n

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed locales/*.json
var localeFS embed.FS

// DefaultLanguage is the language every fallback chain ends at.
const DefaultLanguage = "en"

// languageMeta describes one supported language's display metadata.
type languageMeta struct {
	nativeName string
	direction  string
}

// supported lists every language ALL binaries serve, per PART 31 "no
// partial support". Order here also fixes the deterministic order
// Supported() returns.
var supported = []string{"en", "es", "zh", "fr", "ar", "de", "ja"}

var meta = map[string]languageMeta{
	"en": {nativeName: "English", direction: "ltr"},
	"es": {nativeName: "Español", direction: "ltr"},
	"zh": {nativeName: "中文", direction: "ltr"},
	"fr": {nativeName: "Français", direction: "ltr"},
	"ar": {nativeName: "العربية", direction: "rtl"},
	"de": {nativeName: "Deutsch", direction: "ltr"},
	"ja": {nativeName: "日本語", direction: "ltr"},
}

// catalogs holds every language's flat dot-key -> string map, loaded
// once at package init from the embedded locale files.
var catalogs = loadCatalogs()

// loadCatalogs parses every embedded locales/*.json file. A locale
// file is part of the binary, not runtime input, so a parse failure
// here is a build-time bug and panics rather than degrading silently.
func loadCatalogs() map[string]map[string]string {
	out := make(map[string]map[string]string, len(supported))
	for _, lang := range supported {
		b, err := localeFS.ReadFile("locales/" + lang + ".json")
		if err != nil {
			panic("i18n: missing embedded locale file for " + lang + ": " + err.Error())
		}
		var catalog map[string]string
		if err := json.Unmarshal(b, &catalog); err != nil {
			panic("i18n: invalid JSON in locales/" + lang + ".json: " + err.Error())
		}
		out[lang] = catalog
	}
	return out
}

// Supported returns every supported language code in a fixed,
// deterministic order.
func Supported() []string {
	out := make([]string, len(supported))
	copy(out, supported)
	return out
}

// IsSupported reports whether lang is one of the languages ALL
// binaries serve.
func IsSupported(lang string) bool {
	_, ok := catalogs[lang]
	return ok
}

// Direction returns "rtl" or "ltr" for lang, falling back to the
// default language's direction for an unsupported code.
func Direction(lang string) string {
	if m, ok := meta[lang]; ok {
		return m.direction
	}
	return meta[DefaultLanguage].direction
}

// NativeName returns the language's own name for itself (e.g. "es" ->
// "Español"), falling back to the code itself when unknown.
func NativeName(lang string) string {
	if m, ok := meta[lang]; ok {
		return m.nativeName
	}
	return lang
}

// resolve picks the language to actually translate with: lang itself
// if supported, else DefaultLanguage. This is the "unsupported
// language fallback" rule from PART 31 — never an error.
func resolve(lang string) string {
	if IsSupported(lang) {
		return lang
	}
	return DefaultLanguage
}

// lookup returns the raw catalog string for key in lang, falling back
// to English on a missing key, per PART 31 "missing key fallback".
// The key itself is the last-resort fallback, so a not-yet-translated
// key at least renders something legible instead of an empty string.
func lookup(lang, key string) (string, bool) {
	lang = resolve(lang)
	if v, ok := catalogs[lang][key]; ok {
		return v, true
	}
	if lang != DefaultLanguage {
		if v, ok := catalogs[DefaultLanguage][key]; ok {
			return v, true
		}
	}
	return key, false
}

// Translate returns the string for key in lang.
func Translate(lang, key string) string {
	v, _ := lookup(lang, key)
	return v
}

// TranslateFormat returns the string for key in lang with every
// `{name}` placeholder replaced by vars["name"]. Replacement is
// literal string substitution, never fmt.Sprintf, per PART 31.
func TranslateFormat(lang, key string, vars map[string]string) string {
	v, _ := lookup(lang, key)
	for name, val := range vars {
		v = strings.ReplaceAll(v, "{"+name+"}", val)
	}
	return v
}

// PluralCategory returns the CLDR plural category for n in lang, per
// the PART 31 "Supported Languages" table:
//
//	en, es, de: one, other
//	fr:         one (0 and 1), other
//	zh, ja:     other only (no plural forms)
//	ar:         zero, one, two, few, many, other
func PluralCategory(lang string, n int) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch resolve(lang) {
	case "ar":
		mod100 := abs % 100
		switch {
		case abs == 0:
			return "zero"
		case abs == 1:
			return "one"
		case abs == 2:
			return "two"
		case mod100 >= 3 && mod100 <= 10:
			return "few"
		case mod100 >= 11 && mod100 <= 99:
			return "many"
		default:
			return "other"
		}
	case "fr":
		if abs == 0 || abs == 1 {
			return "one"
		}
		return "other"
	case "zh", "ja":
		return "other"
	default: // en, es, de
		if abs == 1 {
			return "one"
		}
		return "other"
	}
}

// TranslatePlural returns the string for key.<category> in lang, where
// category is the CLDR plural category for n, falling back through
// "other" and then English if the specific category is absent. vars
// additionally receives "count" set to n's decimal string unless the
// caller already supplied one.
func TranslatePlural(lang, key string, n int, vars map[string]string) string {
	if vars == nil {
		vars = map[string]string{}
	}
	if _, ok := vars["count"]; !ok {
		vars = cloneWithCount(vars, n)
	}

	category := PluralCategory(lang, n)
	resolved := resolve(lang)
	pluralKey := key + "." + category
	if v, ok := catalogs[resolved][pluralKey]; ok {
		return substitute(v, vars)
	}
	if v, ok := catalogs[resolved][key+".other"]; ok {
		return substitute(v, vars)
	}
	if resolved != DefaultLanguage {
		if v, ok := catalogs[DefaultLanguage][pluralKey]; ok {
			return substitute(v, vars)
		}
		if v, ok := catalogs[DefaultLanguage][key+".other"]; ok {
			return substitute(v, vars)
		}
	}
	return pluralKey
}

// cloneWithCount copies vars and sets "count" to n's decimal string.
func cloneWithCount(vars map[string]string, n int) map[string]string {
	out := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		out[k] = v
	}
	out["count"] = itoa(n)
	return out
}

// substitute applies literal `{name}` replacement for every var.
func substitute(s string, vars map[string]string) string {
	for name, val := range vars {
		s = strings.ReplaceAll(s, "{"+name+"}", val)
	}
	return s
}

// itoa avoids pulling in strconv for a single call site's worth of use
// beyond what the rest of this file already needs; kept trivial and
// allocation-light for the hot template-render path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Keys returns every translation key defined in English, sorted. It
// exists for build-time/test-time key-parity validation across
// locales (PART 31 "Key validation").
func Keys() []string {
	keys := make([]string, 0, len(catalogs[DefaultLanguage]))
	for k := range catalogs[DefaultLanguage] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MissingKeys returns every English key absent from lang's catalog,
// for the same build-time key-parity check. An unsupported lang
// returns nil rather than every key, since there is nothing
// meaningful to report for a language redxt does not ship.
func MissingKeys(lang string) []string {
	if !IsSupported(lang) {
		return nil
	}
	var missing []string
	for _, k := range Keys() {
		if _, ok := catalogs[lang][k]; !ok {
			missing = append(missing, k)
		}
	}
	return missing
}
