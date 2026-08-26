package notify

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// defaultTemplates embeds every built-in email template listed in AI.md
// PART 18 "Default Templates": the welcome template's admin and user
// variants plus the other 22 named templates, one file per name.
//
//go:embed templates/*.txt
var defaultTemplates embed.FS

// TemplateNames lists every template name recognized by this package,
// in the order AI.md PART 18 documents them.
var TemplateNames = []string{
	"welcome_admin",
	"welcome_user",
	"password_reset",
	"email_verify",
	"login_alert",
	"security_alert",
	"mfa_reminder",
	"2fa_enabled",
	"2fa_disabled",
	"password_changed",
	"token_regenerated",
	"backup_complete",
	"backup_failed",
	"ssl_expiring",
	"ssl_renewed",
	"ssl_renewal_failed",
	"scheduler_error",
	"startup",
	"shutdown",
	"update_available",
	"update_installed",
	"breach_notification",
	"breach_admin_alert",
	"test",
}

// errUnknownTemplate reports a template name with no embedded default.
func errUnknownTemplate(name string) error {
	return fmt.Errorf("notify: unknown template %q", name)
}

// DefaultRaw returns the embedded default raw text for name.
func DefaultRaw(name string) (string, error) {
	if !isKnownTemplate(name) {
		return "", errUnknownTemplate(name)
	}
	b, err := defaultTemplates.ReadFile("templates/" + name + ".txt")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// isKnownTemplate reports whether name is one of TemplateNames.
func isKnownTemplate(name string) bool {
	for _, n := range TemplateNames {
		if n == name {
			return true
		}
	}
	return false
}

// overridePath returns the path a custom override for name would live
// at under configDir, per AI.md PART 18 "Template Storage":
// {config_dir}/template/email/<name>.txt.
func overridePath(configDir, name string) string {
	return filepath.Join(configDir, "template", "email", name+".txt")
}

// Load returns the effective Template for name: the custom override
// under configDir if one exists, otherwise the embedded default. An
// unknown name is always an error, even if a stray override file
// exists for it.
func Load(configDir, name string) (Template, error) {
	if !isKnownTemplate(name) {
		return Template{}, errUnknownTemplate(name)
	}

	if configDir != "" {
		path := overridePath(configDir, name)
		if b, err := os.ReadFile(path); err == nil {
			return ParseTemplate(string(b))
		} else if !os.IsNotExist(err) {
			return Template{}, err
		}
	}

	raw, err := DefaultRaw(name)
	if err != nil {
		return Template{}, err
	}
	return ParseTemplate(raw)
}

// HasOverride reports whether name has a custom override saved under
// configDir.
func HasOverride(configDir, name string) bool {
	if configDir == "" {
		return false
	}
	_, err := os.Stat(overridePath(configDir, name))
	return err == nil
}

// SaveOverride writes a custom override for name under configDir,
// validating it first. Validation errors block the save; warnings do
// not.
func SaveOverride(configDir, name string, t Template) error {
	if !isKnownTemplate(name) {
		return errUnknownTemplate(name)
	}
	if res := Validate(name, t); !res.OK() {
		return fmt.Errorf("notify: template %q failed validation: %v", name, res.Errors)
	}

	path := overridePath(configDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw := fmt.Sprintf("Subject: %s\n---\n%s", t.Subject, t.Body)
	return os.WriteFile(path, []byte(raw), 0o644)
}

// ResetOverride deletes name's custom override under configDir,
// reverting it to the embedded default. Deleting an override that
// does not exist is not an error.
func ResetOverride(configDir, name string) error {
	if !isKnownTemplate(name) {
		return errUnknownTemplate(name)
	}
	err := os.Remove(overridePath(configDir, name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// sortedTemplateNames returns TemplateNames sorted, for deterministic
// iteration in tests and admin listings.
func sortedTemplateNames() []string {
	out := append([]string(nil), TemplateNames...)
	sort.Strings(out)
	return out
}
