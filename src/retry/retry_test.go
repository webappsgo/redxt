package retry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"
)

// statusErr is a test error carrying an HTTP status code via StatusCoder.
type statusErr struct {
	status int
}

func (e statusErr) Error() string {
	return fmt.Sprintf("status %d", e.status)
}

func (e statusErr) HTTPStatusCode() int {
	return e.status
}

// timeoutErr is a test net.Error whose Timeout result is configurable.
type timeoutErr struct {
	timeout bool
}

func (e timeoutErr) Error() string {
	return "net error"
}

func (e timeoutErr) Timeout() bool {
	return e.timeout
}

func (e timeoutErr) Temporary() bool {
	return false
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "wrapped deadline exceeded", err: fmt.Errorf("dial: %w", context.DeadlineExceeded), want: true},
		{name: "connection refused", err: syscall.ECONNREFUSED, want: true},
		{name: "connection reset", err: syscall.ECONNRESET, want: true},
		{name: "host unreachable", err: syscall.EHOSTUNREACH, want: true},
		{name: "timed out", err: syscall.ETIMEDOUT, want: true},
		{name: "net timeout", err: net.Error(timeoutErr{timeout: true}), want: true},
		{name: "net non timeout", err: net.Error(timeoutErr{timeout: false}), want: false},
		{name: "http 503", err: statusErr{status: 503}, want: true},
		{name: "wrapped http 503", err: fmt.Errorf("upstream: %w", statusErr{status: 503}), want: true},
		{name: "http 400 not retryable", err: statusErr{status: 400}, want: false},
		{name: "http 404 not retryable", err: statusErr{status: 404}, want: false},
		{name: "http 429 not retryable", err: statusErr{status: 429}, want: false},
		{name: "http 500 not retryable", err: statusErr{status: 500}, want: false},
		{name: "context canceled", err: context.Canceled, want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Fatalf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// fastPolicy returns a policy with zero waits so tests stay instant.
func fastPolicy(attempts int, retryable func(error) bool) Policy {
	return Policy{
		Backoff:   make([]time.Duration, attempts),
		Max:       MaxBackoff,
		Retryable: retryable,
	}
}

func TestDoWithPolicy(t *testing.T) {
	transient := errors.New("transient")
	permanent := errors.New("permanent")
	alwaysRetry := func(err error) bool { return errors.Is(err, transient) }

	tests := []struct {
		name         string
		attempts     int
		results      []error
		wantErr      error
		wantAttempts int
	}{
		{
			name:         "success on first attempt",
			attempts:     5,
			results:      []error{nil},
			wantErr:      nil,
			wantAttempts: 1,
		},
		{
			name:         "success on second attempt",
			attempts:     5,
			results:      []error{transient, nil},
			wantErr:      nil,
			wantAttempts: 2,
		},
		{
			name:         "exhausts schedule and returns last error",
			attempts:     5,
			results:      []error{transient, transient, transient, transient, transient},
			wantErr:      transient,
			wantAttempts: 5,
		},
		{
			name:         "non retryable short circuits",
			attempts:     5,
			results:      []error{permanent},
			wantErr:      permanent,
			wantAttempts: 1,
		},
		{
			name:         "non retryable after one retry",
			attempts:     5,
			results:      []error{transient, permanent},
			wantErr:      permanent,
			wantAttempts: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			err := DoWithPolicy(context.Background(), fastPolicy(tt.attempts, alwaysRetry), func() error {
				res := tt.results[calls]
				calls++
				return res
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantAttempts {
				t.Fatalf("attempts = %d, want %d", calls, tt.wantAttempts)
			}
		})
	}
}

func TestDoWithPolicyContextCancelled(t *testing.T) {
	transient := errors.New("transient")

	tests := []struct {
		name    string
		ctx     func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name: "cancelled before wait",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: context.Canceled,
		},
		{
			name: "deadline expires during wait",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), time.Millisecond)
			},
			wantErr: context.DeadlineExceeded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.ctx()
			defer cancel()

			policy := Policy{
				Backoff:   []time.Duration{0, 50 * time.Millisecond},
				Max:       MaxBackoff,
				Retryable: func(error) bool { return true },
			}
			calls := 0
			err := DoWithPolicy(ctx, policy, func() error {
				calls++
				return transient
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if calls != 1 {
				t.Fatalf("attempts = %d, want 1", calls)
			}
		})
	}
}

func TestDoWithPolicyDefaults(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
		err    error
		want   int
	}{
		{
			name:   "empty backoff falls back to default classification",
			policy: Policy{Retryable: func(error) bool { return false }},
			err:    errors.New("boom"),
			want:   1,
		},
		{
			name:   "nil retryable uses IsRetryable",
			policy: Policy{Backoff: make([]time.Duration, 3)},
			err:    errors.New("boom"),
			want:   1,
		},
		{
			name:   "nil retryable retries transient errors",
			policy: Policy{Backoff: make([]time.Duration, 3)},
			err:    syscall.ECONNREFUSED,
			want:   3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			err := DoWithPolicy(context.Background(), tt.policy, func() error {
				calls++
				return tt.err
			})
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if calls != tt.want {
				t.Fatalf("attempts = %d, want %d", calls, tt.want)
			}
		})
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if len(p.Backoff) != len(DefaultBackoff) {
		t.Fatalf("backoff length = %d, want %d", len(p.Backoff), len(DefaultBackoff))
	}
	for i, want := range DefaultBackoff {
		if p.Backoff[i] != want {
			t.Fatalf("backoff[%d] = %v, want %v", i, p.Backoff[i], want)
		}
	}
	if p.Max != MaxBackoff {
		t.Fatalf("max = %v, want %v", p.Max, MaxBackoff)
	}
	if p.Retryable == nil {
		t.Fatal("retryable = nil, want IsRetryable")
	}
}

func TestDoSucceedsImmediately(t *testing.T) {
	calls := 0
	if err := Do(context.Background(), func() error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("attempts = %d, want 1", calls)
	}
}
