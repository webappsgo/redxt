//go:build !windows
// +build !windows

package signals

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// kill delivers sig to the current process. Because Listen has already
// called signal.Notify for the signals under test, the Go runtime routes
// the delivery to the listener's channel instead of applying the signal's
// default disposition, so the test process is never terminated.
func kill(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), sig); err != nil {
		t.Fatalf("syscall.Kill(%v): %v", sig, err)
	}
}

func TestUnixSIGHUPIsIgnored(t *testing.T) {
	var shutdownCalls int32
	l := Listen(Handlers{
		Shutdown: func(ctx context.Context) { atomic.AddInt32(&shutdownCalls, 1) },
	})
	t.Cleanup(l.Stop)

	kill(t, syscall.SIGHUP)

	select {
	case <-l.Done():
		t.Fatal("SIGHUP triggered shutdown, want it ignored")
	case <-time.After(200 * time.Millisecond):
	}

	if got := atomic.LoadInt32(&shutdownCalls); got != 0 {
		t.Fatalf("Shutdown called %d times after SIGHUP, want 0", got)
	}
}

func TestUnixSIGUSR1ReopensLogs(t *testing.T) {
	reopened := make(chan struct{}, 1)
	l := Listen(Handlers{
		ReopenLogs: func() { reopened <- struct{}{} },
	})
	t.Cleanup(l.Stop)

	kill(t, syscall.SIGUSR1)

	select {
	case <-reopened:
	case <-time.After(2 * time.Second):
		t.Fatal("ReopenLogs was not invoked for SIGUSR1")
	}

	select {
	case <-l.Done():
		t.Fatal("SIGUSR1 triggered shutdown, want only ReopenLogs")
	default:
	}
}

func TestUnixSIGUSR2DumpsStatus(t *testing.T) {
	dumped := make(chan struct{}, 1)
	l := Listen(Handlers{
		DumpStatus: func() { dumped <- struct{}{} },
	})
	t.Cleanup(l.Stop)

	kill(t, syscall.SIGUSR2)

	select {
	case <-dumped:
	case <-time.After(2 * time.Second):
		t.Fatal("DumpStatus was not invoked for SIGUSR2")
	}

	select {
	case <-l.Done():
		t.Fatal("SIGUSR2 triggered shutdown, want only DumpStatus")
	default:
	}
}

func TestUnixShutdownSignalInvokedOnceEvenWhenSentTwice(t *testing.T) {
	var calls int32
	l := Listen(Handlers{
		Shutdown: func(ctx context.Context) { atomic.AddInt32(&calls, 1) },
	})
	t.Cleanup(l.Stop)

	kill(t, syscall.SIGTERM)
	kill(t, syscall.SIGTERM)

	select {
	case <-l.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close after SIGTERM")
	}

	// Let a second, mis-dispatched call have a chance to land before checking.
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("Shutdown called %d times, want 1", got)
	}
	if got := l.Signal(); got != syscall.SIGTERM {
		t.Fatalf("Signal() = %v, want SIGTERM", got)
	}
}
