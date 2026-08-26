package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// rateClock is a fixed instant every rate-limit test measures from. A
// literal is used rather than time.Now so a window boundary is exact
// instead of raced against the wall clock.
var rateClock = time.Date(2026, time.March, 14, 9, 26, 53, 0, time.UTC)

// recordingStore captures the calls the middleware makes and answers
// with a scripted verdict, so a test can assert which bucket a request
// was billed to without depending on any real counting.
type recordingStore struct {
	calls []string
	deny  map[string]Decision
	err   error
}

// Allow records the call and returns the scripted verdict for the
// bucket, defaulting to allowed.
func (s *recordingStore) Allow(identifier, bucket string, limit int, window time.Duration, _ time.Time) (Decision, error) {
	s.calls = append(s.calls, fmt.Sprintf("%s|%s|%d|%s", identifier, bucket, limit, window))
	if s.err != nil {
		return Decision{}, s.err
	}
	if decision, ok := s.deny[bucket]; ok {
		return decision, nil
	}
	return Decision{Allowed: true, Bucket: bucket, Limit: limit, Remaining: limit - 1}, nil
}

// rateOptions returns an enabled configuration wired to the store, with
// the clock frozen and the auth prefixes the server registers.
func rateOptions(store RateLimitStore) RateLimitOptions {
	return RateLimitOptions{
		Config: config.RateLimit{Enabled: true},
		Store:  store,
		Now:    func() time.Time { return rateClock },
		AuthRules: []AuthRateRule{
			{Prefix: "/api/v1/auth/login", Bucket: BucketLogin},
			{Prefix: "/api/v1/auth/password-reset", Bucket: BucketPasswordReset},
			{Prefix: "/api/v1/auth/register", Bucket: BucketRegistration},
		},
	}
}

// rateRequest builds a request from a fixed client address.
func rateRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "203.0.113.9:41000"
	return req
}

func TestMemoryStoreSlidingWindow(t *testing.T) {
	store := NewMemoryStore()
	const limit = 3
	const window = time.Minute

	for i := 1; i <= limit; i++ {
		decision, err := store.Allow("203.0.113.9", BucketWrite, limit, window, rateClock)
		if err != nil {
			t.Fatalf("Allow error = %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("request %d rejected: the first %d requests are inside the budget", i, limit)
		}
		if want := limit - i; decision.Remaining != want {
			t.Errorf("remaining after request %d = %d, want %d: the header must tell the client its true budget", i, decision.Remaining, want)
		}
	}

	decision, err := store.Allow("203.0.113.9", BucketWrite, limit, window, rateClock)
	if err != nil {
		t.Fatalf("Allow error = %v", err)
	}
	if decision.Allowed {
		t.Fatal("request over the budget was allowed: the window must close once the limit is reached")
	}
	if decision.RetryAfter != window {
		t.Errorf("retry after = %v, want %v: the oldest request leaves the window one full window after it arrived", decision.RetryAfter, window)
	}

	if again, _ := store.Allow("203.0.113.9", BucketWrite, limit, window, rateClock.Add(30*time.Second)); again.Allowed {
		t.Error("a request halfway through the window was allowed: the window slides, it does not reset at a fixed tick")
	}

	slid, err := store.Allow("203.0.113.9", BucketWrite, limit, window, rateClock.Add(window+time.Second))
	if err != nil {
		t.Fatalf("Allow error = %v", err)
	}
	if !slid.Allowed {
		t.Error("a request after the window elapsed was rejected: the oldest timestamps must age out")
	}
}

func TestMemoryStoreRejectionDoesNotExtendTheWindow(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Allow("203.0.113.9", BucketLogin, 1, time.Minute, rateClock); err != nil {
		t.Fatalf("Allow error = %v", err)
	}

	first, _ := store.Allow("203.0.113.9", BucketLogin, 1, time.Minute, rateClock.Add(10*time.Second))
	second, _ := store.Allow("203.0.113.9", BucketLogin, 1, time.Minute, rateClock.Add(20*time.Second))

	if first.Allowed || second.Allowed {
		t.Fatal("a request over the budget was allowed")
	}
	if first.RetryAfter != 50*time.Second || second.RetryAfter != 40*time.Second {
		t.Errorf("retry after = %v then %v, want 50s then 40s: hammering a closed window must not push the client's own retry time further out",
			first.RetryAfter, second.RetryAfter)
	}
}

func TestMemoryStoreKeysAreIndependent(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Allow("203.0.113.9", BucketWrite, 1, time.Minute, rateClock); err != nil {
		t.Fatalf("Allow error = %v", err)
	}

	otherBucket, _ := store.Allow("203.0.113.9", BucketRead, 1, time.Minute, rateClock)
	otherClient, _ := store.Allow("198.51.100.4", BucketWrite, 1, time.Minute, rateClock)

	if !otherBucket.Allowed {
		t.Error("a different bucket was charged: read and write hold separate budgets")
	}
	if !otherClient.Allowed {
		t.Error("a different client was charged: one client must never spend another's budget")
	}
}

func TestMemoryStoreUnlimitedBuckets(t *testing.T) {
	store := NewMemoryStore()

	cases := []struct {
		name   string
		limit  int
		window time.Duration
		reason string
	}{
		{"zero limit", 0, time.Minute, "an unset budget means no limiting, not a limit of zero"},
		{"zero window", 5, 0, "a bucket with no window cannot slide and must not reject"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 3; i++ {
				decision, err := store.Allow("203.0.113.9", "unlimited", tc.limit, tc.window, rateClock)
				if err != nil {
					t.Fatalf("Allow error = %v", err)
				}
				if !decision.Allowed {
					t.Fatalf("request %d rejected: %s", i, tc.reason)
				}
			}
		})
	}
}

func TestMemoryStorePurge(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Allow("203.0.113.9", BucketWrite, 1, time.Minute, rateClock); err != nil {
		t.Fatalf("Allow error = %v", err)
	}

	store.Purge(rateClock.Add(time.Hour))

	decision, _ := store.Allow("203.0.113.9", BucketWrite, 1, time.Minute, rateClock.Add(time.Hour))
	if !decision.Allowed {
		t.Error("the purged history still counted: a key for a client that stopped calling must not accumulate")
	}
}

func TestRateLimitBucketSelection(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		path      string
		wantFirst string
		reason    string
	}{
		{
			name:      "get is a read",
			method:    http.MethodGet,
			path:      "/api/v1/records",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketRead, DefaultReadRequests, DefaultWindow),
			reason:    "browsing is billed to the generous read budget",
		},
		{
			name:      "head is a read",
			method:    http.MethodHead,
			path:      "/api/v1/records",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketRead, DefaultReadRequests, DefaultWindow),
			reason:    "HEAD costs the server what a GET costs",
		},
		{
			name:      "post is a write",
			method:    http.MethodPost,
			path:      "/api/v1/records",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketWrite, DefaultWriteRequests, DefaultWindow),
			reason:    "a write is far more expensive and gets the tighter budget",
		},
		{
			name:      "delete is a write",
			method:    http.MethodDelete,
			path:      "/api/v1/records/1",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketWrite, DefaultWriteRequests, DefaultWindow),
			reason:    "every state-changing method shares the write budget",
		},
		{
			name:      "healthz is health",
			method:    http.MethodGet,
			path:      "/healthz",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketHealth, DefaultHealthRequests, DefaultWindow),
			reason:    "monitoring polls far more often than a human browses",
		},
		{
			name:      "readyz is health",
			method:    http.MethodGet,
			path:      "/readyz",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketHealth, DefaultHealthRequests, DefaultWindow),
			reason:    "the readiness probe is a health endpoint like the others",
		},
		{
			name:      "livez is health",
			method:    http.MethodGet,
			path:      "/livez",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketHealth, DefaultHealthRequests, DefaultWindow),
			reason:    "the liveness probe is a health endpoint like the others",
		},
		{
			name:      "login beats write",
			method:    http.MethodPost,
			path:      "/api/v1/auth/login",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketLogin, DefaultLoginRequests, DefaultLoginWindow),
			reason:    "a login POST is a credential-guessing attempt first and a write second",
		},
		{
			name:      "password reset has its own hourly budget",
			method:    http.MethodPost,
			path:      "/api/v1/auth/password-reset",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketPasswordReset, DefaultPasswordResetRequests, DefaultAuthLongWindow),
			reason:    "reset mail is expensive to send and trivial to abuse",
		},
		{
			name:      "registration has its own hourly budget",
			method:    http.MethodPost,
			path:      "/api/v1/auth/register",
			wantFirst: fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketRegistration, DefaultRegistrationRequests, DefaultAuthLongWindow),
			reason:    "mass signup is the cheapest abuse there is",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingStore{}
			rec := httptest.NewRecorder()
			RateLimit(rateOptions(store))(okHandler).ServeHTTP(rec, rateRequest(tc.method, tc.path))

			if len(store.calls) == 0 {
				t.Fatalf("no bucket was charged: %s", tc.reason)
			}
			if store.calls[0] != tc.wantFirst {
				t.Errorf("charged %q, want %q: %s", store.calls[0], tc.wantFirst, tc.reason)
			}
		})
	}
}

func TestRateLimitGlobalBurst(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		wantCalls int
		reason    string
	}{
		{
			name:      "an ordinary request also pays the global ceiling",
			path:      "/api/v1/records",
			wantCalls: 2,
			reason:    "the per-IP ceiling caps a client spreading its load across every class",
		},
		{
			name:      "an authentication request is exempt",
			path:      "/api/v1/auth/login",
			wantCalls: 1,
			reason:    "the login budget is already far tighter than the global ceiling",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingStore{}
			rec := httptest.NewRecorder()
			RateLimit(rateOptions(store))(okHandler).ServeHTTP(rec, rateRequest(http.MethodPost, tc.path))

			if len(store.calls) != tc.wantCalls {
				t.Fatalf("store calls = %v, want %d: %s", store.calls, tc.wantCalls, tc.reason)
			}
			if tc.wantCalls == 2 {
				want := fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketGlobal, DefaultGlobalBurst, DefaultWindow)
				if store.calls[1] != want {
					t.Errorf("second charge = %q, want %q: %s", store.calls[1], want, tc.reason)
				}
			}
		})
	}
}

func TestRateLimitDisabledAndBypassed(t *testing.T) {
	cases := []struct {
		name      string
		opts      func(*recordingStore) RateLimitOptions
		wrap      func(Middleware) Middleware
		wantCalls int
		reason    string
	}{
		{
			name: "disabled config charges nothing",
			opts: func(s *recordingStore) RateLimitOptions {
				opts := rateOptions(s)
				opts.Config.Enabled = false
				return opts
			},
			reason: "an operator who turned limiting off must not pay for a counter lookup",
		},
		{
			name: "a nil store charges nothing",
			opts: func(*recordingStore) RateLimitOptions {
				opts := rateOptions(nil)
				opts.Store = nil
				return opts
			},
			reason: "without somewhere to count, enforcement is impossible and must not be faked",
		},
		{
			name: "an allow-listed client is skipped",
			opts: func(s *recordingStore) RateLimitOptions { return rateOptions(s) },
			wrap: func(next Middleware) Middleware {
				allow := Allowlist(AllowlistOptions{Enabled: true, Contains: func(string) bool { return true }})
				return Chain(allow, next)
			},
			reason: "an operator's own monitoring host must not be throttled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingStore{}
			stage := RateLimit(tc.opts(store))
			if tc.wrap != nil {
				stage = tc.wrap(stage)
			}

			rec := httptest.NewRecorder()
			stage(okHandler).ServeHTTP(rec, rateRequest(http.MethodPost, "/api/v1/records"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, tc.reason)
			}
			if len(store.calls) != tc.wantCalls {
				t.Errorf("store calls = %v, want %d: %s", store.calls, tc.wantCalls, tc.reason)
			}
		})
	}
}

func TestRateLimitStoreFailureAllowsTheRequest(t *testing.T) {
	failure := errors.New("counter unavailable")
	store := &recordingStore{err: failure}

	var reported error
	opts := rateOptions(store)
	opts.OnError = func(_ *http.Request, err error) { reported = err }

	rec := httptest.NewRecorder()
	RateLimit(opts)(okHandler).ServeHTTP(rec, rateRequest(http.MethodPost, "/api/v1/records"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: an unavailable counter must degrade to unlimited, not to unavailable", rec.Code, http.StatusOK)
	}
	if !errors.Is(reported, failure) {
		t.Errorf("OnError got %v, want %v: a silent counter failure would hide the outage", reported, failure)
	}
}

func TestRateLimitRejection(t *testing.T) {
	store := &recordingStore{
		deny: map[string]Decision{
			BucketWrite: {Allowed: false, Bucket: BucketWrite, Limit: DefaultWriteRequests, RetryAfter: time.Minute},
		},
	}

	reached := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	RateLimit(rateOptions(store))(handler).ServeHTTP(rec, rateRequest(http.MethodPost, "/api/v1/records"))

	if reached {
		t.Error("the handler ran: a rejected request must cost no work at all")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want %q: a client that cannot see the wait will simply retry immediately", got, "60")
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", got, "0")
	}

	want := "{\n  \"ok\": false,\n  \"error\": \"RATE_LIMITED\",\n  \"message\": \"Too many requests\",\n  \"retry_after\": 60\n}\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body =\n%q\nwant\n%q: the 429 envelope is fixed by PART 12", got, want)
	}
}

func TestRateLimitRoundsRetryAfterUp(t *testing.T) {
	cases := []struct {
		name   string
		retry  time.Duration
		want   string
		reason string
	}{
		{"whole seconds", 30 * time.Second, "30", "an exact wait needs no adjustment"},
		{"fraction rounds up", 1500 * time.Millisecond, "2", "rounding down would invite a retry the server still rejects"},
		{"sub-second becomes one", 10 * time.Millisecond, "1", "Retry-After has one-second resolution and zero would mean retry now"},
		{"zero becomes one", 0, "1", "a client told to wait zero seconds is told nothing"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingStore{
				deny: map[string]Decision{
					BucketRead: {Allowed: false, Bucket: BucketRead, Limit: DefaultReadRequests, RetryAfter: tc.retry},
				},
			}

			rec := httptest.NewRecorder()
			RateLimit(rateOptions(store))(okHandler).ServeHTTP(rec, rateRequest(http.MethodGet, "/api/v1/records"))

			if got := rec.Header().Get("Retry-After"); got != tc.want {
				t.Errorf("Retry-After = %q, want %q: %s", got, tc.want, tc.reason)
			}
		})
	}
}

func TestRateLimitConfiguredBucketsOverrideTheDefaults(t *testing.T) {
	store := &recordingStore{}
	opts := rateOptions(store)
	opts.Config.Read = config.RateBucket{Requests: 7, Window: config.Duration(30 * time.Second)}
	opts.Config.GlobalBurst = 9

	rec := httptest.NewRecorder()
	RateLimit(opts)(okHandler).ServeHTTP(rec, rateRequest(http.MethodGet, "/api/v1/records"))

	want := []string{
		fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketRead, 7, 30*time.Second),
		fmt.Sprintf("203.0.113.9|%s|%d|%s", BucketGlobal, 9, DefaultWindow),
	}
	if len(store.calls) != len(want) || store.calls[0] != want[0] || store.calls[1] != want[1] {
		t.Errorf("charges = %v, want %v: a configured bucket must win over the shipped default", store.calls, want)
	}
}
