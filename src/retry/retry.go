// Package retry implements the transient-failure retry and exponential
// backoff policy defined by AI.md PART 9 (ERROR HANDLING & CACHING).
//
// The default schedule waits 0s, 1s, 2s, 4s and 8s between five attempts, with
// any individual wait capped at 30 seconds. Only transient failures are
// retried; deterministic client errors are returned to the caller unchanged.
package retry

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"
)

// DefaultBackoff is the PART 9 wait schedule: the first attempt runs
// immediately, then each retry waits progressively longer.
var DefaultBackoff = []time.Duration{0, 1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// MaxBackoff is the ceiling applied to any single wait.
const MaxBackoff = 30 * time.Second

// StatusCoder is implemented by errors that carry an HTTP status code. It lets
// error types opt into retry classification without this package importing
// them, which would create an import cycle.
type StatusCoder interface {
	HTTPStatusCode() int
}

// Policy describes how an operation is retried.
type Policy struct {
	// Backoff is the per-attempt wait schedule. Its length sets the maximum
	// number of attempts. An empty schedule falls back to DefaultBackoff.
	Backoff []time.Duration
	// Max caps any individual wait.
	Max time.Duration
	// Retryable decides whether a failure is transient. A nil value falls back
	// to IsRetryable.
	Retryable func(error) bool
}

// DefaultPolicy returns the PART 9 policy: the default schedule, the 30 second
// ceiling, and the default transient-error classification.
func DefaultPolicy() Policy {
	return Policy{
		Backoff:   DefaultBackoff,
		Max:       MaxBackoff,
		Retryable: IsRetryable,
	}
}

// Do runs fn under the default policy.
func Do(ctx context.Context, fn func() error) error {
	return DoWithPolicy(ctx, DefaultPolicy(), fn)
}

// DoWithPolicy runs fn until it succeeds, until a non-retryable error is
// returned, until the schedule is exhausted, or until ctx is cancelled.
//
// The first attempt runs immediately. Before every later attempt the policy's
// wait for that attempt is applied, capped at Max; a cancelled context during
// the wait returns the context's error. The last retryable error is returned
// once the schedule is exhausted.
func DoWithPolicy(ctx context.Context, p Policy, fn func() error) error {
	backoff := p.Backoff
	if len(backoff) == 0 {
		backoff = DefaultBackoff
	}
	retryable := p.Retryable
	if retryable == nil {
		retryable = IsRetryable
	}

	var lastErr error
	for attempt := 0; attempt < len(backoff); attempt++ {
		if attempt > 0 {
			wait := backoff[attempt]
			if p.Max > 0 && wait > p.Max {
				wait = p.Max
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		if err := fn(); err != nil {
			if !retryable(err) {
				return err
			}
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

// IsRetryable reports whether an error is transient and worth retrying.
//
// Deadline expiry, connection refused/reset, unreachable hosts, timed-out
// syscalls, any net.Error reporting a timeout, and any error reporting HTTP
// 503 through StatusCoder are retryable. Every 4xx condition is deterministic
// and is explicitly NOT retryable: retrying it would only repeat the same
// rejection.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var coder StatusCoder
	if errors.As(err, &coder) && coder.HTTPStatusCode() == 503 {
		return true
	}
	return false
}
