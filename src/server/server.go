// Package server implements the HTTP surface defined in AI.md PART 14
// (API STRUCTURE) and binds the listeners described in AI.md PART 15
// (SSL/TLS & LET'S ENCRYPT).
//
// The package owns listener lifecycle and routing only. Certificates
// come from src/ssl, the middleware chain comes from src/server/
// middleware, and the documentation surfaces come from src/swagger and
// src/graphql; every one of them is injected through Options so this
// package never reaches into a subsystem it does not own.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/paths"
	"github.com/webappsgo/redxt/src/ssl"
)

// Logger is the subset of the project logger this package needs. The
// concrete logger lives in src/logging; depending on the interface keeps
// the HTTP layer testable without a log file on disk.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// TLSProvider supplies the certificate material for the HTTPS listener.
// It is implemented by src/ssl and injected so that this package holds
// no certificate policy of its own.
type TLSProvider interface {
	// TLSConfig returns the configuration for the HTTPS listener,
	// including the certificate callback that lets a renewed
	// certificate take effect without restarting the process.
	TLSConfig() *tls.Config
	// HTTP01Handler returns the handler that answers HTTP-01 challenges,
	// mounted on the plaintext listener under the ACME challenge prefix.
	// It answers 404 for every path that is not a challenge in flight,
	// so it is safe to mount whatever challenge type is configured.
	HTTP01Handler() http.Handler
}

// Options describes everything the HTTP surface needs to run.
type Options struct {
	// Config is the loaded server.yml document.
	Config *config.Config
	// Paths carries the resolved filesystem locations.
	Paths paths.Paths
	// Log receives listener lifecycle messages.
	Log Logger
	// Mode is the resolved application mode, reported by health.
	Mode string
	// Debug reports whether the debug flag is on. AI.md PART 11 keys
	// the dev_only sanitization stage on this flag, never on the mode.
	Debug bool
	// Middleware wraps the router. It is the composed chain from
	// src/server/middleware; a nil value serves the router unwrapped.
	Middleware func(http.Handler) http.Handler
	// TLS supplies certificates. A nil value disables the HTTPS
	// listener regardless of the configured ports.
	TLS TLSProvider
	// Routes carries the handler set the router mounts.
	Routes Routes
	// HealthProbe collects the live half of the health document. The
	// startup sequence supplies it because it owns the databases, the
	// scheduler, and the listeners the checks inspect.
	HealthProbe HealthProbe
	// Cluster lists the advertised cluster node URLs for
	// /api/autodiscover. A nil value advertises a single-node server.
	Cluster func() []string
	// Features reports the enabled optional subsystems published in the
	// autodiscover document's config.features object. A nil value
	// publishes an empty object rather than guessing.
	Features func() map[string]any
	// TrustedProxy reports whether a remote address is a trusted
	// reverse proxy, which gates every X-Forwarded-* header. A nil
	// value trusts nothing.
	TrustedProxy func(remoteAddr string) bool
	// Started is the process start time, used to report uptime.
	Started time.Time
}

// trustsProxy reports whether the request arrived from a trusted
// reverse proxy, and therefore whether its X-Forwarded-* headers may be
// believed.
func (o Options) trustsProxy(r *http.Request) bool {
	if o.TrustedProxy == nil || r == nil {
		return false
	}
	return o.TrustedProxy(r.RemoteAddr)
}

// Server owns the plaintext and TLS listeners and the shared router.
type Server struct {
	cfg   *config.Config
	log   Logger
	tls   TLSProvider
	http  *http.Server
	https *http.Server

	// listeners are bound in Start and closed by the http.Server
	// instances during Shutdown.
	httpLn  net.Listener
	httpsLn net.Listener

	// serveErr carries the first non-graceful listener failure.
	serveErr chan error

	// wg tracks the goroutines running Serve.
	wg sync.WaitGroup

	// handler is the fully composed router, kept so ServeExtra can put the
	// same request pipeline behind an overlay-network listener.
	handler http.Handler

	// extra holds the servers ServeExtra started, so Shutdown can drain
	// them alongside the clearnet listeners.
	extra []*http.Server

	// mu guards urls, which Start fills in once the listeners are bound,
	// and extra, which ServeExtra appends to after startup.
	mu   sync.Mutex
	urls []string
}

// New builds the server and its router without binding any port.
func New(o Options) (*Server, error) {
	if o.Config == nil {
		return nil, errors.New("server: a configuration is required")
	}
	if o.Log == nil {
		return nil, errors.New("server: a logger is required")
	}
	if o.Started.IsZero() {
		o.Started = time.Now()
	}

	router := NewRouter(o)

	var handler http.Handler = router
	if o.Middleware != nil {
		handler = o.Middleware(router)
	}

	limits := o.Config.Server.Limits
	build := func() *http.Server {
		return &http.Server{
			Handler:           handler,
			ReadTimeout:       time.Duration(limits.ReadTimeout),
			ReadHeaderTimeout: time.Duration(limits.ReadTimeout),
			WriteTimeout:      time.Duration(limits.WriteTimeout),
			IdleTimeout:       time.Duration(limits.IdleTimeout),
			MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		}
	}

	s := &Server{
		cfg:      o.Config,
		log:      o.Log,
		tls:      o.TLS,
		handler:  handler,
		serveErr: make(chan error, 2),
	}

	if port := o.Config.HTTPPort(); port > 0 {
		s.http = build()
		s.http.Addr = net.JoinHostPort(o.Config.Server.Listen, strconv.Itoa(port))
	}
	if port := o.Config.HTTPSPort(); port > 0 && o.TLS != nil {
		s.https = build()
		s.https.Addr = net.JoinHostPort(o.Config.Server.Listen, strconv.Itoa(port))
		s.https.TLSConfig = o.TLS.TLSConfig()
	}

	if s.http == nil && s.https == nil {
		return nil, errors.New("server: no listener is configured")
	}

	return s, nil
}

// Start binds every configured listener and serves in the background.
// Binding happens synchronously so a port conflict fails startup rather
// than surfacing later as a silent absence of a listener.
func (s *Server) Start(ctx context.Context) error {
	lc := &net.ListenConfig{}

	if s.http != nil {
		ln, err := lc.Listen(ctx, "tcp", s.http.Addr)
		if err != nil {
			return fmt.Errorf("server: binding %s: %w", s.http.Addr, err)
		}
		s.httpLn = ln
	}

	if s.https != nil {
		ln, err := lc.Listen(ctx, "tcp", s.https.Addr)
		if err != nil {
			if s.httpLn != nil {
				_ = s.httpLn.Close()
				s.httpLn = nil
			}
			return fmt.Errorf("server: binding %s: %w", s.https.Addr, err)
		}
		s.httpsLn = ln
	}

	s.mu.Lock()
	s.urls = s.buildURLs()
	s.mu.Unlock()

	if s.httpLn != nil {
		// The server and listener are captured rather than read from the
		// receiver, so a shutdown that lands before this goroutine is
		// scheduled cannot leave it dereferencing a cleared field.
		srv, ln := s.http, s.httpLn
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.serveErr <- err
			}
		}()
		s.log.Infof("HTTP listener bound on %s", s.httpLn.Addr())
	}

	if s.httpsLn != nil {
		srv, ln := s.https, s.httpsLn
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			// The certificates come from the TLS config's callback, so
			// ServeTLS is called without file arguments.
			if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.serveErr <- err
			}
		}()
		s.log.Infof("HTTPS listener bound on %s", s.httpsLn.Addr())
	}

	return nil
}

// ServeExtra serves the same router on a listener the caller already
// bound. It exists for the AI.md PART 32 overlay networks: the Tor
// hidden service and the I2P eepsite each forward to a dedicated
// loopback listener whose wrapping (PROXY protocol, for Tor) belongs to
// the overlay package, not here. Shutdown drains these alongside the
// clearnet listeners.
//
// A failure is reported on Err like any other listener failure, using a
// non-blocking send so an overlay listener dying after the channel is
// already full can never strand its goroutine.
func (s *Server) ServeExtra(ln net.Listener, label string) {
	srv := &http.Server{
		Handler:           s.handler,
		ReadTimeout:       time.Duration(s.cfg.Server.Limits.ReadTimeout),
		ReadHeaderTimeout: time.Duration(s.cfg.Server.Limits.ReadTimeout),
		WriteTimeout:      time.Duration(s.cfg.Server.Limits.WriteTimeout),
		IdleTimeout:       time.Duration(s.cfg.Server.Limits.IdleTimeout),
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
	}

	s.mu.Lock()
	s.extra = append(s.extra, srv)
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case s.serveErr <- err:
			default:
			}
		}
	}()
	s.log.Infof("%s listener bound on %s", label, ln.Addr())
}

// Err returns the channel carrying the first non-graceful listener
// failure. A caller selecting on it learns that a listener died without
// having to poll.
func (s *Server) Err() <-chan error {
	return s.serveErr
}

// Shutdown stops both listeners gracefully, giving in-flight requests
// the context's deadline to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error

	// The overlay listeners go first: a Tor or I2P client must stop being
	// served before the clearnet side starts tearing down.
	s.mu.Lock()
	extra := s.extra
	s.extra = nil
	s.mu.Unlock()
	for _, srv := range extra {
		errs = append(errs, srv.Shutdown(ctx))
	}

	if s.https != nil {
		errs = append(errs, s.https.Shutdown(ctx))
		s.https = nil
	}
	if s.http != nil {
		errs = append(errs, s.http.Shutdown(ctx))
		s.http = nil
	}

	s.wg.Wait()
	s.httpLn = nil
	s.httpsLn = nil

	return errors.Join(errs...)
}

// URLs returns the externally meaningful URLs of the bound listeners,
// with the default ports stripped per the AI.md PART 15 display rules.
func (s *Server) URLs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.urls...)
}

// buildURLs renders one URL per bound listener.
func (s *Server) buildURLs() []string {
	host := displayHost(s.cfg.Server.Listen)

	var urls []string
	if s.httpLn != nil {
		urls = append(urls, ssl.FormatURL(host, listenerPort(s.httpLn), false))
	}
	if s.httpsLn != nil {
		urls = append(urls, ssl.FormatURL(host, listenerPort(s.httpsLn), true))
	}
	return urls
}

// displayHost turns a listen address into the host half of an
// advertised URL. A wildcard address is meaningless to a client, so it
// becomes localhost, and an IPv6 literal is bracketed because the PART
// 15 formatter takes the host verbatim.
func displayHost(listen string) string {
	switch listen {
	case "", "0.0.0.0", "::", "[::]", "*":
		return "localhost"
	}
	if strings.Contains(listen, ":") && !strings.HasPrefix(listen, "[") {
		return "[" + listen + "]"
	}
	return listen
}

// listenerPort reports the port a bound listener actually got, which
// differs from the configured value when the config asked for port 0.
func listenerPort(ln net.Listener) int {
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}
