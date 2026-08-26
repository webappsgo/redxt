package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/config"
)

// Rate-limit bucket names. They are stored verbatim in the bucket column
// of the rate_limits table, so a cluster shares one budget per client
// per class rather than one per node.
const (
	// BucketRead limits GET and HEAD.
	BucketRead = "read"
	// BucketWrite limits POST, PUT, PATCH and DELETE.
	BucketWrite = "write"
	// BucketHealth limits the health and status endpoints, which
	// monitoring polls far more often than a human browses.
	BucketHealth = "health"
	// BucketGlobal is the per-IP ceiling across every other class.
	BucketGlobal = "global"
	// BucketLogin limits login attempts.
	BucketLogin = "login"
	// BucketPasswordReset limits password reset requests.
	BucketPasswordReset = "password_reset"
	// BucketRegistration limits account registrations.
	BucketRegistration = "registration"
)

// PART 12 rate-limit defaults, applied when server.yml leaves a bucket
// unset. They are stated as a request count and a window length.
const (
	// DefaultReadRequests is the read budget per DefaultWindow.
	DefaultReadRequests = 120
	// DefaultWriteRequests is the write budget per DefaultWindow.
	DefaultWriteRequests = 10
	// DefaultHealthRequests is the health budget per DefaultWindow.
	DefaultHealthRequests = 120
	// DefaultGlobalBurst is the per-IP ceiling across all classes, per
	// DefaultWindow.
	DefaultGlobalBurst = 240
	// DefaultWindow is the window the general buckets use.
	DefaultWindow = time.Minute
	// DefaultLoginRequests is the login budget per DefaultLoginWindow.
	DefaultLoginRequests = 5
	// DefaultLoginWindow is the login window, 15 minutes.
	DefaultLoginWindow = 15 * time.Minute
	// DefaultPasswordResetRequests is the reset budget per hour.
	DefaultPasswordResetRequests = 3
	// DefaultRegistrationRequests is the registration budget per hour.
	DefaultRegistrationRequests = 5
	// DefaultAuthLongWindow is the window the reset and registration
	// buckets use, one hour.
	DefaultAuthLongWindow = time.Hour
)

// defaultHealthPaths are the endpoints billed to the health bucket.
var defaultHealthPaths = []string{"/healthz", "/readyz", "/livez", "/server/healthz", "/server/status"}

// Decision is one rate-limit verdict.
type Decision struct {
	// Allowed reports whether the request may proceed.
	Allowed bool
	// Bucket names the rule that produced the verdict.
	Bucket string
	// Limit is the bucket's request budget.
	Limit int
	// Remaining is the budget left after this request, never negative.
	Remaining int
	// RetryAfter is how long the client must wait before the budget
	// frees up. It is zero on an allowed request.
	RetryAfter time.Duration
}

// RateLimitStore records and evaluates the sliding windows.
//
// Allow counts one request against a bucket and returns the verdict. It
// must be safe for concurrent use. An implementation that fails — a
// database that is briefly unreachable, say — returns an error rather
// than a verdict, and the middleware lets the request through: a broken
// counter must not take the whole server offline.
type RateLimitStore interface {
	Allow(identifier, bucket string, limit int, window time.Duration, now time.Time) (Decision, error)
}

// AuthRateRule maps a path prefix to one of the stricter authentication
// buckets. The first matching rule wins, so list the more specific
// prefixes first.
type AuthRateRule struct {
	// Prefix is the path prefix the rule covers, matched on segment
	// boundaries.
	Prefix string
	// Bucket is the bucket to bill matching requests to.
	Bucket string
}

// RateLimitOptions configures the PART 12 rate-limit stage.
type RateLimitOptions struct {
	// Config carries the server.rate_limit.* settings. Buckets left at
	// zero fall back to the PART 12 defaults.
	Config config.RateLimit

	// Store persists the sliding windows. Nil disables enforcement.
	Store RateLimitStore

	// ClientIP resolves the trusted-proxy-aware client address, which is
	// the identifier every bucket is keyed by. Nil falls back to the
	// request's TCP peer.
	ClientIP func(*http.Request) string

	// AuthRules maps authentication paths to their stricter buckets.
	AuthRules []AuthRateRule

	// HealthPaths overrides the endpoints billed to the health bucket.
	HealthPaths []string

	// Now supplies the current time. Nil uses time.Now. Tests inject a
	// clock here so a window boundary is exact rather than raced.
	Now func() time.Time

	// OnError is called when the store fails. The request is allowed
	// through regardless; this hook exists so the failure is visible in
	// the logs. Nil discards the error.
	OnError func(*http.Request, error)
}

// rateLimitBody is the PART 14 envelope for a 429. It is a dedicated
// type rather than the shared apierror.Response because retry_after is
// specific to this rejection.
type rateLimitBody struct {
	// OK is always false.
	OK bool `json:"ok"`
	// Error is the machine-readable code.
	Error string `json:"error"`
	// Message is the human-readable summary.
	Message string `json:"message"`
	// RetryAfter is the wait in whole seconds, matching the Retry-After
	// header.
	RetryAfter int `json:"retry_after"`
}

// RateLimit returns the PART 12 rate-limit middleware.
//
// Every request is billed to its class bucket — read, write, health, or
// one of the stricter authentication buckets — and, unless it landed in
// an authentication bucket, to the per-IP global burst ceiling as well.
// Authentication paths are exempt from the global ceiling because their
// own budgets are already far tighter than it.
//
// The identifier is the trusted-proxy-aware client IP, so a client
// behind a reverse proxy is limited as itself rather than as the proxy.
// Allow-listed clients skip the stage entirely.
//
// A rejection is a 429 carrying Retry-After and the PART 14 envelope. A
// store failure is not a rejection: the request proceeds and OnError is
// notified, because an unavailable counter must degrade to unlimited
// rather than to unavailable.
func RateLimit(opts RateLimitOptions) Middleware {
	if !opts.Config.Enabled || opts.Store == nil {
		return passthrough
	}
	clientIP := opts.ClientIP
	if clientIP == nil {
		clientIP = remoteHost
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	healthPaths := opts.HealthPaths
	if len(healthPaths) == 0 {
		healthPaths = defaultHealthPaths
	}
	global := globalBucket(opts.Config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if IsAllowlisted(req.Context()) {
				next.ServeHTTP(w, req)
				return
			}

			identifier := clientIP(req)
			at := now()
			bucket, limit, window, isAuth := classify(opts, healthPaths, req)

			decision, err := opts.Store.Allow(identifier, bucket, limit, window, at)
			if err != nil {
				reportRateLimitError(opts, req, err)
				next.ServeHTTP(w, req)
				return
			}
			if !decision.Allowed {
				writeRateLimited(w, decision)
				return
			}

			if !isAuth && global.Requests > 0 {
				globalDecision, gerr := opts.Store.Allow(identifier, BucketGlobal, global.Requests, global.Window.Duration(), at)
				if gerr != nil {
					reportRateLimitError(opts, req, gerr)
					next.ServeHTTP(w, req)
					return
				}
				if !globalDecision.Allowed {
					writeRateLimited(w, globalDecision)
					return
				}
			}

			next.ServeHTTP(w, req)
		})
	}
}

// classify picks the bucket a request is billed to and that bucket's
// budget. Authentication paths win over the method-based classes: a
// login POST is a login attempt first and a write second.
func classify(opts RateLimitOptions, healthPaths []string, req *http.Request) (bucket string, limit int, window time.Duration, isAuth bool) {
	for _, rule := range opts.AuthRules {
		if matchesPathPrefix(req.URL.Path, rule.Prefix) {
			b := authBucket(opts.Config.Auth, rule.Bucket)
			return rule.Bucket, b.Requests, b.Window.Duration(), true
		}
	}
	if matchesAnyPath(req.URL.Path, healthPaths) {
		b := resolveBucket(opts.Config.Health, DefaultHealthRequests, DefaultWindow)
		return BucketHealth, b.Requests, b.Window.Duration(), false
	}
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		b := resolveBucket(opts.Config.Read, DefaultReadRequests, DefaultWindow)
		return BucketRead, b.Requests, b.Window.Duration(), false
	}
	b := resolveBucket(opts.Config.Write, DefaultWriteRequests, DefaultWindow)
	return BucketWrite, b.Requests, b.Window.Duration(), false
}

// authBucket returns the configured budget for one of the stricter
// authentication buckets, falling back to the PART 12 defaults. An
// unrecognized bucket name is treated as a login, which is the tightest
// of the three and therefore the safe default.
func authBucket(cfg config.AuthRateLimit, bucket string) config.RateBucket {
	switch bucket {
	case BucketPasswordReset:
		return resolveBucket(cfg.PasswordReset, DefaultPasswordResetRequests, DefaultAuthLongWindow)
	case BucketRegistration:
		return resolveBucket(cfg.Registration, DefaultRegistrationRequests, DefaultAuthLongWindow)
	default:
		return resolveBucket(cfg.Login, DefaultLoginRequests, DefaultLoginWindow)
	}
}

// globalBucket returns the per-IP ceiling across all classes.
func globalBucket(cfg config.RateLimit) config.RateBucket {
	requests := cfg.GlobalBurst
	if requests <= 0 {
		requests = DefaultGlobalBurst
	}
	return config.RateBucket{Requests: requests, Window: config.Duration(DefaultWindow)}
}

// resolveBucket fills a bucket's unset fields from the PART 12 defaults.
func resolveBucket(bucket config.RateBucket, requests int, window time.Duration) config.RateBucket {
	if bucket.Requests <= 0 {
		bucket.Requests = requests
	}
	if bucket.Window.Duration() <= 0 {
		bucket.Window = config.Duration(window)
	}
	return bucket
}

// writeRateLimited sends the PART 14 429 response: the Retry-After
// header browsers and well-behaved clients honor, and the JSON envelope
// every other rejection in the API uses.
func writeRateLimited(w http.ResponseWriter, decision Decision) {
	seconds := int(decision.RetryAfter / time.Second)
	if decision.RetryAfter%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Retry-After", strconv.Itoa(seconds))
	h.Set("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
	h.Set("X-RateLimit-Remaining", "0")
	w.WriteHeader(http.StatusTooManyRequests)

	_ = apierror.WriteJSON(w, rateLimitBody{
		OK:         false,
		Error:      "RATE_LIMITED",
		Message:    "Too many requests",
		RetryAfter: seconds,
	})
}

// reportRateLimitError hands a store failure to the configured hook.
func reportRateLimitError(opts RateLimitOptions, req *http.Request, err error) {
	if opts.OnError != nil {
		opts.OnError(req, err)
	}
}

// MemoryStore is an in-process sliding-window log. It is exact rather
// than approximate — it remembers each request's timestamp — which makes
// it the right store for a single-node deployment and for tests. A
// cluster needs SQLStore so every node reads one shared budget.
type MemoryStore struct {
	mu      sync.Mutex
	windows map[string][]time.Time
}

// NewMemoryStore returns an empty in-process store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{windows: make(map[string][]time.Time)}
}

// Allow counts one request against a bucket and returns the verdict.
//
// Timestamps older than the window are dropped on every call, so the
// map holds at most one window's worth of history per active key and
// needs no separate sweep. A rejected request is not recorded: a client
// that keeps hammering a closed window must not push its own retry time
// further out with every attempt.
func (s *MemoryStore) Allow(identifier, bucket string, limit int, window time.Duration, now time.Time) (Decision, error) {
	if limit <= 0 || window <= 0 {
		return Decision{Allowed: true, Bucket: bucket, Limit: limit}, nil
	}
	key := identifier + "\x00" + bucket
	cutoff := now.Add(-window)

	s.mu.Lock()
	defer s.mu.Unlock()

	stamps := s.windows[key]
	kept := stamps[:0]
	for _, stamp := range stamps {
		if stamp.After(cutoff) {
			kept = append(kept, stamp)
		}
	}

	if len(kept) >= limit {
		s.windows[key] = kept
		retry := kept[0].Add(window).Sub(now)
		if retry < 0 {
			retry = 0
		}
		return Decision{Allowed: false, Bucket: bucket, Limit: limit, RetryAfter: retry}, nil
	}

	kept = append(kept, now)
	s.windows[key] = kept
	return Decision{Allowed: true, Bucket: bucket, Limit: limit, Remaining: limit - len(kept)}, nil
}

// Purge drops every recorded request older than cutoff, and every key
// left with no requests at all. A long-running server calls it from the
// PART 19 scheduler with a cutoff one longest-window back, so keys for
// clients that stopped calling do not accumulate.
func (s *MemoryStore) Purge(cutoff time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, stamps := range s.windows {
		live := stamps[:0]
		for _, stamp := range stamps {
			if stamp.After(cutoff) {
				live = append(live, stamp)
			}
		}
		if len(live) == 0 {
			delete(s.windows, key)
			continue
		}
		s.windows[key] = live
	}
}
