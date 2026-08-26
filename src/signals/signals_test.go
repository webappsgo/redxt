package signals

import (
	"context"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSignal is a synthetic os.Signal used to drive the dispatch loop
// without touching real OS signal delivery. Because it never equals a
// platform signal constant, dispatch always routes it to Shutdown - which
// is exactly what is needed to test the shutdown-path behaviors below.
type fakeSignal struct{ name string }

func (f fakeSignal) String() string { return f.name }
func (f fakeSignal) Signal()        {}

// noopRegister satisfies newListener's register parameter without calling
// into the real os/signal package, so tests are deterministic and portable.
func noopRegister(chan<- os.Signal) {}

// waitFor polls cond until it is true or the deadline passes, failing the
// test on timeout. Used instead of a fixed sleep for goroutine-exit checks.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %v", timeout)
	}
}

func TestShutdownInvokedExactlyOnce(t *testing.T) {
	var calls int32
	h := Handlers{
		Shutdown: func(ctx context.Context) {
			atomic.AddInt32(&calls, 1)
		},
	}
	l := newListener(h, noopRegister)
	t.Cleanup(l.Stop)

	l.sigChan <- fakeSignal{"first"}
	l.sigChan <- fakeSignal{"second"}

	select {
	case <-l.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close after shutdown signal")
	}

	// Give the second signal a chance to be (mis)dispatched before asserting.
	time.Sleep(20 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("Shutdown called %d times, want 1", got)
	}
}

func TestSignalReportsTrigger(t *testing.T) {
	h := Handlers{Shutdown: func(ctx context.Context) {}}
	l := newListener(h, noopRegister)
	t.Cleanup(l.Stop)

	if got := l.Signal(); got != nil {
		t.Fatalf("Signal() = %v before any signal received, want nil", got)
	}

	want := fakeSignal{"trigger"}
	l.sigChan <- want
	<-l.Done()

	if got := l.Signal(); got != want {
		t.Fatalf("Signal() = %v, want %v", got, want)
	}
}

func TestShutdownNilHandlerDoesNotPanic(t *testing.T) {
	l := newListener(Handlers{}, noopRegister)
	t.Cleanup(l.Stop)

	l.sigChan <- fakeSignal{"noop"}

	select {
	case <-l.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close with nil Shutdown handler")
	}
}

func TestShutdownTimeoutExpiresContext(t *testing.T) {
	const timeout = 30 * time.Millisecond
	errCh := make(chan error, 1)
	h := Handlers{
		ShutdownTimeout: timeout,
		Shutdown: func(ctx context.Context) {
			<-ctx.Done()
			errCh <- ctx.Err()
		},
	}
	l := newListener(h, noopRegister)
	t.Cleanup(l.Stop)

	start := time.Now()
	l.sigChan <- fakeSignal{"slow"}

	select {
	case err := <-errCh:
		if err != context.DeadlineExceeded {
			t.Fatalf("ctx.Err() = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown handler never observed ctx.Done()")
	}

	if elapsed := time.Since(start); elapsed < timeout {
		t.Fatalf("context expired after %v, want at least %v", elapsed, timeout)
	}

	<-l.Done()
}

func TestDefaultShutdownTimeoutApplied(t *testing.T) {
	l := newListener(Handlers{}, noopRegister)
	t.Cleanup(l.Stop)

	if l.handlers.ShutdownTimeout != DefaultShutdownTimeout {
		t.Fatalf("ShutdownTimeout = %v, want default %v", l.handlers.ShutdownTimeout, DefaultShutdownTimeout)
	}
}

func TestStopUnregistersAndDoesNotLeakGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()

	l := newListener(Handlers{}, noopRegister)
	l.Stop()

	// Calling Stop again must be safe and must not block.
	l.Stop()

	waitFor(t, 2*time.Second, func() bool {
		runtime.Gosched()
		return runtime.NumGoroutine() <= before
	})
}

func TestReopenAndDumpStatusRoutingUnaffectedByFakeSignal(t *testing.T) {
	// A signal unknown to the platform's classifiers must fall through to
	// Shutdown, never to ReopenLogs or DumpStatus - proving the default
	// branch in dispatch is shutdown, not a silent no-op.
	var reopenCalls, dumpCalls, shutdownCalls int32
	h := Handlers{
		ReopenLogs: func() { atomic.AddInt32(&reopenCalls, 1) },
		DumpStatus: func() { atomic.AddInt32(&dumpCalls, 1) },
		Shutdown:   func(ctx context.Context) { atomic.AddInt32(&shutdownCalls, 1) },
	}
	l := newListener(h, noopRegister)
	t.Cleanup(l.Stop)

	l.sigChan <- fakeSignal{"unclassified"}
	<-l.Done()

	if reopenCalls != 0 || dumpCalls != 0 {
		t.Fatalf("reopenCalls=%d dumpCalls=%d, want 0/0", reopenCalls, dumpCalls)
	}
	if shutdownCalls != 1 {
		t.Fatalf("shutdownCalls=%d, want 1", shutdownCalls)
	}
}
