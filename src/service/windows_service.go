//go:build windows

package service

import "golang.org/x/sys/windows/svc"

// windowsService adapts a plain start/stop pair of functions to the
// svc.Handler interface AI.md PART 25 "Windows Service" requires.
type windowsService struct {
	start func() error
	stop  func()
}

// Execute implements svc.Handler. It reports StartPending, calls start,
// reports Running (accepting Stop and Shutdown), then on receiving either
// reports StopPending, calls stop, and returns.
func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}

	if err := ws.start(); err != nil {
		s <- svc.Status{State: svc.Stopped}
		return true, 1
	}

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			ws.stop()
			return false, 0
		}
	}
	return false, 0
}

// RunWindowsService runs the current process as the named Windows
// service, calling start once at StartPending and stop once a Stop or
// Shutdown control request arrives. It blocks until the service stops.
func RunWindowsService(name string, start func() error, stop func()) error {
	return svc.Run(name, &windowsService{start: start, stop: stop})
}

// IsWindowsService reports whether the current process is running under
// the Windows Service Control Manager, as opposed to an interactive
// session.
func IsWindowsService() (bool, error) {
	return svc.IsWindowsService()
}
