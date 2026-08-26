package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

// PART 11 stage 4 truncation ceilings. A long field is both a leakage
// risk and a denial-of-service risk, so every string leaving the server
// is capped by what kind of string it is.
const (
	// MaxURLLength caps a URL-valued field.
	MaxURLLength = 256
	// MaxMessageLength caps a message or sample field.
	MaxMessageLength = 200
	// MaxStackBytes caps a stack trace field, 2 KB.
	MaxStackBytes = 2048
	// TruncationMarker is appended to a value that was cut short, so a
	// reader can tell a truncated value from a naturally short one.
	TruncationMarker = "…"
	// RedactedMarker replaces a value stage 2 or stage 3 removed.
	RedactedMarker = "[redacted]"
	// DefaultMinAuthDuration is the fixed floor stage 6 pads an
	// authentication response to.
	DefaultMinAuthDuration = 100 * time.Millisecond
	// DevOnlyTag marks a struct field that exists only for debugging.
	DevOnlyTag = "dev_only"
)

// timeType is time.Time's reflect type, compared against rather than
// type-asserted so the check costs nothing on every other struct.
var timeType = reflect.TypeOf(time.Time{})

// sensitiveParams are the query parameter names stage 2 redacts,
// compared case-insensitively.
var sensitiveParams = []string{
	"token", "session", "code", "key", "password", "secret",
	"auth", "pwd", "api_key", "apikey", "access_token", "refresh_token",
}

// urlFieldNames are the field names treated as URLs: they get the URL
// truncation ceiling, they are run through the query-parameter
// redaction, and they are exempt from filesystem-path stripping, whose
// pattern would otherwise mangle a legitimate URL path.
var urlFieldNames = []string{"url", "uri", "href", "link", "location", "endpoint", "route", "path", "referer", "referrer"}

// stackFieldNames are the field names treated as stack traces, which get
// the 2 KB ceiling instead of the message ceiling.
var stackFieldNames = []string{"stack", "stacktrace", "stack_trace", "traceback", "backtrace"}

// internalIPPattern matches the private and loopback ranges stage 3
// strips: RFC 1918 blocks, loopback, and the link-local range.
//
// The leading boundary group keeps it from matching inside a longer
// number — Go's regexp has no lookbehind, so the boundary is captured
// and written back by the replacement.
var internalIPPattern = regexp.MustCompile(`(^|[^0-9.])((?:10\.|172\.(?:1[6-9]|2[0-9]|3[01])\.|192\.168\.|127\.|169\.254\.)[0-9]{1,3}\.[0-9]{1,3}(?:\.[0-9]{1,3})?)`)

// filesystemPathPattern matches an absolute filesystem path of two or
// more segments, which is what leaks a server's directory layout. One
// segment is deliberately not enough: "/login" and "/api" are routes,
// not paths, and redacting them would gut every legitimate message.
var filesystemPathPattern = regexp.MustCompile(`(^|[\s"'(\[=,])((?:/[A-Za-z0-9._+-]+){2,}/?)`)

// Sanitizer runs the PART 11 six stage output sanitization pipeline. Its
// zero value is usable and behaves as production does: the debug flag is
// off, so stage 5 strips every dev_only field.
//
// A Sanitizer is safe for concurrent use once configured, and must not
// be mutated after the first request.
type Sanitizer struct {
	// Debug mirrors the --debug flag and the DEBUG environment
	// variable. It is the ONLY thing that keeps dev_only fields in a
	// response: the operating mode, production or development, does not
	// affect stage 5.
	Debug bool

	// AllowFields is stage 1. When it is non-empty, only the JSON field
	// names it lists survive; every other field is dropped. Leave it
	// empty for a response shape whose struct tags are already the
	// allow-list.
	AllowFields []string

	// MinAuthDuration is the floor stage 6 pads to. Zero selects
	// DefaultMinAuthDuration.
	MinAuthDuration time.Duration

	// Now supplies the current time for stage 6. Nil uses time.Now.
	Now func() time.Time

	// Sleep pads the remaining time in stage 6. Nil uses time.Sleep.
	// Tests inject both this and Now so the padding is asserted without
	// a real delay and without racing the wall clock.
	Sleep func(time.Duration)
}

// Sanitize runs stages 1 through 5 over a value and returns the
// sanitized copy. The input is never modified.
//
// The result is built from maps and slices rather than the original
// struct types, because dropping a field is not something a struct can
// express. Encoding a map sorts its keys, so the same input always
// marshals to the same bytes.
//
// Stage 6 is not here: padding a response's duration is a property of
// the request, not of the payload, so it lives in FinalizeAuth.
func (s *Sanitizer) Sanitize(value any) any {
	return s.sanitizeValue(reflect.ValueOf(value), "", true)
}

// sanitizeValue walks one value. fieldName is the JSON name of the field
// the value came from, which decides which truncation ceiling and which
// string rules apply; top marks the outermost value, where the stage 1
// allow-list is enforced.
func (s *Sanitizer) sanitizeValue(rv reflect.Value, fieldName string, top bool) any {
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return s.sanitizeValue(rv.Elem(), fieldName, top)
	case reflect.String:
		return s.sanitizeString(rv.String(), fieldName)
	case reflect.Struct:
		// A timestamp is a struct only by implementation. Walking its
		// unexported fields would turn a value that marshals as an RFC
		// 3339 string into an empty object.
		if rv.Type() == timeType {
			return rv.Interface()
		}
		return s.sanitizeStruct(rv, top)
	case reflect.Map:
		return s.sanitizeMap(rv, top)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			return s.sanitizeString(string(rv.Bytes()), fieldName)
		}
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out = append(out, s.sanitizeValue(rv.Index(i), fieldName, false))
		}
		return out
	default:
		return rv.Interface()
	}
}

// sanitizeStruct converts a struct to a map keyed by JSON field name,
// applying stage 5 to drop dev_only fields and stage 1 to enforce the
// allow-list.
func (s *Sanitizer) sanitizeStruct(rv reflect.Value, top bool) map[string]any {
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if !s.Debug && field.Tag.Get(DevOnlyTag) == "true" {
			continue
		}

		name, omitEmpty, skip := jsonFieldName(field)
		if skip {
			continue
		}
		value := rv.Field(i)
		if omitEmpty && value.IsZero() {
			continue
		}
		if top && !s.fieldAllowed(name) {
			continue
		}
		out[name] = s.sanitizeValue(value, name, false)
	}
	return out
}

// sanitizeMap converts a map to a map keyed by its string keys. A
// non-string key is rendered by its natural formatting, so a map keyed
// by anything else still round-trips through JSON.
func (s *Sanitizer) sanitizeMap(rv reflect.Value, top bool) map[string]any {
	out := make(map[string]any, rv.Len())
	for _, key := range rv.MapKeys() {
		name := mapKeyName(key)
		if isDevOnlyName(name) && !s.Debug {
			continue
		}
		if top && !s.fieldAllowed(name) {
			continue
		}
		out[name] = s.sanitizeValue(rv.MapIndex(key), name, false)
	}
	return out
}

// fieldAllowed applies stage 1. An empty allow-list allows everything,
// so a handler whose struct tags already describe the response shape
// does not have to restate them.
func (s *Sanitizer) fieldAllowed(name string) bool {
	if len(s.AllowFields) == 0 {
		return true
	}
	for _, allowed := range s.AllowFields {
		if allowed == name {
			return true
		}
	}
	return false
}

// sanitizeString applies stages 2, 3 and 4 to one string, in that order:
// redact sensitive query parameters, strip internal addresses and
// filesystem paths, then truncate to the ceiling for this kind of field.
func (s *Sanitizer) sanitizeString(value, fieldName string) string {
	isURL := nameMatches(fieldName, urlFieldNames)

	if isURL || strings.Contains(value, "?") {
		value = RedactURLParams(value)
	}
	value = StripInternal(value, isURL)

	limit := MaxMessageLength
	switch {
	case nameMatches(fieldName, stackFieldNames):
		limit = MaxStackBytes
	case isURL:
		limit = MaxURLLength
	}
	return Truncate(value, limit)
}

// RedactQueryParams applies stage 2 to a bare query string — the form
// the access log records — and returns it with every sensitive value
// replaced. Parameter names and their order are preserved so a log line
// still shows what was asked for, only not with what credential.
func RedactQueryParams(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	pairs := strings.Split(rawQuery, "&")
	for i, pair := range pairs {
		name, _, hasValue := strings.Cut(pair, "=")
		if !hasValue {
			continue
		}
		decoded, err := url.QueryUnescape(name)
		if err != nil {
			decoded = name
		}
		if isSensitiveParam(decoded) {
			pairs[i] = name + "=" + RedactedMarker
		}
	}
	return strings.Join(pairs, "&")
}

// RedactURLParams applies stage 2 to a string that may contain a URL,
// returning it with the sensitive query parameters redacted. A value
// that does not parse as a URL is returned unchanged; stage 3 still
// inspects it afterwards.
func RedactURLParams(value string) string {
	head, query, found := strings.Cut(value, "?")
	if !found {
		return value
	}
	query, fragment, hasFragment := strings.Cut(query, "#")
	redacted := head + "?" + RedactQueryParams(query)
	if hasFragment {
		redacted += "#" + fragment
	}
	return redacted
}

// StripInternal applies stage 3, replacing private and loopback IP
// addresses with [redacted]. When keepPaths is false it also replaces
// absolute filesystem paths, which disclose the server's directory
// layout.
//
// keepPaths is true for URL-valued fields: "/api/v1/records" inside a
// URL is a route the client needs, not a path on disk.
func StripInternal(value string, keepPaths bool) string {
	value = internalIPPattern.ReplaceAllString(value, "${1}"+RedactedMarker)
	if keepPaths {
		return value
	}
	return filesystemPathPattern.ReplaceAllString(value, "${1}"+RedactedMarker)
}

// Truncate applies stage 4, cutting value to limit runes and marking the
// cut. It counts runes rather than bytes so a multi-byte character is
// never split into invalid UTF-8.
func Truncate(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + TruncationMarker
}

// FinalizeAuth applies stage 6: it blocks until at least MinAuthDuration
// has passed since started.
//
// Login, password reset and token validation all take measurably
// different amounts of time depending on whether the account exists and
// whether the secret matched. Padding every one of them to the same
// floor removes that signal, which is what makes the PART 11 rule — never
// disclose which half of a failed login was wrong — hold against a
// caller with a stopwatch as well as against one reading the message.
func (s *Sanitizer) FinalizeAuth(started time.Time) {
	minimum := s.MinAuthDuration
	if minimum <= 0 {
		minimum = DefaultMinAuthDuration
	}
	now := s.Now
	if now == nil {
		now = time.Now
	}
	sleep := s.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	if remaining := minimum - now().Sub(started); remaining > 0 {
		sleep(remaining)
	}
}

// sanitizerKey stores the active Sanitizer in a request context.
var sanitizerKey = contextKey{name: "sanitizer"}

// SanitizeMiddleware attaches s to every request context so a handler
// can reach the one configured pipeline instead of constructing its own
// with different settings.
//
// It is not a response filter. A filter here would have to buffer and
// re-encode every response body, which would break streaming downloads
// and file serving for no gain: the handler knows its own response shape
// and calls SanitizerFromContext(ctx).Sanitize on it before encoding.
func SanitizeMiddleware(s *Sanitizer) Middleware {
	if s == nil {
		return passthrough
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), sanitizerKey, s)))
		})
	}
}

// SanitizerFromContext returns the Sanitizer attached to ctx, or a
// production-default one when no middleware attached it. The fallback is
// never nil and never permissive: a handler that runs outside the chain
// still strips dev_only fields.
func SanitizerFromContext(ctx context.Context) *Sanitizer {
	if ctx != nil {
		if s, ok := ctx.Value(sanitizerKey).(*Sanitizer); ok && s != nil {
			return s
		}
	}
	return &Sanitizer{}
}

// jsonFieldName resolves a struct field's JSON name and options,
// reporting skip for a field tagged `json:"-"`.
func jsonFieldName(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	name, options, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	for _, option := range strings.Split(options, ",") {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

// mapKeyName renders a map key as a string, so a map keyed by anything
// else still produces a JSON object.
func mapKeyName(key reflect.Value) string {
	if key.Kind() == reflect.String {
		return key.String()
	}
	return fmt.Sprint(key.Interface())
}

// isDevOnlyName reports whether a map key names one of the debug-only
// fields. A map has no struct tags, so the convention — a leading
// underscore, as in _debug and _internal_id — carries the marking
// instead.
func isDevOnlyName(name string) bool {
	return strings.HasPrefix(name, "_")
}

// isSensitiveParam reports whether a query parameter name is on the
// stage 2 list, comparing case-insensitively.
func isSensitiveParam(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, sensitive := range sensitiveParams {
		if name == sensitive {
			return true
		}
	}
	return false
}

// nameMatches reports whether a JSON field name is one of the listed
// names, ignoring case, underscores and hyphens so "stack_trace",
// "stackTrace" and "StackTrace" are all recognized.
func nameMatches(name string, list []string) bool {
	normalized := normalizeFieldName(name)
	for _, candidate := range list {
		if normalized == normalizeFieldName(candidate) {
			return true
		}
	}
	return false
}

// normalizeFieldName lowercases a field name and drops the separators
// that distinguish naming styles rather than meanings.
func normalizeFieldName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '_' || r == '-' || r == ' ' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// SensitiveParams returns the stage 2 parameter names in sorted order.
// The admin panel lists them so an operator can see exactly what is
// redacted rather than inferring it from behavior.
func SensitiveParams() []string {
	out := make([]string, len(sensitiveParams))
	copy(out, sensitiveParams)
	sort.Strings(out)
	return out
}
