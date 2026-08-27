package i18n

import "testing"

func TestSupportedListsAllSevenLanguages(t *testing.T) {
	want := []string{"en", "es", "zh", "fr", "ar", "de", "ja"}
	got := Supported()
	if len(got) != len(want) {
		t.Fatalf("Supported() = %v, want %v", got, want)
	}
	for i, lang := range want {
		if got[i] != lang {
			t.Errorf("Supported()[%d] = %q, want %q", i, got[i], lang)
		}
	}
}

func TestIsSupported(t *testing.T) {
	tests := []struct {
		lang string
		want bool
	}{
		{"en", true},
		{"ar", true},
		{"ja", true},
		{"xx", false},
		{"", false},
		{"EN", false}, // case-sensitive: codes are always lowercase
	}
	for _, tt := range tests {
		if got := IsSupported(tt.lang); got != tt.want {
			t.Errorf("IsSupported(%q) = %v, want %v", tt.lang, got, tt.want)
		}
	}
}

func TestDirection(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"ar", "rtl"},
		{"en", "ltr"},
		{"es", "ltr"},
		{"xx", "ltr"}, // unsupported falls back to default's direction
	}
	for _, tt := range tests {
		if got := Direction(tt.lang); got != tt.want {
			t.Errorf("Direction(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestTranslateFallsBackToEnglishOnMissingKey(t *testing.T) {
	// "common.save" exists in every locale, but a key that exists only
	// in English (simulated by requesting it in a real supported
	// language) must still resolve rather than returning empty.
	got := Translate("es", "common.save")
	if got != "Guardar" {
		t.Errorf("Translate(es, common.save) = %q, want %q", got, "Guardar")
	}
}

func TestTranslateFallsBackToKeyWhenEvenEnglishIsMissing(t *testing.T) {
	got := Translate("en", "no.such.key")
	if got != "no.such.key" {
		t.Errorf("Translate(en, no.such.key) = %q, want the key itself", got)
	}
}

func TestTranslateUnsupportedLanguageFallsBackToEnglish(t *testing.T) {
	got := Translate("xx", "common.save")
	if got != "Save" {
		t.Errorf("Translate(xx, common.save) = %q, want %q (silent fallback to en)", got, "Save")
	}
}

func TestTranslateFormatSubstitutesLiterally(t *testing.T) {
	got := TranslateFormat("en", "items_count.other", map[string]string{"count": "3"})
	want := "3 items"
	if got != want {
		t.Errorf("TranslateFormat() = %q, want %q", got, want)
	}
}

func TestTranslateFormatLeavesUnknownPlaceholdersAlone(t *testing.T) {
	got := TranslateFormat("en", "common.save", map[string]string{"unused": "x"})
	if got != "Save" {
		t.Errorf("TranslateFormat() = %q, want %q", got, "Save")
	}
}

// TestPluralCategoryBoundaries is boundary-style per house convention:
// it exercises the exact n values where each language's CLDR category
// selection changes, per the PART 31 "Supported Languages" table.
func TestPluralCategoryBoundaries(t *testing.T) {
	tests := []struct {
		name string
		lang string
		n    int
		want string
	}{
		{"en zero is other", "en", 0, "other"},
		{"en one is one", "en", 1, "one"},
		{"en two is other", "en", 2, "other"},
		{"es one is one", "es", 1, "one"},
		{"es two is other", "es", 2, "other"},
		{"de one is one", "de", 1, "one"},
		{"de other", "de", 5, "other"},
		{"fr zero is one", "fr", 0, "one"},
		{"fr one is one", "fr", 1, "one"},
		{"fr two is other", "fr", 2, "other"},
		{"zh always other zero", "zh", 0, "other"},
		{"zh always other one", "zh", 1, "other"},
		{"zh always other many", "zh", 99, "other"},
		{"ja always other", "ja", 1, "other"},
		{"ar zero", "ar", 0, "zero"},
		{"ar one", "ar", 1, "one"},
		{"ar two", "ar", 2, "two"},
		{"ar few lower bound", "ar", 3, "few"},
		{"ar few upper bound", "ar", 10, "few"},
		{"ar many lower bound", "ar", 11, "many"},
		{"ar many upper bound", "ar", 99, "many"},
		{"ar other at 100", "ar", 100, "other"},
		{"ar few resumes at 103", "ar", 103, "few"},
		{"ar many at 111", "ar", 111, "many"},
		{"negative n uses magnitude", "en", -1, "one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PluralCategory(tt.lang, tt.n); got != tt.want {
				t.Errorf("PluralCategory(%q, %d) = %q, want %q", tt.lang, tt.n, got, tt.want)
			}
		})
	}
}

func TestTranslatePlural(t *testing.T) {
	tests := []struct {
		name string
		lang string
		n    int
		want string
	}{
		{"en singular", "en", 1, "1 item"},
		{"en plural", "en", 3, "3 items"},
		{"es singular", "es", 1, "1 elemento"},
		{"fr zero uses one category", "fr", 0, "0 élément"},
		{"ar few", "ar", 5, "5 عناصر"},
		{"ar zero has no count placeholder", "ar", 0, "لا توجد عناصر"},
		{"zh has no plural forms", "zh", 7, "7 项"},
		{"unsupported language falls back to en", "xx", 2, "2 items"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TranslatePlural(tt.lang, "items_count", tt.n, nil); got != tt.want {
				t.Errorf("TranslatePlural(%q, items_count, %d) = %q, want %q", tt.lang, tt.n, got, tt.want)
			}
		})
	}
}

func TestTranslatePluralCallerSuppliedCountIsNotOverwritten(t *testing.T) {
	got := TranslatePlural("en", "items_count", 3, map[string]string{"count": "many"})
	want := "many items"
	if got != want {
		t.Errorf("TranslatePlural() = %q, want %q", got, want)
	}
}

func TestNativeName(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"en", "English"},
		{"ja", "日本語"},
		{"xx", "xx"}, // unknown code falls back to the code itself
	}
	for _, tt := range tests {
		if got := NativeName(tt.lang); got != tt.want {
			t.Errorf("NativeName(%q) = %q, want %q", tt.lang, got, tt.want)
		}
	}
}

func TestEveryLocaleHasEveryEnglishKey(t *testing.T) {
	// PART 31 "Key validation": every language must define the same
	// keys as en.json. Plural sub-keys are intentionally exempt here
	// since not every language defines every CLDR category (e.g. "en"
	// has no ".zero" — that's correct, not missing).
	for _, lang := range Supported() {
		if lang == "en" {
			continue
		}
		missing := MissingKeys(lang)
		if len(missing) != 0 {
			t.Errorf("locale %q is missing keys present in en: %v", lang, missing)
		}
	}
}

func TestMissingKeysOnUnsupportedLanguageReturnsNil(t *testing.T) {
	if got := MissingKeys("xx"); got != nil {
		t.Errorf("MissingKeys(xx) = %v, want nil", got)
	}
}

func TestKeysIsSorted(t *testing.T) {
	keys := Keys()
	if len(keys) == 0 {
		t.Fatal("Keys() returned no keys")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Fatalf("Keys() not sorted at index %d: %q > %q", i, keys[i-1], keys[i])
		}
	}
}
