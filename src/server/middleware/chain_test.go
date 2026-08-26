package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/logging"
)

// okHandler is the terminal handler every chain test wraps. It records
// nothing and answers 200 so a test asserting on a rejection can tell a
// short-circuit from a pass-through.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

// recorderMiddleware returns a middleware that appends name to trace
// when it runs, so a test can assert the exact execution order.
func recorderMiddleware(trace *[]string, name string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			*trace = append(*trace, name)
			next.ServeHTTP(w, req)
		})
	}
}

func TestChainExecutionOrder(t *testing.T) {
	cases := []struct {
		name   string
		names  []string
		want   string
		reason string
	}{
		{
			name:   "three stages run first to last",
			names:  []string{"a", "b", "c"},
			want:   "a,b,c",
			reason: "Chain must preserve argument order so the PART 12 numbered list reads the same as the code",
		},
		{
			name:   "single stage",
			names:  []string{"only"},
			want:   "only",
			reason: "a one element chain must still wrap the handler",
		},
		{
			name:   "empty chain",
			names:  nil,
			want:   "",
			reason: "an empty chain is the identity and must still reach the handler",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trace := []string{}
			stages := make([]Middleware, 0, len(tc.names))
			for _, name := range tc.names {
				stages = append(stages, recorderMiddleware(&trace, name))
			}

			rec := httptest.NewRecorder()
			Chain(stages...)(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if got := strings.Join(trace, ","); got != tc.want {
				t.Errorf("order = %q, want %q: %s", got, tc.want, tc.reason)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d: the chain must reach the handler", rec.Code, http.StatusOK)
			}
		})
	}
}

func TestChainSkipsNilStages(t *testing.T) {
	trace := []string{}
	chain := Chain(nil, recorderMiddleware(&trace, "a"), nil, recorderMiddleware(&trace, "b"), nil)

	rec := httptest.NewRecorder()
	chain(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := strings.Join(trace, ","); got != "a,b" {
		t.Errorf("order = %q, want %q: a nil stage must be skipped, not panic", got, "a,b")
	}
}

func TestChainShortCircuitStopsLaterStages(t *testing.T) {
	trace := []string{}
	blocker := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			trace = append(trace, "blocker")
			w.WriteHeader(http.StatusForbidden)
		})
	}
	chain := Chain(recorderMiddleware(&trace, "before"), blocker, recorderMiddleware(&trace, "after"))

	rec := httptest.NewRecorder()
	chain(okHandler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := strings.Join(trace, ","); got != "before,blocker" {
		t.Errorf("order = %q, want %q: a stage that answers must not run the stages after it", got, "before,blocker")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: the short-circuiting stage owns the response", rec.Code, http.StatusForbidden)
	}
}

func TestNewRunsTheFullStackInPartTwelveOrder(t *testing.T) {
	store := NewMemoryStore()
	opts := Options{}
	opts.CORS.Configured = "https://example.test"
	opts.CSRF.Enabled = true
	opts.Allowlist.Enabled = true
	opts.Allowlist.Contains = func(string) bool { return false }
	opts.Blocklist.Enabled = true
	opts.Blocklist.Contains = func(string) bool { return false }
	opts.RateLimit.Config.Enabled = true
	opts.RateLimit.Store = store
	opts.GeoIP.Lookup = func(string) (GeoResult, bool) { return GeoResult{Country: "US"}, true }

	logged := []string{}
	opts.Logging.Sink = func(entry logging.Entry) { logged = append(logged, entry.Path) }

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	req.RemoteAddr = "203.0.113.9:41000"
	New(opts)(okHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: an ordinary request must survive the whole chain", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(HeaderContentTypeOptions); got != ValueNoSniff {
		t.Errorf("%s = %q, want %q: the header stage must have run", HeaderContentTypeOptions, got, ValueNoSniff)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID is empty: the request ID stage must have run")
	}
	if len(logged) != 1 || logged[0] != "/records" {
		t.Errorf("logged = %v, want one entry for /records: the log stage must have run innermost", logged)
	}
}
