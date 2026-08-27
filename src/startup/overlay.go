package startup

import (
	"context"

	"github.com/webappsgo/redxt/src/overlay"
)

// startOverlays brings up the AI.md PART 32 overlay networks. It runs
// after the HTTP server so both can share the composed router through
// ServeExtra, and before the scheduler so tor_health and i2p_health have
// something to check.
//
// Neither network can fail startup. Tor is required in the sense that it
// is never a toggle — it comes up whenever a tor binary exists — but a
// host without one is a perfectly valid deployment, and an overlay that
// cannot start must not take the clearnet server down with it.
func (s *Server) startOverlays(ctx context.Context) {
	s.startTor(ctx)
	s.startI2P(ctx)
}

// startTor implements PART 32.1. The hidden service has no enable
// switch: if a tor binary is found it runs, and the only thing that
// stops it is the absence of one. It is app-scoped — the torrc this
// build generates publishes a single hidden service and never relays,
// exits, or proxies for anyone else.
func (s *Server) startTor(ctx context.Context) {
	manager := overlay.NewTorManager(ctx, &s.Config.Tor, s.Paths, s.Log)
	if err := manager.Start(); err != nil {
		s.Log.Infof("Tor hidden service is not available: %v", err)
		return
	}
	s.Tor = manager

	address := manager.OnionAddress()
	if address != "" && address != s.Config.Tor.OnionAddress {
		s.Config.Tor.OnionAddress = address
		if err := s.Config.Save(); err != nil {
			s.Log.Warnf("Saving the onion address: %v", err)
		}
	}
	s.Log.Infof("Tor hidden service available at http://%s", address)

	// The backend listener is the only one that speaks the PROXY
	// protocol, because torrc sets HiddenServiceExportCircuitID haproxy.
	service := manager.Service()
	if service == nil {
		return
	}
	listener, err := overlay.NewTorBackendListener(service.BackendPort())
	if err != nil {
		s.Log.Errorf("Tor backend listener: %v", err)
		return
	}
	s.HTTP.ServeExtra(listener, "Tor")
}

// startI2P implements PART 32.2. The eepsite is opt-in and stays off
// until an operator sets server.i2p.enabled, which is why this returns
// before touching anything when the flag is unset — auto-enabling it is
// never permitted.
func (s *Server) startI2P(ctx context.Context) {
	if !s.Config.Server.I2P.Enabled {
		return
	}

	manager := overlay.NewI2PManager(ctx, &s.Config.Server.I2P, s.Paths, s.Log)
	if err := manager.Start(); err != nil {
		s.Log.Warnf("I2P eepsite is enabled but did not start: %v", err)
		return
	}
	s.I2P = manager
	s.Log.Infof("I2P eepsite available at http://%s", manager.EepsiteAddress())

	service := manager.Service()
	if service == nil {
		return
	}
	listener, err := overlay.NewI2PBackendListener(service.BackendPort())
	if err != nil {
		s.Log.Errorf("I2P backend listener: %v", err)
		return
	}
	s.HTTP.ServeExtra(listener, "I2P")
}

// stopOverlays closes both overlay networks. Tor goes first: it is the
// one that always exists when either does, and a client mid-request on
// the hidden service should lose the overlay before the clearnet
// listeners underneath it disappear.
func (s *Server) stopOverlays() []error {
	var errs []error
	if s.Tor != nil {
		errs = append(errs, s.Tor.Close())
		s.Tor = nil
	}
	if s.I2P != nil {
		errs = append(errs, s.I2P.Close())
		s.I2P = nil
	}
	return errs
}

// torHealth is the scheduler handler for PART 19's tor_health task. The
// manager's own monitor restarts a dead process; this task is what makes
// a failure visible in scheduler_history to an operator who is not
// watching the log.
func (s *Server) torHealth(ctx context.Context) error {
	if s.Tor == nil {
		return nil
	}
	service := s.Tor.Service()
	if service == nil {
		return nil
	}
	return service.HealthCheck(ctx)
}

// i2pHealth is the scheduler handler for PART 19's i2p_health task.
func (s *Server) i2pHealth(ctx context.Context) error {
	if s.I2P == nil {
		return nil
	}
	service := s.I2P.Service()
	if service == nil {
		return nil
	}
	return service.HealthCheck(ctx)
}
