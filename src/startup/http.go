package startup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/webappsgo/redxt/src/health"
	"github.com/webappsgo/redxt/src/paths"
	"github.com/webappsgo/redxt/src/server"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/ssl"
	"github.com/webappsgo/redxt/src/urlvars"
)

const (
	// httpShutdownTimeout bounds how long in-flight requests are given to
	// finish once graceful shutdown begins.
	httpShutdownTimeout = 30 * time.Second
	// databasePingTimeout bounds the health probe's datastore check, so a
	// hung database degrades the health document instead of holding the
	// health request open.
	databasePingTimeout = 3 * time.Second
)

// startHTTP performs step 18 of the AI.md PART 8 startup sequence: it
// resolves the URL variables every later stage depends on, brings up the
// certificate manager from AI.md PART 15, composes the PART 12
// middleware chain, and binds the listeners the PART 14 router serves.
//
// Binding is synchronous, so a port already in use fails startup rather
// than leaving a server that answers nothing.
func (s *Server) startHTTP(ctx context.Context) error {
	s.URLVars = s.newResolver()

	if err := s.openTLS(); err != nil {
		return err
	}

	httpServer, err := server.New(server.Options{
		Config:       s.Config,
		Paths:        s.Paths,
		Log:          s.Log,
		Mode:         string(s.Mode.Mode),
		Debug:        s.Mode.Debug,
		Middleware:   s.middleware(),
		TLS:          s.tlsProvider(),
		HealthProbe:  s.healthProbe,
		TrustedProxy: middleware.TrustedProxyFunc(s.URLVars),
		Started:      s.Started,
	})
	if err != nil {
		return err
	}
	if err := httpServer.Start(ctx); err != nil {
		return err
	}
	s.HTTP = httpServer

	s.issueCertificate(ctx)
	return nil
}

// newResolver builds the URL variable resolver from AI.md PART 8. It is
// shared: the middleware chain keys the rate limiter and the access log
// on its client-address resolution, and the certificate manager takes
// its FQDN from the same place, so the whole process agrees on who the
// client is and what this server is called.
func (s *Server) newResolver() *urlvars.Resolver {
	return urlvars.New(urlvars.Options{
		Listen:         s.Config.Server.Listen,
		Port:           s.Config.Server.Port,
		BaseURL:        s.Config.Server.BaseURL,
		TrustedProxies: s.Config.Server.TrustedProxies,
		OnionAddress:   s.Config.Tor.OnionAddress,
		Mode:           s.Mode.Mode,
		ProjectName:    paths.ProjectName(),
		Logf:           s.Log.Infof,
	})
}

// middleware composes the PART 12 chain around the router. The two
// stages the defaults cannot fill in are supplied here because only the
// startup sequence holds them: the rate limiter's window store lives in
// server.db, and the access log writes through the configured logger.
func (s *Server) middleware() func(http.Handler) http.Handler {
	opts := middleware.DefaultOptions(s.Config, s.URLVars)
	opts.RateLimit.Store = middleware.NewSQLStore(s.ServerDB.DB, 0)
	opts.RateLimit.OnError = func(_ *http.Request, err error) {
		s.Log.Errorf("Rate limit store: %v", err)
	}
	opts.Logging.Sink = s.Log.Access
	return middleware.New(opts)
}

// openTLS builds the certificate manager when the configuration asks for
// an HTTPS listener, and runs the PART 15 four-tier lookup once so that
// a certificate already on disk is serving from the first handshake.
//
// A misconfiguration — an unparsable TLS version, an unknown challenge
// type, a hostname Let's Encrypt cannot issue for — fails startup,
// because the operator asked for something that can never work. A merely
// absent certificate does not: with Let's Encrypt enabled the listener
// binds anyway and issuance runs once the plaintext listener is up to
// answer the challenge, and without it the server keeps serving over
// HTTP instead of refusing to start at all.
func (s *Server) openTLS() error {
	if s.Config.HTTPSPort() <= 0 {
		return nil
	}

	_, fqdn, _ := s.URLVars.URLVars(nil)
	manager, err := ssl.New(s.Config.Server.SSL, s.Paths.SSL, fqdn)
	if err != nil {
		return err
	}

	cert, err := manager.Load()
	switch {
	case err == nil:
		s.Log.Infof("Certificate for %s loaded from %s, valid until %s",
			cert.FQDN, cert.Dir, cert.NotAfter.Format(time.RFC3339))
	case manager.ACMEEnabled():
		s.Log.Warnf("No usable certificate for %s yet: %v", fqdn, err)
	default:
		s.Log.Warnf("HTTPS is disabled for this run: %v", err)
		return nil
	}

	s.SSL = manager
	return nil
}

// tlsProvider returns the certificate provider for the HTTPS listener.
// It exists so a disabled TLS setup hands the server a nil interface
// rather than a non-nil interface holding a nil manager, which would
// bind an HTTPS listener that panics on its first handshake.
func (s *Server) tlsProvider() server.TLSProvider {
	if s.SSL == nil {
		return nil
	}
	return s.SSL
}

// issueCertificate requests the first Let's Encrypt certificate when the
// PART 15 lookup found none. It runs in the background because it talks
// to the CA, which answers over the network and can be slow or down, and
// because HTTP-01 is answered by the listener that is only serving once
// startHTTP has returned. Startup never blocks on a certificate; the
// HTTPS listener simply refuses handshakes until one exists.
func (s *Server) issueCertificate(ctx context.Context) {
	if s.SSL == nil || !s.SSL.ACMEEnabled() || s.SSL.Current() != nil {
		return
	}

	issueCtx, cancel := context.WithCancel(ctx)
	s.acmeCancel = cancel

	s.acmeWG.Add(1)
	go func() {
		defer s.acmeWG.Done()

		fqdn := s.SSL.FQDN()
		s.Log.Infof("Requesting a certificate for %s over %s", fqdn, s.SSL.Challenge())

		cert, err := s.SSL.Issue(issueCtx)
		if err != nil {
			var pending *ssl.DNS01RecordRequiredError
			if errors.As(err, &pending) {
				s.Log.Warnf("Certificate for %s is waiting on a DNS record: %v", fqdn, err)
				return
			}
			s.Log.Errorf("Requesting a certificate for %s: %v", fqdn, err)
			return
		}
		s.Log.Infof("Certificate for %s issued, valid until %s",
			cert.FQDN, cert.NotAfter.Format(time.RFC3339))
	}()
}

// stopHTTP takes the listeners down and waits for any in-flight ACME
// issuance to unwind. It is the first step of shutdown so that no new
// request is accepted while the databases behind it are closing.
func (s *Server) stopHTTP() error {
	if s.acmeCancel != nil {
		s.acmeCancel()
		s.acmeCancel = nil
	}
	s.acmeWG.Wait()

	if s.HTTP == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()

	err := s.HTTP.Shutdown(ctx)
	s.HTTP = nil
	return err
}

// healthProbe collects the live half of the PART 13 health document.
//
// Only the subsystems this sequence has actually started are reported.
// A check left empty means the component that owns it is not part of
// this build yet, and health.Overall skips it rather than counting a
// component that does not exist as a failure.
func (s *Server) healthProbe(ctx context.Context) server.HealthSnapshot {
	return server.HealthSnapshot{
		State: health.State{ShuttingDown: s.shuttingDown},
		Checks: health.ChecksInfo{
			Database: health.Check(s.databasesReachable(ctx)),
		},
		Features: health.FeaturesInfo{
			Tor: health.TorInfo{
				Enabled:  s.Config.Tor.OnionAddress != "",
				Hostname: s.Config.Tor.OnionAddress,
			},
		},
	}
}

// databasesReachable reports whether both stores answer a ping inside
// the probe's own deadline.
func (s *Server) databasesReachable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, databasePingTimeout)
	defer cancel()

	for _, db := range []interface {
		PingContext(context.Context) error
	}{s.ServerDB, s.UsersDB} {
		if db == nil {
			return false
		}
		if err := db.PingContext(ctx); err != nil {
			return false
		}
	}
	return true
}

// listenURLs returns the URLs the banner and the startup log announce.
// They come from the bound listeners rather than the configuration, so a
// port the kernel chose (config port 0) is reported as the port clients
// must actually use.
func (s *Server) listenURLs() []string {
	if s.HTTP != nil {
		if urls := s.HTTP.URLs(); len(urls) > 0 {
			return urls
		}
	}
	return []string{fmt.Sprintf("http://%s:%d", s.Config.Server.Listen, s.Config.Server.Port)}
}
