package notify

import (
	"path/filepath"
	"testing"
)

func TestDefaultRawAllTemplatesParse(t *testing.T) {
	for _, name := range TemplateNames {
		name := name
		t.Run(name, func(t *testing.T) {
			raw, err := DefaultRaw(name)
			if err != nil {
				t.Fatalf("DefaultRaw(%q) error: %v", name, err)
			}
			tmpl, err := ParseTemplate(raw)
			if err != nil {
				t.Fatalf("ParseTemplate(%q) error: %v", name, err)
			}
			res := Validate(name, tmpl)
			if !res.OK() {
				t.Errorf("Validate(%q) errors: %v", name, res.Errors)
			}
		})
	}
}

func TestDefaultRawUnknown(t *testing.T) {
	if _, err := DefaultRaw("does_not_exist"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}

func TestLoadFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir, "test")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want, _ := DefaultRaw("test")
	wantTmpl, _ := ParseTemplate(want)
	if got.Subject != wantTmpl.Subject {
		t.Errorf("Subject = %q, want %q", got.Subject, wantTmpl.Subject)
	}
}

func TestLoadUnknownTemplate(t *testing.T) {
	if _, err := Load(t.TempDir(), "nope"); err == nil {
		t.Fatal("expected error for unknown template name")
	}
}

func TestSaveHasResetOverride(t *testing.T) {
	dir := t.TempDir()
	name := "test"

	if HasOverride(dir, name) {
		t.Fatal("HasOverride should be false before any save")
	}

	custom := Template{Subject: "Custom subject", Body: "custom body\n"}
	if err := SaveOverride(dir, name, custom); err != nil {
		t.Fatalf("SaveOverride() error: %v", err)
	}
	if !HasOverride(dir, name) {
		t.Fatal("HasOverride should be true after save")
	}

	got, err := Load(dir, name)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.Subject != custom.Subject {
		t.Errorf("Subject = %q, want %q", got.Subject, custom.Subject)
	}

	if err := ResetOverride(dir, name); err != nil {
		t.Fatalf("ResetOverride() error: %v", err)
	}
	if HasOverride(dir, name) {
		t.Fatal("HasOverride should be false after reset")
	}

	// Resetting an already-absent override is not an error.
	if err := ResetOverride(dir, name); err != nil {
		t.Fatalf("ResetOverride() on absent file error: %v", err)
	}
}

func TestSaveOverrideRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	invalid := Template{Subject: "", Body: ""}
	if err := SaveOverride(dir, "test", invalid); err == nil {
		t.Fatal("expected SaveOverride to reject an invalid template")
	}
}

func TestSaveOverrideUnknownTemplate(t *testing.T) {
	dir := t.TempDir()
	if err := SaveOverride(dir, "nope", Template{Subject: "s", Body: "b"}); err == nil {
		t.Fatal("expected error for unknown template name")
	}
}

func TestOverridePath(t *testing.T) {
	got := overridePath("/config", "test")
	want := filepath.Join("/config", "template", "email", "test.txt")
	if got != want {
		t.Errorf("overridePath() = %q, want %q", got, want)
	}
}

func TestSortedTemplateNames(t *testing.T) {
	names := sortedTemplateNames()
	if len(names) != len(TemplateNames) {
		t.Fatalf("got %d names, want %d", len(names), len(TemplateNames))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("names not sorted: %q before %q", names[i-1], names[i])
		}
	}
}
