//go:build windows

package signals

import (
	"os"
	"os/signal"
)

// notify registers the Windows signal set on ch: os.Interrupt (Ctrl+C,
// Ctrl+Break) for graceful shutdown. Windows has no SIGQUIT, SIGHUP,
// SIGUSR1, or SIGUSR2, so those are unavailable on this platform.
func notify(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}

// isReopenLogsSignal always reports false on Windows - there is no SIGUSR1.
func isReopenLogsSignal(sig os.Signal) bool {
	return false
}

// isDumpStatusSignal always reports false on Windows - there is no SIGUSR2.
func isDumpStatusSignal(sig os.Signal) bool {
	return false
}
