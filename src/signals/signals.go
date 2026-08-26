// Package signals wires OS signals to graceful-shutdown, log-reopen, and
// status-dump callbacks. Package name is "signals", not "signal", so call
// sites can import the standard library "os/signal" package without a name
// collision.
//
// Signal routing (see AI.md PART 8, "Signal Handling & Graceful Shutdown"):
//
//	SIGTERM, SIGINT, SIGQUIT, SIGRTMIN+3 -> Handlers.Shutdown
//	SIGHUP                               -> ignored (config hot-reloads via file watcher)
//	SIGUSR1                              -> Handlers.ReopenLogs
//	SIGUSR2                              -> Handlers.DumpStatus
//	os.Interrupt (Windows)               -> Handlers.Shutdown
//
// Windows has no SIGQUIT, SIGHUP, SIGUSR1, or SIGUSR2, so only os.Interrupt
// is registered there; ReopenLogs and DumpStatus are simply never triggered
// on that platform. Platform differences live in signals_unix.go and
// signals_windows.go behind build tags; this file is identical on both.
package signals

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"time"
)

// DefaultShutdownTimeout is used when Handlers.ShutdownTimeout is zero.
const DefaultShutdownTimeout = 30 * time.Second

// Handlers holds the optional callbacks invoked for each routed signal.
// A nil field means "do nothing" for that signal class.
type Handlers struct {
	// Shutdown is invoked exactly once when a shutdown signal (SIGTERM,
	// SIGINT, SIGQUIT, SIGRTMIN+3 on Unix; os.Interrupt on Windows) is
	// received. The supplied context is cancelled after ShutdownTimeout
	// elapses; Shutdown should honour ctx.Done() and return promptly.
	Shutdown func(ctx context.Context)

	// ReopenLogs is invoked on SIGUSR1 (Unix only) for log-file rotation.
	ReopenLogs func()

	// DumpStatus is invoked on SIGUSR2 (Unix only) to dump status to the log.
	DumpStatus func()

	// ShutdownTimeout bounds the context passed to Shutdown. Zero means
	// DefaultShutdownTimeout.
	ShutdownTimeout time.Duration
}

// notifyFunc registers ch to receive the platform's signal set. It is a
// package-level variable assigned from the platform-specific implementation
// so tests can substitute a no-op and drive the dispatch loop with
// synthetic signals instead of real OS signals.
var notifyFunc = notify

// Listener represents an active signal subscription. Create one with
// Listen and release it with Stop.
type Listener struct {
	handlers Handlers

	sigChan chan os.Signal
	done    chan struct{}
	loopWG  sync.WaitGroup

	shutdownOnce sync.Once
	stopOnce     sync.Once

	mu   sync.Mutex
	trig os.Signal
}

// Listen registers the platform signal set and starts routing signals to h.
// Call Stop when the listener is no longer needed to unregister cleanly and
// release the dispatch goroutine.
func Listen(h Handlers) *Listener {
	return newListener(h, notifyFunc)
}

// newListener builds a Listener using register to subscribe sigChan. It is
// split out from Listen so tests can inject a no-op register and drive the
// dispatch loop by writing directly to the internal channel.
func newListener(h Handlers, register func(chan<- os.Signal)) *Listener {
	if h.ShutdownTimeout <= 0 {
		h.ShutdownTimeout = DefaultShutdownTimeout
	}

	l := &Listener{
		handlers: h,
		sigChan:  make(chan os.Signal, 1),
		done:     make(chan struct{}),
	}

	register(l.sigChan)

	l.loopWG.Add(1)
	go l.loop()

	return l
}

// loop reads signals off sigChan until it is closed by Stop.
func (l *Listener) loop() {
	defer l.loopWG.Done()
	for sig := range l.sigChan {
		l.dispatch(sig)
	}
}

// dispatch routes one received signal to the matching handler.
func (l *Listener) dispatch(sig os.Signal) {
	switch {
	case isReopenLogsSignal(sig):
		if l.handlers.ReopenLogs != nil {
			l.handlers.ReopenLogs()
		}
	case isDumpStatusSignal(sig):
		if l.handlers.DumpStatus != nil {
			l.handlers.DumpStatus()
		}
	default:
		l.triggerShutdown(sig)
	}
}

// triggerShutdown runs Handlers.Shutdown at most once, bounded by a context
// that expires after ShutdownTimeout, then closes Done.
func (l *Listener) triggerShutdown(sig os.Signal) {
	l.shutdownOnce.Do(func() {
		l.mu.Lock()
		l.trig = sig
		l.mu.Unlock()

		if l.handlers.Shutdown != nil {
			ctx, cancel := context.WithTimeout(context.Background(), l.handlers.ShutdownTimeout)
			l.handlers.Shutdown(ctx)
			cancel()
		}

		close(l.done)
	})
}

// Done returns a channel that closes once a shutdown signal has been
// received and Handlers.Shutdown has returned (or was nil).
func (l *Listener) Done() <-chan struct{} {
	return l.done
}

// Signal returns the signal that triggered shutdown, or nil if no shutdown
// signal has been received yet.
func (l *Listener) Signal() os.Signal {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.trig
}

// Stop unregisters the listener from further OS signals and waits for the
// dispatch goroutine to exit. Stop is idempotent and safe to call even if
// no shutdown signal was ever received.
func (l *Listener) Stop() {
	l.stopOnce.Do(func() {
		signal.Stop(l.sigChan)
		close(l.sigChan)
	})
	l.loopWG.Wait()
}
