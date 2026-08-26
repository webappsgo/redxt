package middleware

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sanitizeAccount is the response shape the stage 1 and stage 5 cases
// marshal: a public half, a field no public handler should ever return,
// and a field that exists only for troubleshooting.
type sanitizeAccount struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Password string `json:"password_hash"`
	Internal string `json:"_internal_id,omitempty" dev_only:"true"`
	Debug    string `json:"_debug,omitempty" dev_only:"true"`
	Skipped  string `json:"-"`
}

// sanitizeReport carries one field of each kind stage 4 gives a
// different ceiling to.
type sanitizeReport struct {
	URL     string `json:"url"`
	Message string `json:"message"`
	Stack   string `json:"stack_trace"`
}

// sanitizeAsMap runs the pipeline and asserts the result is an object,
// which every struct and map input produces.
func sanitizeAsMap(t *testing.T, s *Sanitizer, value any) map[string]any {
	t.Helper()
	sanitized := s.Sanitize(value)
	out, ok := sanitized.(map[string]any)
	if !ok {
		t.Fatalf("Sanitize returned %T, want map[string]any: a struct or map must sanitize to an object", sanitized)
	}
	return out
}

func TestSanitizeStageOneAllowList(t *testing.T) {
	cases := []struct {
		name    string
		allow   []string
		wantHas []string
		wantNot []string
		reason  string
	}{
		{
			name:    "allow-list keeps only what it names",
			allow:   []string{"id", "name"},
			wantHas: []string{"id", "name"},
			wantNot: []string{"password_hash"},
			reason:  "a sensitive field added to a shared struct must not reach a public response",
		},
		{
			name:    "empty allow-list keeps every tagged field",
			wantHas: []string{"id", "name", "password_hash"},
			reason:  "a handler whose struct already is the response shape must not have to restate it",
		},
		{
			name:    "allow-list naming an absent field drops everything else",
			allow:   []string{"nothing_here"},
			wantNot: []string{"id", "name", "password_hash"},
			reason:  "stage 1 is an allow-list, so an unmatched name must fail closed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Sanitizer{AllowFields: tc.allow}
			out := sanitizeAsMap(t, s, sanitizeAccount{ID: "u1", Name: "casey", Password: "argon2id$..."})

			for _, key := range tc.wantHas {
				if _, ok := out[key]; !ok {
					t.Errorf("field %q missing from %v: %s", key, out, tc.reason)
				}
			}
			for _, key := range tc.wantNot {
				if _, ok := out[key]; ok {
					t.Errorf("field %q survived in %v: %s", key, out, tc.reason)
				}
			}
		})
	}
}

func TestSanitizeStageOneSkipsIgnoredFields(t *testing.T) {
	out := sanitizeAsMap(t, &Sanitizer{}, sanitizeAccount{ID: "u1", Skipped: "never"})

	if _, ok := out["Skipped"]; ok {
		t.Errorf("json:\"-\" field survived in %v: a field the encoder skips must not reappear through the pipeline", out)
	}
}

func TestSanitizeStageTwoRedactsSensitiveParams(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		want   string
		reason string
	}{
		{"token", "token=abc123", "token=" + RedactedMarker, "a bearer credential in a URL is a credential in the log"},
		{"session", "session=s-1", "session=" + RedactedMarker, "a session identifier is as good as the session"},
		{"code", "code=oauth-code", "code=" + RedactedMarker, "an OAuth code exchanges for a full token"},
		{"key", "key=k-1", "key=" + RedactedMarker, "a bare key parameter is the credential itself"},
		{"password", "password=hunter2", "password=" + RedactedMarker, "a password must never be stored anywhere in clear"},
		{"secret", "secret=s-1", "secret=" + RedactedMarker, "a secret parameter is named for what it is"},
		{"auth", "auth=basic-blob", "auth=" + RedactedMarker, "an auth parameter carries the whole credential"},
		{"pwd", "pwd=hunter2", "pwd=" + RedactedMarker, "the short spelling is just as sensitive"},
		{"api_key", "api_key=k-1", "api_key=" + RedactedMarker, "an API key grants the caller's full rights"},
		{"apikey", "apikey=k-1", "apikey=" + RedactedMarker, "the compact spelling must be caught too"},
		{"access_token", "access_token=a-1", "access_token=" + RedactedMarker, "an access token is a live credential"},
		{"refresh_token", "refresh_token=r-1", "refresh_token=" + RedactedMarker, "a refresh token outlives the access token it mints"},
		{"uppercase name", "TOKEN=abc123", "TOKEN=" + RedactedMarker, "PART 11 requires a case-insensitive match"},
		{"mixed case name", "Api_Key=k-1", "Api_Key=" + RedactedMarker, "a client picks its own capitalization"},
		{"harmless parameter kept", "page=2", "page=2", "redacting everything would make the log useless"},
		{
			name:   "only the sensitive pair is touched",
			query:  "page=2&token=abc123&sort=name",
			want:   "page=2&token=" + RedactedMarker + "&sort=name",
			reason: "parameter names and order must survive so the log still shows what was asked for",
		},
		{
			name:   "valueless parameter is left alone",
			query:  "token",
			want:   "token",
			reason: "a flag-style parameter carries no secret to remove",
		},
		{"empty query", "", "", "an empty query must not grow an equals sign"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactQueryParams(tc.query); got != tc.want {
				t.Errorf("RedactQueryParams(%q) = %q, want %q: %s", tc.query, got, tc.want, tc.reason)
			}
		})
	}
}

func TestSanitizeStageTwoRedactsInsideURLs(t *testing.T) {
	cases := []struct {
		name   string
		value  string
		want   string
		reason string
	}{
		{
			name:   "query inside a url",
			value:  "https://redxt.example.test/callback?code=oauth-code&state=xyz",
			want:   "https://redxt.example.test/callback?code=" + RedactedMarker + "&state=xyz",
			reason: "a callback URL echoed into a response body carries the code with it",
		},
		{
			name:   "fragment is preserved",
			value:  "https://redxt.example.test/p?token=abc#section",
			want:   "https://redxt.example.test/p?token=" + RedactedMarker + "#section",
			reason: "the fragment identifies the view and must survive the redaction",
		},
		{
			name:   "url without a query is unchanged",
			value:  "https://redxt.example.test/records",
			want:   "https://redxt.example.test/records",
			reason: "a value with nothing to redact must come back byte-identical",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactURLParams(tc.value); got != tc.want {
				t.Errorf("RedactURLParams(%q) = %q, want %q: %s", tc.value, got, tc.want, tc.reason)
			}
		})
	}
}

func TestSanitizeStageThreeStripsInternalDetail(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		keepPaths bool
		want      string
		reason    string
	}{
		{
			name:   "rfc 1918 ten block",
			value:  "dial tcp 10.1.2.3 refused",
			want:   "dial tcp " + RedactedMarker + " refused",
			reason: "a private address maps the operator's internal network",
		},
		{
			name:   "rfc 1918 172 block lower bound",
			value:  "peer 172.16.0.9 timed out",
			want:   "peer " + RedactedMarker + " timed out",
			reason: "the 172.16/12 block starts at 172.16",
		},
		{
			name:   "rfc 1918 172 block upper bound",
			value:  "peer 172.31.255.1 timed out",
			want:   "peer " + RedactedMarker + " timed out",
			reason: "the 172.16/12 block ends at 172.31",
		},
		{
			name:   "172 outside the private block is public",
			value:  "peer 172.32.0.1 timed out",
			want:   "peer 172.32.0.1 timed out",
			reason: "172.32 is public space and redacting it would hide a real client",
		},
		{
			name:   "rfc 1918 192.168 block",
			value:  "node 192.168.1.20 unreachable",
			want:   "node " + RedactedMarker + " unreachable",
			reason: "the home-network block is still internal detail",
		},
		{
			name:   "loopback",
			value:  "bound 127.0.0.1 already",
			want:   "bound " + RedactedMarker + " already",
			reason: "loopback discloses that a service is co-located",
		},
		{
			name:   "link local",
			value:  "metadata 169.254.169.254 denied",
			want:   "metadata " + RedactedMarker + " denied",
			reason: "the cloud metadata address is the classic SSRF target",
		},
		{
			name:   "public address is kept",
			value:  "client 203.0.113.9 connected",
			want:   "client 203.0.113.9 connected",
			reason: "the pipeline must not blank out the one address an operator needs",
		},
		{
			name:   "digits before the block are not an address",
			value:  "build 1710.1.2.3 shipped",
			want:   "build 1710.1.2.3 shipped",
			reason: "the boundary group stops the pattern matching inside a longer number",
		},
		{
			name:   "filesystem path is stripped",
			value:  "open /etc/webappsgo/redxt/server.yml failed",
			want:   "open " + RedactedMarker + " failed",
			reason: "an absolute path discloses the server's directory layout",
		},
		{
			name:   "single segment is a route not a path",
			value:  "route /login rejected",
			want:   "route /login rejected",
			reason: "redacting one-segment paths would gut every legitimate message",
		},
		{
			name:      "url fields keep their paths",
			value:     "https://redxt.example.test/api/v1/records",
			keepPaths: true,
			want:      "https://redxt.example.test/api/v1/records",
			reason:    "a route inside a URL is what the client needs, not a path on disk",
		},
		{
			name:      "url fields still lose internal addresses",
			value:     "http://10.1.2.3/api/v1/records",
			keepPaths: true,
			want:      "http://" + RedactedMarker + "/api/v1/records",
			reason:    "an internal host is disclosure even when it appears in a URL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripInternal(tc.value, tc.keepPaths); got != tc.want {
				t.Errorf("StripInternal(%q, %v) = %q, want %q: %s", tc.value, tc.keepPaths, got, tc.want, tc.reason)
			}
		})
	}
}

func TestSanitizeStageFourTruncate(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		limit   int
		wantLen int
		wantCut bool
		reason  string
	}{
		{
			name:    "value under the limit is untouched",
			value:   "short",
			limit:   MaxMessageLength,
			wantLen: 5,
			reason:  "a naturally short value must not be marked as truncated",
		},
		{
			name:    "value exactly at the limit is untouched",
			value:   strings.Repeat("a", MaxMessageLength),
			limit:   MaxMessageLength,
			wantLen: MaxMessageLength,
			reason:  "the ceiling is inclusive, so an exact fit is not a cut",
		},
		{
			name:    "value over the limit is cut and marked",
			value:   strings.Repeat("a", MaxMessageLength+1),
			limit:   MaxMessageLength,
			wantLen: MaxMessageLength + 1,
			wantCut: true,
			reason:  "a reader must be able to tell a cut value from a short one",
		},
		{
			name:    "multi-byte runes are never split",
			value:   strings.Repeat("é", MaxMessageLength+10),
			limit:   MaxMessageLength,
			wantLen: MaxMessageLength + 1,
			wantCut: true,
			reason:  "counting bytes would emit invalid UTF-8 and break the JSON encoder",
		},
		{
			name:    "a non-positive limit disables truncation",
			value:   strings.Repeat("a", 5000),
			limit:   0,
			wantLen: 5000,
			reason:  "an unset ceiling must mean no ceiling rather than an empty value",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.value, tc.limit)
			if length := len([]rune(got)); length != tc.wantLen {
				t.Errorf("length = %d runes, want %d: %s", length, tc.wantLen, tc.reason)
			}
			if cut := strings.HasSuffix(got, TruncationMarker); cut != tc.wantCut {
				t.Errorf("truncation marker present = %v, want %v: %s", cut, tc.wantCut, tc.reason)
			}
			if !strings.ContainsRune(got, []rune(tc.value)[0]) {
				t.Errorf("result %q dropped the original content: %s", got, tc.reason)
			}
		})
	}
}

func TestSanitizeStageFourCeilingsByFieldKind(t *testing.T) {
	report := sanitizeReport{
		URL:     "https://redxt.example.test/" + strings.Repeat("a", 400),
		Message: strings.Repeat("b", 400),
		Stack:   strings.Repeat("c", MaxStackBytes+400),
	}
	out := sanitizeAsMap(t, &Sanitizer{}, report)

	cases := []struct {
		field   string
		wantLen int
		reason  string
	}{
		{"url", MaxURLLength + 1, "a URL field gets the 256 rune ceiling"},
		{"message", MaxMessageLength + 1, "a message field gets the 200 rune ceiling"},
		{"stack_trace", MaxStackBytes + 1, "a stack field gets the 2 KB ceiling"},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			value, ok := out[tc.field].(string)
			if !ok {
				t.Fatalf("field %q is %T, want string", tc.field, out[tc.field])
			}
			if length := len([]rune(value)); length != tc.wantLen {
				t.Errorf("length = %d runes, want %d: %s", length, tc.wantLen, tc.reason)
			}
		})
	}
}

func TestSanitizeStageFiveDependsOnTheDebugFlagOnly(t *testing.T) {
	cases := []struct {
		name    string
		debug   bool
		wantHas bool
		reason  string
	}{
		{
			name:   "production strips dev_only fields",
			reason: "the debug flag is off, so troubleshooting fields must never leave the server",
		},
		{
			name:    "the debug flag keeps dev_only fields",
			debug:   true,
			wantHas: true,
			reason:  "an operator who asked for debug output must actually get it",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Sanitizer{Debug: tc.debug}
			out := sanitizeAsMap(t, s, sanitizeAccount{ID: "u1", Internal: "row-42", Debug: "timing"})

			for _, key := range []string{"_internal_id", "_debug"} {
				if _, ok := out[key]; ok != tc.wantHas {
					t.Errorf("field %q present = %v, want %v: %s", key, ok, tc.wantHas, tc.reason)
				}
			}
			if _, ok := out["id"]; !ok {
				t.Errorf("field %q missing from %v: stage 5 must only drop the tagged fields", "id", out)
			}
		})
	}
}

func TestSanitizeStageFiveAppliesToMapKeys(t *testing.T) {
	payload := map[string]any{"id": "u1", "_debug": "timing", "_internal_id": "row-42"}

	production := sanitizeAsMap(t, &Sanitizer{}, payload)
	if _, ok := production["_debug"]; ok {
		t.Errorf("map key %q survived in %v: a map has no struct tags, so the underscore convention carries the marking", "_debug", production)
	}
	if _, ok := production["id"]; !ok {
		t.Errorf("map key %q missing from %v: an ordinary key must survive", "id", production)
	}

	debug := sanitizeAsMap(t, &Sanitizer{Debug: true}, payload)
	if _, ok := debug["_debug"]; !ok {
		t.Errorf("map key %q missing from %v: the debug flag must keep it", "_debug", debug)
	}
}

func TestSanitizeStageSixPadsAuthResponses(t *testing.T) {
	started := time.Date(2026, time.March, 14, 9, 26, 53, 0, time.UTC)

	cases := []struct {
		name      string
		minimum   time.Duration
		elapsed   time.Duration
		wantSleep time.Duration
		reason    string
	}{
		{
			name:      "a fast rejection is padded to the floor",
			elapsed:   40 * time.Millisecond,
			wantSleep: 60 * time.Millisecond,
			reason:    "a missing account rejects faster than a wrong password, and that difference is the leak",
		},
		{
			name:      "a slow success is not delayed further",
			elapsed:   150 * time.Millisecond,
			wantSleep: 0,
			reason:    "the floor is a minimum, not a fixed cost added to every login",
		},
		{
			name:      "reaching the floor exactly does not sleep",
			elapsed:   DefaultMinAuthDuration,
			wantSleep: 0,
			reason:    "a zero-length sleep is a wasted syscall and would still be correct only by accident",
		},
		{
			name:      "a configured floor overrides the default",
			minimum:   250 * time.Millisecond,
			elapsed:   40 * time.Millisecond,
			wantSleep: 210 * time.Millisecond,
			reason:    "an operator on slow hardware must be able to raise the floor above 100ms",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slept := time.Duration(-1)
			s := &Sanitizer{
				MinAuthDuration: tc.minimum,
				Now:             func() time.Time { return started.Add(tc.elapsed) },
				Sleep:           func(d time.Duration) { slept = d },
			}
			s.FinalizeAuth(started)

			if tc.wantSleep == 0 {
				if slept != -1 {
					t.Errorf("slept %s, want no sleep at all: %s", slept, tc.reason)
				}
				return
			}
			if slept != tc.wantSleep {
				t.Errorf("slept %s, want %s: %s", slept, tc.wantSleep, tc.reason)
			}
		})
	}
}

func TestSanitizeMiddlewareAttachesThePipeline(t *testing.T) {
	configured := &Sanitizer{Debug: true}

	var got *Sanitizer
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got = SanitizerFromContext(req.Context())
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	SanitizeMiddleware(configured)(handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/records", nil))

	if got != configured {
		t.Errorf("handler got %v, want the configured pipeline: a handler must not build its own with different settings", got)
	}
}

func TestSanitizeMiddlewarePassesThroughWhenUnset(t *testing.T) {
	rec := httptest.NewRecorder()
	SanitizeMiddleware(nil)(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/records", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: an unconfigured stage must not break the chain", rec.Code, http.StatusOK)
	}
}

func TestSanitizerFromContextFallsBackToProduction(t *testing.T) {
	s := SanitizerFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	if s == nil {
		t.Fatal("SanitizerFromContext returned nil: a handler running outside the chain must still get a pipeline")
	}
	if s.Debug {
		t.Error("fallback pipeline has Debug set: the fallback must never be more permissive than production")
	}
}

func TestSensitiveParamsIsSortedAndComplete(t *testing.T) {
	got := SensitiveParams()
	want := []string{
		"access_token", "api_key", "apikey", "auth", "code", "key",
		"password", "pwd", "refresh_token", "secret", "session", "token",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("SensitiveParams() = %v, want %v: the admin panel lists these, so the order must be stable and the set complete", got, want)
	}
}
