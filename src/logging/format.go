package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/security"
)

// CEF header identity constants. They are frozen project variables and
// never come from user input.
const (
	// CEFVendor is the vendor field of the CEF header.
	CEFVendor = "webappsgo"
	// CEFProduct is the product field of the CEF header and the
	// syslog APP-NAME.
	CEFProduct = "redxt"
)

// Timestamp layouts used by the file formats. Log files are
// machine-parseable, so every layout is fixed and never localized.
const (
	// apacheTimeLayout is the Apache and Nginx access-log timestamp.
	apacheTimeLayout = "02/Jan/2006:15:04:05 -0700"
	// textTimeLayout is the leading timestamp of the text line format.
	textTimeLayout = time.RFC3339
	// millisTimeLayout is the ISO 8601 timestamp with milliseconds
	// required for audit entries and used by syslog and JSON lines.
	millisTimeLayout = "2006-01-02T15:04:05.000Z07:00"
	// customTimeLayout renders the {time} custom-format variable.
	customTimeLayout = "15:04:05"
	// customDateLayout renders the {date} custom-format variable.
	customDateLayout = "2006-01-02"
	// customDateTimeLayout renders the {datetime} custom-format
	// variable.
	customDateTimeLayout = "2006-01-02 15:04:05"
)

// emptyField is the placeholder written whenever a log field has no
// value, per the Apache and Nginx conventions.
const emptyField = "-"

// syslogFacilitySecurity is RFC 5424 facility 4, "security/
// authorization messages", which is what this server's syslog lines
// carry.
const syslogFacilitySecurity = 4

// Entry is one access-log record. It carries every variable from the
// AI.md PART 11 "Custom Format Variables" table so a single value can
// feed any of the access formats.
type Entry struct {
	// Time is when the request completed.
	Time time.Time
	// RemoteIP is the client IP address.
	RemoteIP string
	// Method is the HTTP method.
	Method string
	// Path is the request path without its query string.
	Path string
	// Query is the raw query string without the leading "?".
	Query string
	// Status is the HTTP response status code.
	Status int
	// Bytes is the response body size.
	Bytes int64
	// Latency is how long the request took.
	Latency time.Duration
	// UserAgent is the User-Agent header.
	UserAgent string
	// Referer is the Referer header.
	Referer string
	// RequestID is the per-request correlation identifier.
	RequestID string
	// FQDN is the request host.
	FQDN string
	// Protocol is the HTTP protocol version, for example HTTP/1.1.
	Protocol string
	// TLSVersion is the negotiated TLS version, empty for plain HTTP.
	TLSVersion string
	// Country is the GeoIP country code.
	Country string
	// ASN is the GeoIP autonomous system number.
	ASN string
}

// target returns the sanitized request target: the path with its query
// string appended when one is present, which is what a real request
// line carried.
func (e Entry) target() string {
	path := sanitize(e.Path)
	if path == "" {
		path = emptyField
	}
	if q := sanitize(e.Query); q != "" {
		return path + "?" + q
	}
	return path
}

// FormatApache renders an entry in Apache Combined Log Format. Every
// field that has no value is written as "-".
func FormatApache(e Entry) string {
	return fmt.Sprintf("%s - - [%s] %q %s %s %q %q",
		orDash(e.RemoteIP),
		e.Time.Format(apacheTimeLayout),
		orDash(e.Method)+" "+e.target()+" "+orDash(e.Protocol),
		statusField(e.Status),
		bytesField(e.Bytes),
		orDash(e.Referer),
		orDash(e.UserAgent),
	)
}

// FormatNginx renders an entry in Nginx Common Log Format, which is
// the Apache Combined line without the referer and user-agent pair.
func FormatNginx(e Entry) string {
	return fmt.Sprintf("%s - - [%s] %q %s %s",
		orDash(e.RemoteIP),
		e.Time.Format(apacheTimeLayout),
		orDash(e.Method)+" "+e.target()+" "+orDash(e.Protocol),
		statusField(e.Status),
		bytesField(e.Bytes),
	)
}

// accessJSON is the JSON access-log record. The field order is fixed
// by the AI.md PART 11 example and is preserved by declaring the
// struct fields in that order.
type accessJSON struct {
	IP     string `json:"ip"`
	Time   string `json:"time"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Size   int64  `json:"size"`
	UA     string `json:"ua"`
}

// FormatAccessJSON renders an entry as a single JSON access-log
// record with the exact key set and order the spec shows.
func FormatAccessJSON(e Entry) ([]byte, error) {
	rec := accessJSON{
		IP:     sanitize(e.RemoteIP),
		Time:   e.Time.UTC().Format(time.RFC3339),
		Method: sanitize(e.Method),
		Path:   e.target(),
		Status: e.Status,
		Size:   e.Bytes,
		UA:     sanitize(e.UserAgent),
	}
	out, err := json.Marshal(rec)
	if err != nil {
		return nil, fmt.Errorf("logging: encode access json: %w", err)
	}
	return out, nil
}

// FormatCustom substitutes the AI.md PART 11 custom-format variables
// into tmpl. An unknown "{token}" is left in the output verbatim.
//
// The template is walked exactly once rather than being run through a
// series of strings.ReplaceAll calls: a substituted value that itself
// contains braces (a crafted user agent, for example) must never be
// re-expanded by a later replacement pass, which would let a client
// inject fields into its own log line.
func FormatCustom(tmpl string, e Entry) string {
	vars := customVars(e)
	var b strings.Builder
	b.Grow(len(tmpl))
	for i := 0; i < len(tmpl); {
		if tmpl[i] != '{' {
			b.WriteByte(tmpl[i])
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			b.WriteString(tmpl[i:])
			break
		}
		name := tmpl[i+1 : i+end]
		if value, ok := vars[name]; ok {
			b.WriteString(value)
		} else {
			b.WriteString(tmpl[i : i+end+1])
		}
		i += end + 1
	}
	return b.String()
}

// customVars builds the substitution table for FormatCustom. Every
// value is sanitized here, so no caller can forget to do it.
func customVars(e Entry) map[string]string {
	return map[string]string{
		"time":        e.Time.Format(customTimeLayout),
		"date":        e.Time.Format(customDateLayout),
		"datetime":    e.Time.Format(customDateTimeLayout),
		"remote_ip":   orDash(e.RemoteIP),
		"method":      orDash(e.Method),
		"path":        orDash(e.Path),
		"query":       orDash(e.Query),
		"status":      statusField(e.Status),
		"bytes":       bytesField(e.Bytes),
		"latency":     e.Latency.String(),
		"latency_ms":  strconv.FormatInt(e.Latency.Milliseconds(), 10),
		"user_agent":  orDash(e.UserAgent),
		"referer":     orDash(e.Referer),
		"request_id":  orDash(e.RequestID),
		"fqdn":        orDash(e.FQDN),
		"protocol":    orDash(e.Protocol),
		"tls_version": orDash(e.TLSVersion),
		"country":     orDash(e.Country),
		"asn":         orDash(e.ASN),
	}
}

// FormatText renders one line of the PART 11 text log format:
// "2006-01-02T15:04:05-07:00 [LEVEL] message key=value ...". Level
// names are upper case and attributes are redacted before rendering.
func FormatText(t time.Time, level slog.Level, msg string, attrs []slog.Attr) string {
	line := t.Format(textTimeLayout) + " [" + levelName(level) + "] " + sanitize(msg)
	if rendered := renderAttrs(attrs); rendered != "" {
		line += " " + rendered
	}
	return line
}

// FormatJSON renders one line of the PART 11 JSON log format. The
// leading time, level, and msg keys keep the order the spec shows;
// attributes follow in sorted key order so the output is
// deterministic.
func FormatJSON(t time.Time, level slog.Level, msg string, attrs []slog.Attr) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"time":`)
	writeJSONString(&buf, t.UTC().Format(millisTimeLayout))
	buf.WriteString(`,"level":`)
	writeJSONString(&buf, levelName(level))
	buf.WriteString(`,"msg":`)
	writeJSONString(&buf, sanitize(msg))

	fields := security.RedactMap(attrsToMap(attrs))
	for _, key := range sortedKeys(fields) {
		encoded, err := json.Marshal(fields[key])
		if err != nil {
			return nil, fmt.Errorf("logging: encode log field %q: %w", key, err)
		}
		buf.WriteByte(',')
		writeJSONString(&buf, sanitize(key))
		buf.WriteByte(':')
		buf.Write(encoded)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// FormatFail2ban renders the PART 11 fail2ban line, which fail2ban
// filters match on the "[security]" tag.
func FormatFail2ban(t time.Time, msg string) string {
	return t.Format(textTimeLayout) + " [security] " + sanitize(msg)
}

// FormatSyslog renders an RFC 5424 syslog line with facility 4
// (security/authorization messages). PROCID, MSGID, and
// STRUCTURED-DATA are written as the nil value "-" so the line stays
// deterministic and free of host-specific data.
func FormatSyslog(t time.Time, level slog.Level, hostname, tag, msg string) string {
	pri := syslogFacilitySecurity*8 + syslogSeverity(level)
	return fmt.Sprintf("<%d>1 %s %s %s - - %s",
		pri,
		t.Format(millisTimeLayout),
		orDash(hostname),
		orDash(tag),
		sanitize(msg),
	)
}

// FormatCEF renders an ArcSight Common Event Format line:
// "CEF:0|vendor|product|version|signatureID|name|severity|extensions".
// Extension keys are emitted in sorted order so the line is
// deterministic, and every extension value is redacted by key name
// before it is written.
func FormatCEF(version, signatureID, name string, severity int, ext map[string]string) string {
	var b strings.Builder
	b.WriteString("CEF:0|")
	b.WriteString(cefHeaderEscape(CEFVendor))
	b.WriteByte('|')
	b.WriteString(cefHeaderEscape(CEFProduct))
	b.WriteByte('|')
	b.WriteString(cefHeaderEscape(orDash(version)))
	b.WriteByte('|')
	b.WriteString(cefHeaderEscape(orDash(signatureID)))
	b.WriteByte('|')
	b.WriteString(cefHeaderEscape(orDash(name)))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(severity))
	b.WriteByte('|')

	keys := make([]string, 0, len(ext))
	for k := range ext {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		value := ext[k]
		if security.IsSensitiveField(k) {
			value = security.RedactValue()
		}
		b.WriteString(cefExtensionEscape(sanitize(k)))
		b.WriteByte('=')
		b.WriteString(cefExtensionEscape(sanitize(value)))
	}
	return b.String()
}

// cefHeaderEscape escapes the CEF header separators: a backslash and a
// pipe.
func cefHeaderEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "|", `\|`)
}

// cefExtensionEscape escapes the CEF extension separators: a backslash
// and an equals sign.
func cefExtensionEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "=", `\=`)
}

// levelName returns the upper-case name of a slog level.
func levelName(level slog.Level) string {
	return strings.ToUpper(level.String())
}

// syslogSeverity maps a slog level onto an RFC 5424 severity code.
func syslogSeverity(level slog.Level) int {
	switch {
	case level >= slog.LevelError:
		return 3
	case level >= slog.LevelWarn:
		return 4
	case level >= slog.LevelInfo:
		return 6
	default:
		return 7
	}
}

// renderAttrs redacts an attribute set and renders it as sorted
// "key=value" pairs. A value containing a space or a quote is quoted
// so the pairs stay parseable.
func renderAttrs(attrs []slog.Attr) string {
	fields := security.RedactMap(attrsToMap(attrs))
	flat := make(map[string]string)
	flattenFields("", fields, flat)

	var b strings.Builder
	for i, key := range sortedKeys(flat) {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(sanitize(key))
		b.WriteByte('=')
		b.WriteString(textValue(flat[key]))
	}
	return b.String()
}

// textValue renders one attribute value for the text format, quoting
// it when it would otherwise break the key=value pairing.
func textValue(v string) string {
	v = sanitize(v)
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \"=") {
		return strconv.Quote(v)
	}
	return v
}

// flattenFields flattens a redacted field tree into dotted keys, so a
// nested group renders as "group.key=value" on a text line.
func flattenFields(prefix string, in map[string]any, out map[string]string) {
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			flattenFields(key, nested, out)
			continue
		}
		out[key] = fmt.Sprint(v)
	}
}

// attrsToMap converts a slog attribute slice into a plain map so it
// can be passed through security.RedactMap. Groups become nested maps
// and every value is resolved to its underlying Go value.
func attrsToMap(attrs []slog.Attr) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for _, a := range attrs {
		if a.Equal(slog.Attr{}) {
			continue
		}
		value := a.Value.Resolve()
		if value.Kind() == slog.KindGroup {
			group := attrsToMap(value.Group())
			if a.Key == "" {
				for k, v := range group {
					out[k] = v
				}
				continue
			}
			// Two attributes may carry the same group key, for
			// example when a handler wraps several record attributes
			// in one open group. Merging keeps both instead of
			// letting the later one replace the earlier.
			if existing, ok := out[a.Key].(map[string]any); ok {
				for k, v := range group {
					existing[k] = v
				}
				continue
			}
			out[a.Key] = group
			continue
		}
		out[a.Key] = value.Any()
	}
	return out
}

// sortedKeys returns the keys of a map in ascending order, so every
// rendered line is byte-for-byte reproducible.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeJSONString appends a JSON-encoded string to buf. The value is
// already sanitized by the caller, so encoding cannot fail.
func writeJSONString(buf *bytes.Buffer, s string) {
	encoded, err := json.Marshal(s)
	if err != nil {
		buf.WriteString(`""`)
		return
	}
	buf.Write(encoded)
}

// orDash sanitizes a value and substitutes "-" when nothing is left.
func orDash(s string) string {
	if out := sanitize(s); out != "" {
		return out
	}
	return emptyField
}

// statusField renders an HTTP status code, or "-" when the entry
// carries none.
func statusField(status int) string {
	if status <= 0 {
		return emptyField
	}
	return strconv.Itoa(status)
}

// bytesField renders a response size, or "-" when the response had no
// body, which is the Apache convention.
func bytesField(n int64) string {
	if n <= 0 {
		return emptyField
	}
	return strconv.FormatInt(n, 10)
}

// sanitize strips everything that must never reach a log file: ANSI
// escape sequences, control characters, and non-ASCII bytes. Removing
// carriage returns, newlines, and tabs is what stops a crafted header
// value from forging an additional log line, and dropping the escape
// sequences keeps files plain ASCII per the PART 11 log output rules.
func sanitize(s string) string {
	if s == "" {
		return ""
	}
	clean := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] >= 0x7f {
			clean = false
			break
		}
	}
	if clean {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == 0x1b {
			i = skipEscapeSequence(s, i)
			continue
		}
		if c < 0x20 || c >= 0x7f {
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// skipEscapeSequence returns the index just past the ANSI escape
// sequence starting at i. It understands CSI sequences (ESC [ ...
// final byte), OSC sequences (ESC ] ... BEL or ST), and the two-byte
// escapes; an escape at the very end of the string consumes only
// itself.
func skipEscapeSequence(s string, i int) int {
	i++
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[':
		i++
		for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
			i++
		}
		if i < len(s) {
			i++
		}
		return i
	case ']':
		i++
		for i < len(s) {
			if s[i] == 0x07 {
				return i + 1
			}
			if s[i] == 0x1b {
				if i+1 < len(s) && s[i+1] == '\\' {
					return i + 2
				}
				return i
			}
			i++
		}
		return i
	default:
		return i + 1
	}
}
