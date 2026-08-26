//go:build !windows
// +build !windows

package signals

import (
	"os"
	"os/signal"
	"syscall"
)

// notify registers the Unix signal set on ch: SIGTERM, SIGINT, SIGQUIT, and
// SIGRTMIN+3 (signal 37, Docker STOPSIGNAL) for graceful shutdown; SIGUSR1
// for log reopen; SIGUSR2 for status dump. SIGHUP is explicitly ignored -
// config auto-reloads via a file watcher, so SIGHUP must never trigger
// shutdown or any other handler.
func notify(ch chan<- os.Signal) {
	signal.Notify(ch,
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
		syscall.Signal(37),
	)
	signal.Ignore(syscall.SIGHUP)
}

// isReopenLogsSignal reports whether sig requests log-file reopening.
func isReopenLogsSignal(sig os.Signal) bool {
	return sig == syscall.SIGUSR1
}

// isDumpStatusSignal reports whether sig requests a status dump.
func isDumpStatusSignal(sig os.Signal) bool {
	return sig == syscall.SIGUSR2
}
