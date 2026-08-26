package logging

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// testTime is the fixed timestamp every format test renders, chosen to
// match the AI.md PART 11 examples.
var testTime = time.Date(2024, time.October, 10, 13, 55, 36, 0, time.FixedZone("MST", -7*3600))

// sampleEntry is a fully populated access record.
func sampleEntry() Entry {
	return Entry{
		Time:       testTime,
		RemoteIP:   "127.0.0.1",
		Method:     "GET",
		Path:       "/api/v1/server/healthz",
		Status:     200,
		Bytes:      2326,
		Latency:    1500 * time.Microsecond,
		UserAgent:  "curl/7.64.1",
		RequestID:  "req-1",
		FQDN:       "dns.example.com",
		Protocol:   "HTTP/1.1",
		TLSVersion: "TLS1.3",
		Country:    "US",
		ASN:        "AS64500",
	}
}

func TestFormatApache(t *testing.T) {
	tests := []struct {
		name  string
		entry func() Entry
		want  string
	}{
		{
			name:  "full entry",
			entry: sampleEntry,
			want:  `127.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/server/healthz HTTP/1.1" 200 2326 "-" "curl/7.64.1"`,
		},
		{
			name: "empty fields become dashes",
			entry: func() Entry {
				return Entry{Time: testTime}
			},
			want: `- - - [10/Oct/2024:13:55:36 -0700] "- - -" - - "-" "-"`,
		},
		{
			name: "query string is part of the request target",
			entry: func() Entry {
				e := sampleEntry()
				e.Query = "verbose=1"
				return e
			},
			want: `127.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/server/healthz?verbose=1 HTTP/1.1" 200 2326 "-" "curl/7.64.1"`,
		},
		{
			name: "control characters cannot forge a line",
			entry: func() Entry {
				e := sampleEntry()
				e.UserAgent = "evil\n127.0.0.1 - - injected"
				return e
			},
			want: `127.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/server/healthz HTTP/1.1" 200 2326 "-" "evil127.0.0.1 - - injected"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatApache(tc.entry()); got != tc.want {
				t.Errorf("FormatApache() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatNginx(t *testing.T) {
	want := `127.0.0.1 - - [10/Oct/2024:13:55:36 -0700] "GET /api/v1/server/healthz HTTP/1.1" 200 2326`
	if got := FormatNginx(sampleEntry()); got != want {
		t.Errorf("FormatNginx() = %q, want %q", got, want)
	}
}

func TestFormatAccessJSON(t *testing.T) {
	got, err := FormatAccessJSON(sampleEntry())
	if err != nil {
		t.Fatalf("FormatAccessJSON() error = %v", err)
	}
	want := `{"ip":"127.0.0.1","time":"2024-10-10T20:55:36Z","method":"GET","path":"/api/v1/server/healthz","status":200,"size":2326,"ua":"curl/7.64.1"}`
	if string(got) != want {
		t.Errorf("FormatAccessJSON() = %s, want %s", got, want)
	}
}

func TestFormatCustom(t *testing.T) {
	hostile := sampleEntry()
	hostile.UserAgent = "{method} {path}"

	tests := []struct {
		name  string
		tmpl  string
		entry Entry
		want  string
	}{
		{
			name:  "every field kind",
			tmpl:  "{remote_ip} {method} {path} {status} {bytes} {latency_ms} {request_id} {fqdn} {country} {asn}",
			entry: sampleEntry(),
			want:  "127.0.0.1 GET /api/v1/server/healthz 200 2326 1 req-1 dns.example.com US AS64500",
		},
		{
			name:  "unknown token stays verbatim",
			tmpl:  "{method} {not_a_variable} {path}",
			entry: sampleEntry(),
			want:  "GET {not_a_variable} /api/v1/server/healthz",
		},
		{
			name:  "unterminated token stays verbatim",
			tmpl:  "{method} {path",
			entry: sampleEntry(),
			want:  "GET {path",
		},
		{
			name:  "substituted value is never re-expanded",
			tmpl:  "{user_agent}",
			entry: hostile,
			want:  "{method} {path}",
		},
		{
			name:  "date and time variables",
			tmpl:  "{date}T{time} {datetime}",
			entry: sampleEntry(),
			want:  "2024-10-10T13:55:36 2024-10-10 13:55:36",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatCustom(tc.tmpl, tc.entry); got != tc.want {
				t.Errorf("FormatCustom() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatText(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
		msg   string
		attrs []slog.Attr
		want  string
	}{
		{
			name:  "plain message",
			level: slog.LevelInfo,
			msg:   "Server started on :8080",
			want:  "2024-10-10T13:55:36-07:00 [INFO] Server started on :8080",
		},
		{
			name:  "attributes are sorted and redacted",
			level: slog.LevelError,
			msg:   "login failed",
			attrs: []slog.Attr{slog.String("password", "hunter2"), slog.Int("attempt", 3)},
			want:  "2024-10-10T13:55:36-07:00 [ERROR] login failed attempt=3 password=xxxxx",
		},
		{
			name:  "grouped attributes flatten to dotted keys",
			level: slog.LevelWarn,
			msg:   "rate limited",
			attrs: []slog.Attr{slog.Group("client", slog.String("ip", "10.0.0.1"))},
			want:  "2024-10-10T13:55:36-07:00 [WARN] rate limited client.ip=10.0.0.1",
		},
		{
			name:  "values with spaces are quoted",
			level: slog.LevelInfo,
			msg:   "zone loaded",
			attrs: []slog.Attr{slog.String("zone", "example com")},
			want:  `2024-10-10T13:55:36-07:00 [INFO] zone loaded zone="example com"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatText(testTime, tc.level, tc.msg, tc.attrs); got != tc.want {
				t.Errorf("FormatText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatJSON(t *testing.T) {
	got, err := FormatJSON(testTime, slog.LevelError, "database connection failed", []slog.Attr{
		slog.String("error", "timeout"),
		slog.String("api_key", "abcdef"),
	})
	if err != nil {
		t.Fatalf("FormatJSON() error = %v", err)
	}
	want := `{"time":"2024-10-10T20:55:36.000Z","level":"ERROR","msg":"database connection failed","api_key":"xxxxx","error":"timeout"}`
	if string(got) != want {
		t.Errorf("FormatJSON() = %s, want %s", got, want)
	}
}

func TestFormatSecurityFormats(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "fail2ban",
			got:  FormatFail2ban(testTime, "Rate limit exceeded from 192.168.1.100"),
			want: "2024-10-10T13:55:36-07:00 [security] Rate limit exceeded from 192.168.1.100",
		},
		{
			name: "syslog",
			got:  FormatSyslog(testTime, slog.LevelWarn, "dns1", CEFProduct, "brute force detected"),
			want: "<36>1 2024-10-10T13:55:36.000-07:00 dns1 redxt - - brute force detected",
		},
		{
			name: "syslog with error level and no hostname",
			got:  FormatSyslog(testTime, slog.LevelError, "", "", "boom"),
			want: "<35>1 2024-10-10T13:55:36.000-07:00 - - - - boom",
		},
		{
			name: "cef",
			got: FormatCEF("1.2.3", "security.ip_blocked", "IP blocked", cefDefaultSeverity, map[string]string{
				"src":   "192.168.1.100",
				"token": "abcdef123456",
			}),
			want: "CEF:0|webappsgo|redxt|1.2.3|security.ip_blocked|IP blocked|5|src=192.168.1.100 token=xxxxx",
		},
		{
			name: "cef header separators are escaped",
			got:  FormatCEF("1.0", "a|b", `c\d`, 3, nil),
			want: `CEF:0|webappsgo|redxt|1.0|a\|b|c\\d|3|`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain ascii is untouched", in: "GET /healthz", want: "GET /healthz"},
		{name: "empty stays empty", in: "", want: ""},
		{name: "ansi color is stripped", in: "\x1b[31mred\x1b[0m", want: "red"},
		{name: "newline and tab are removed", in: "a\nb\tc", want: "abc"},
		{
			name: "crafted line injection is defused",
			in:   "ok\x1b[1m\n2024-10-10T13:55:36-07:00 [INFO] forged",
			want: "ok2024-10-10T13:55:36-07:00 [INFO] forged",
		},
		{name: "osc sequence is stripped", in: "a\x1b]0;title\x07b", want: "ab"},
		{name: "non ascii is dropped", in: "café", want: "caf"},
		{name: "emoji is dropped", in: "ok ✅", want: "ok "},
		{name: "carriage return is removed", in: "a\r\nb", want: "ab"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.in)
			if got != tc.want {
				t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.ContainsAny(got, "\n\r\t\x1b") {
				t.Errorf("sanitize(%q) left a control character in %q", tc.in, got)
			}
		})
	}
}
