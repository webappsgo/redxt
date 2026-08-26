// Package notify implements AI.md PART 18 "Email & Notifications":
// SMTP autodetection and sending, the plain-text {variable} email
// template engine with its 22 embedded default templates (24 files
// counting the welcome template's admin/user variants), and template
// validation.
//
// The template syntax is deliberately not Go's text/template: PART 18
// specifies a small, fixed `{variable}` substitution grammar so an
// admin editing a template in the browser never has to reason about
// Go template actions, only about which curly-brace names exist.
package notify

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Template is a parsed "Subject: ...\n---\nbody" document.
type Template struct {
	Subject string
	Body    string
}

// ErrEmptyTemplate reports that a template's raw text had no content
// at all.
var errEmptyTemplate = fmt.Errorf("notify: template is empty")

// ParseTemplate splits raw template text into its subject and body
// per the PART 18 "Template Format": a first "Subject: ..." line, a
// "---" separator on its own line, then the plain-text body.
func ParseTemplate(raw string) (Template, error) {
	if strings.TrimSpace(raw) == "" {
		return Template{}, errEmptyTemplate
	}

	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return Template{}, errEmptyTemplate
	}

	first := lines[0]
	const prefix = "Subject:"
	if !strings.HasPrefix(first, prefix) {
		return Template{}, fmt.Errorf("notify: template must start with %q", prefix)
	}
	subject := strings.TrimSpace(strings.TrimPrefix(first, prefix))

	sepIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			sepIdx = i
			break
		}
	}
	if sepIdx < 0 {
		return Template{}, fmt.Errorf("notify: template is missing the --- separator")
	}

	body := strings.Join(lines[sepIdx+1:], "\n")
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimRight(body, "\n") + "\n"

	return Template{Subject: subject, Body: body}, nil
}

// varPattern matches a {variable_name} placeholder: a letter or
// underscore followed by letters, digits, or underscores.
var varPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Render substitutes every {variable} placeholder in s with the value
// from vars. A placeholder with no matching key is left in place
// unchanged, so a rendering call never fails on an unknown or
// not-yet-supplied variable; Validate is what catches that case
// before the template is saved.
func Render(s string, vars map[string]string) string {
	return varPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

// RenderTemplate renders both the subject and the body of t.
func RenderTemplate(t Template, vars map[string]string) Template {
	return Template{
		Subject: Render(t.Subject, vars),
		Body:    Render(t.Body, vars),
	}
}

// Variables returns every distinct {variable} name referenced in s,
// in first-appearance order.
func Variables(s string) []string {
	matches := varPattern.FindAllStringSubmatch(s, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// GlobalVariables lists the {variable} names available in every
// template, per AI.md PART 18 "Global Variables (Available in All
// Templates)".
var GlobalVariables = []string{
	"app_name",
	"app_url",
	"fqdn",
	"onion_url",
	"onion_address",
	"i2p_url",
	"i2p_address",
	"admin_email",
	"recipient_email",
	"recipient_username",
	"timestamp",
	"year",
}

// TemplateVariables lists the additional {variable} names each named
// template accepts, per AI.md PART 18 "Template-Specific Variables".
// A name not present here and not in GlobalVariables is unknown.
var TemplateVariables = map[string][]string{
	"welcome_admin":       {"admin_url", "admin_username"},
	"welcome_user":        {"login_url", "profile_url"},
	"password_reset":      {"reset_link", "expires", "ip"},
	"email_verify":        {"verify_link", "expires"},
	"login_alert":         {"ip", "location", "device", "time"},
	"security_alert":      {"event", "ip", "details"},
	"mfa_reminder":        {"setup_url", "dismiss_url"},
	"2fa_enabled":         {"method", "ip"},
	"2fa_disabled":        {"method", "ip"},
	"password_changed":    {"ip", "method"},
	"token_regenerated":   {"ip", "token_name"},
	"backup_complete":     {"filename", "size"},
	"backup_failed":       {"filename", "error"},
	"ssl_expiring":        {"expires_in", "expiry_date"},
	"ssl_renewed":         {"valid_until"},
	"ssl_renewal_failed":  {"error", "expires_in", "expiry_date", "next_retry"},
	"scheduler_error":     {"task_name", "error", "next_run"},
	"startup":             {"version", "mode"},
	"shutdown":            {"reason", "uptime"},
	"update_available":    {"current_version", "new_version", "channel"},
	"update_installed":    {"previous_version", "new_version"},
	"breach_notification": {"breach_id", "breach_date", "breach_type", "affected_data", "breach_summary", "recommended_actions", "contact_email", "contact_phone", "dpo_contact", "regulatory_notice", "notification_deadline"},
	"breach_admin_alert":  {"breach_id", "severity", "breach_type", "breach_summary", "detection_method", "trigger", "source_ip", "affected_scope", "affected_users", "affected_data", "auto_actions", "compliance_requirements", "notify_deadline", "admin_url"},
	"test":                {},
}

// AccountTemplates lists the templates AI.md PART 18's "Account
// Email Requirements" applies to: they must be able to carry
// {recipient_email}, and Validate requires it be referenced.
var AccountTemplates = map[string]bool{
	"welcome_user":        true,
	"password_reset":      true,
	"email_verify":        true,
	"login_alert":         true,
	"security_alert":      true,
	"mfa_reminder":        true,
	"2fa_enabled":         true,
	"2fa_disabled":        true,
	"password_changed":    true,
	"token_regenerated":   true,
	"breach_notification": true,
}

// ValidationResult holds Validate's findings. A non-empty Errors means
// the template must not be saved; Warnings never block a save.
type ValidationResult struct {
	Errors   []string
	Warnings []string
}

// OK reports whether the template has no blocking errors.
func (r ValidationResult) OK() bool {
	return len(r.Errors) == 0
}

// Validate checks a template's subject and body against the PART 18
// "Template Validation" table: unknown variables, a missing required
// {recipient_email} on an account template, and empty subject/body
// are errors; a long subject line and an account template missing
// its disclaimer are warnings.
func Validate(name string, t Template) ValidationResult {
	var res ValidationResult

	if strings.TrimSpace(t.Subject) == "" {
		res.Errors = append(res.Errors, "Subject cannot be empty")
	}
	if strings.TrimSpace(t.Body) == "" {
		res.Errors = append(res.Errors, "Body cannot be empty")
	}

	allowed := allowedVariables(name)
	used := make(map[string]bool)
	for _, v := range Variables(t.Subject) {
		used[v] = true
	}
	for _, v := range Variables(t.Body) {
		used[v] = true
	}

	names := make([]string, 0, len(used))
	for v := range used {
		names = append(names, v)
	}
	sort.Strings(names)

	for _, v := range names {
		if allowed[v] {
			continue
		}
		if suggestion := closestVariable(v, allowed); suggestion != "" {
			res.Errors = append(res.Errors, fmt.Sprintf("Unknown variable: {%s}. Did you mean {%s}?", v, suggestion))
		} else {
			res.Errors = append(res.Errors, fmt.Sprintf("Unknown variable: {%s}", v))
		}
	}

	if AccountTemplates[name] && !used["recipient_email"] {
		res.Errors = append(res.Errors, "Account emails must include {recipient_email}")
	}

	if len(t.Subject) > 78 {
		res.Warnings = append(res.Warnings, "Subject line is longer than 78 characters")
	}
	if AccountTemplates[name] && !strings.Contains(strings.ToLower(t.Body), "did not") {
		res.Warnings = append(res.Warnings, "Account emails should include a disclaimer for unsolicited recipients")
	}

	return res
}

// allowedVariables returns the set of {variable} names name may use:
// every global variable plus that template's own.
func allowedVariables(name string) map[string]bool {
	allowed := make(map[string]bool, len(GlobalVariables)+4)
	for _, v := range GlobalVariables {
		allowed[v] = true
	}
	for _, v := range TemplateVariables[name] {
		allowed[v] = true
	}
	return allowed
}

// closestVariable returns the allowed name with the smallest edit
// distance to want, or "" if none is close enough to suggest.
func closestVariable(want string, allowed map[string]bool) string {
	best := ""
	bestDist := -1
	for name := range allowed {
		d := levenshtein(want, name)
		if bestDist == -1 || d < bestDist {
			bestDist = d
			best = name
		}
	}
	if bestDist >= 0 && bestDist <= 3 {
		return best
	}
	return ""
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
