// Package startup implements the server startup sequence defined in
// AI.md PART 8 "Server Startup Sequence". It owns the ordering of the
// phases — immediate-exit flags, path resolution, directory creation,
// logging, the PID file, configuration, and the database — and the
// matching teardown, so that main stays a thin entry point.
//
// The steps this package does not own are the ones whose subsystems
// belong to later PARTs: the privilege drop and the system user (PART
// 24), privileged port pre-binding and the HTTP server (PART 14), the
// scheduler (PART 19), and the overlay networks (PART 32). Each is
// wired in by its owning PART at the point the sequence reserves for
// it.
package startup

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/cli"
	"github.com/webappsgo/redxt/src/common/banner"
	"github.com/webappsgo/redxt/src/common/color"
	"github.com/webappsgo/redxt/src/common/version"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/daemon"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/logging"
	"github.com/webappsgo/redxt/src/mode"
	"github.com/webappsgo/redxt/src/paths"
	"github.com/webappsgo/redxt/src/pidfile"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server"
	"github.com/webappsgo/redxt/src/signals"
	"github.com/webappsgo/redxt/src/ssl"
	"github.com/webappsgo/redxt/src/urlvars"
)

// setupTokenSecret is the app_secrets name under which the hash of the
// first-run setup token is stored. Only the hash is persisted; the
// token itself is shown once on the console and never written to a log
// file or to the configuration.
const setupTokenSecret = "setup_token_hash"

// Exit codes. AI.md PART 8 fixes 0 for a clean exit and 1 for a
// startup failure; 2 is reserved for a command-line error, which the
// flag package already treats as distinct from a runtime failure.
const (
	// ExitOK reports a clean exit.
	ExitOK = 0
	// ExitFailure reports a startup or runtime failure.
	ExitFailure = 1
	// ExitUsage reports an unusable command line.
	ExitUsage = 2
)

// readyHook is called once the sequence has completed step 20 and is
// about to block in step 21. It is the single point at which a caller
// inside this package can observe readiness; tests replace it so they
// can drive the main loop deterministically instead of guessing how
// long startup takes.
var readyHook = func(*Server) {}

// IO carries the process streams so the sequence can be driven under
// test without touching the real stdout and stderr.
type IO struct {
	Out io.Writer
	Err io.Writer
}

// Server is a started server: every subsystem the sequence brought up,
// held so that Shutdown can take them back down in reverse order.
type Server struct {
	// Paths are the locations resolved once in step 7.
	Paths paths.Paths
	// Config is the loaded configuration.
	Config *config.Config
	// Mode is the resolved application mode.
	Mode mode.State
	// Log is the configured logger.
	Log *logging.Logger
	// ServerDB holds the operational tables.
	ServerDB *database.DB
	// UsersDB holds the identity tables.
	UsersDB *database.DB
	// FirstRun reports whether this run created the configuration.
	FirstRun bool
	// SetupToken is the one-time first-run token. It is empty on every
	// subsequent run and is never persisted in plaintext.
	SetupToken string
	// Started is the instant the sequence began, which the health
	// document (PART 13) reports as the process uptime.
	Started time.Time
	// HTTP is the listener set from PART 14, held so shutdown can drain
	// it before the databases behind it close.
	HTTP *server.Server
	// SSL is the certificate manager from PART 15. It is nil when no
	// HTTPS listener is configured, and the scheduler's daily renewal
	// check reads it once PART 19 is wired in.
	SSL *ssl.Manager
	// URLVars resolves the request-facing URL variables and the client
	// address the middleware chain keys on.
	URLVars *urlvars.Resolver

	// pidPath is the PID file to remove on shutdown. It is empty when
	// no PID file was written, which is always the case in a container.
	pidPath string
	// forceColor is the parsed --color flag: nil for auto-detection.
	forceColor *bool
	// shuttingDown reports that teardown has begun, so the health
	// document answers "shutting_down" while connections drain.
	shuttingDown bool
	// acmeCancel stops a first-issuance attempt still in flight, and
	// acmeWG waits for it, so no goroutine outlives shutdown.
	acmeCancel context.CancelFunc
	acmeWG     sync.WaitGroup
}

// Run executes the whole sequence and blocks until a shutdown signal
// arrives. It returns the process exit code.
func Run(ctx context.Context, args []string, streams IO) int {
	name := cli.BinaryName()

	// Step 1: parse the command line.
	opts, err := cli.Parse(name, args, streams.Err)
	if err != nil {
		// The flag package has already reported the problem and the
		// usage text through streams.Err.
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}

	// Step 2: immediate-exit flags, handled before anything starts.
	if code, done := handleImmediate(ctx, opts, name, streams); done {
		return code
	}

	// Daemonization happens before any resource is claimed, so the
	// parent never opens a log file or a PID file the child also wants.
	if opts.Daemon && !daemon.IsChild() {
		pid, err := daemon.Daemonize(daemon.Options{Args: daemon.FilterDaemonFlag(args)})
		if err != nil {
			fmt.Fprintf(streams.Err, "%s: %v\n", name, err)
			return ExitFailure
		}
		fmt.Fprintf(streams.Out, "Daemon started with PID %d\n", pid)
		return ExitOK
	}

	server, err := Start(ctx, opts, streams)
	if err != nil {
		fmt.Fprintf(streams.Err, "%s: %v\n", name, err)
		return ExitFailure
	}

	// Step 19: signal handlers, registered only once the server is up
	// so that a signal can never race a half-built subsystem.
	listener := signals.Listen(signals.Handlers{
		Shutdown:   func(context.Context) {},
		ReopenLogs: func() { server.reopenLogs() },
		DumpStatus: func() { server.dumpStatus() },
	})
	defer listener.Stop()

	// Step 20: the startup banner and the startup log lines.
	server.announce(streams)
	readyHook(server)

	// Step 21: block until a shutdown signal or a cancelled context.
	select {
	case <-listener.Done():
		server.Log.Infof("Received %s, shutting down", listener.Signal())
	case <-ctx.Done():
		server.Log.Infof("Context cancelled, shutting down")
	}

	if err := server.Shutdown(); err != nil {
		fmt.Fprintf(streams.Err, "%s: %v\n", name, err)
		return ExitFailure
	}
	return ExitOK
}

// handleImmediate runs the PHASE 1 flags that print and exit without
// starting anything. The second return value reports whether the
// process is finished.
func handleImmediate(ctx context.Context, opts *cli.Options, name string, streams IO) (int, bool) {
	switch {
	case opts.Help:
		fmt.Fprint(streams.Out, cli.Help(name))
		return ExitOK, true
	case opts.Version:
		fmt.Fprintln(streams.Out, version.String(name))
		return ExitOK, true
	case opts.Shell != "":
		return cli.RunShell(opts.Shell, opts.ShellName, name, streams.Out, streams.Err), true
	case opts.Status:
		return statusCommand(ctx, opts, streams), true
	}
	return ExitOK, false
}

// statusCommand resolves just enough configuration to find the running
// server, then hands over to the health client. It never starts a
// subsystem and never writes to a log file.
func statusCommand(ctx context.Context, opts *cli.Options, streams IO) int {
	resolved := paths.ResolveWith(overridesFrom(opts))
	cfg, err := config.Load(resolved)
	if err != nil {
		fmt.Fprintf(streams.Err, "status: %v\n", err)
		return ExitFailure
	}

	target := cli.StatusOptions{
		Address: cfg.Server.Listen,
		Port:    cfg.Server.Port,
		BaseURL: cfg.Server.BaseURL,
	}
	if opts.Address != "" {
		target.Address = opts.Address
	}
	if ports, err := opts.Ports(); err == nil && len(ports) > 0 {
		target.Port = ports[0]
	}
	if opts.BaseURL != "" {
		target.BaseURL = opts.BaseURL
	}
	return cli.Status(ctx, target, streams.Out, streams.Err)
}

// Start executes steps 6 through 15 and returns the running server. On
// failure it unwinds whatever it already brought up, so a caller never
// inherits a half-started process.
func Start(ctx context.Context, opts *cli.Options, streams IO) (*Server, error) {
	// Step 6: run context. The privilege level is locked the moment the
	// paths package initializes, before anything can drop privileges.
	s := &Server{
		Mode:    mode.Resolve(opts.Mode, opts.DebugFlag()),
		Started: time.Now(),
	}

	// Step 7: resolve every path once, CLI flag over environment
	// variable over OS default.
	s.Paths = paths.ResolveWith(overridesFrom(opts))

	// Steps 8b and 9: create the directory tree. The elevated branch
	// additionally creates the service user and chowns the tree; that is
	// PART 24 work and attaches here.
	if err := paths.EnsureAll(s.Paths); err != nil {
		return nil, err
	}
	if err := paths.EnsurePIDFile(s.Paths.PIDFile, paths.DirMode()); err != nil {
		return nil, err
	}

	// A first run is decided before the configuration is loaded,
	// because Load itself creates the file it would be tested for.
	s.FirstRun = !fileExists(s.Paths.ConfigFile)

	colorFlag, err := color.ParseColorFlag(opts.Color)
	if err != nil {
		return nil, err
	}
	s.forceColor = colorFlag
	useColor := color.ColorEnabled(colorFlag)

	// Step 10: bring logging up on the defaults so that every later
	// step has somewhere to report a failure.
	defaults := config.DefaultConfig()
	logger, err := logging.New(defaults.Server.Logs, s.Paths.Logs, logging.Options{
		Debug:        s.Mode.Debug,
		Console:      streams.Out,
		ConsoleColor: useColor,
		Version:      version.Version(),
	})
	if err != nil {
		return nil, err
	}
	s.Log = logger
	s.Log.Infof("Server starting, version %s", version.Version())

	// Steps 11 and 12: the PID file. Containers get none at all — the
	// runtime supervises the process and a namespace-local PID is wrong
	// when read from outside the namespace.
	if err := s.writePIDFile(); err != nil {
		return nil, s.unwind(err)
	}

	// Step 13: load the configuration. An invalid value is replaced
	// with its default and warned about; it never fails startup.
	cfg, err := config.Load(s.Paths)
	if err != nil {
		return nil, s.unwind(err)
	}
	cfg.ApplyRuntimeEnv()
	applyCLIOverrides(cfg, opts)
	s.Config = cfg
	for _, warning := range cfg.Warnings() {
		s.Log.Warnf("%s", warning)
	}

	// Step 14: rebuild logging from the loaded configuration.
	if err := s.reconfigureLogging(streams, useColor); err != nil {
		return nil, s.unwind(err)
	}

	// Step 15: open both databases and converge their schemas.
	if err := s.openDatabases(ctx); err != nil {
		return nil, s.unwind(err)
	}

	// Steps 16 and 17 — the scheduler and the overlay networks — attach
	// here, each owned by its own PART.

	// Step 18: the listeners, the middleware chain, and the certificate
	// manager (PART 14 and PART 15).
	if err := s.startHTTP(ctx); err != nil {
		return nil, s.unwind(err)
	}

	if s.FirstRun {
		token, err := s.ensureSetupToken(ctx)
		if err != nil {
			return nil, s.unwind(err)
		}
		s.SetupToken = token
	}

	return s, nil
}

// writePIDFile performs steps 11 and 12.
func (s *Server) writePIDFile() error {
	if pidfile.IsContainer() {
		s.Log.Infof("Container runtime detected, skipping the PID file")
		return nil
	}
	if err := pidfile.Write(s.Paths.PIDFile); err != nil {
		return err
	}
	s.pidPath = s.Paths.PIDFile
	return nil
}

// reconfigureLogging performs step 14. The bootstrap logger is closed
// only once its replacement is open, so no window exists in which a
// failure has nowhere to be reported.
func (s *Server) reconfigureLogging(streams IO, useColor bool) error {
	replacement, err := logging.New(s.Config.Server.Logs, s.Paths.Logs, logging.Options{
		Debug:        s.Mode.Debug,
		Console:      streams.Out,
		ConsoleColor: useColor,
		Version:      version.Version(),
	})
	if err != nil {
		return err
	}

	previous := s.Log
	s.Log = replacement
	return previous.Close()
}

// openDatabases performs step 15.
func (s *Server) openDatabases(ctx context.Context) error {
	serverDB, err := database.OpenServer(s.Config.Server.Database, s.Paths.DB)
	if err != nil {
		return err
	}
	s.ServerDB = serverDB
	if err := database.EnsureServerSchema(ctx, serverDB); err != nil {
		return err
	}

	usersDB, err := database.OpenUsers(s.Config.Server.Database, s.Paths.DB)
	if err != nil {
		return err
	}
	s.UsersDB = usersDB
	if err := database.EnsureUsersSchema(ctx, usersDB); err != nil {
		return err
	}

	s.Log.Infof("Database ready (driver %s)", serverDB.Driver())
	return nil
}

// ensureSetupToken generates the one-time first-run token. Only its
// hash reaches the database, and the plaintext is returned for a single
// console line — it is never logged.
func (s *Server) ensureSetupToken(ctx context.Context) (string, error) {
	token, err := security.GenerateSetupToken()
	if err != nil {
		return "", err
	}

	stored, _, err := database.EnsureSecret(ctx, s.ServerDB, setupTokenSecret, func() (string, error) {
		return security.HashToken(token), nil
	})
	if err != nil {
		return "", err
	}

	// A token already present means an earlier first run created one
	// that was never consumed; that token cannot be recovered from its
	// hash, so nothing is displayed.
	if stored != security.HashToken(token) {
		return "", nil
	}
	return token, nil
}

// announce performs step 20.
func (s *Server) announce(streams IO) {
	urls := s.listenURLs()
	banner.PrintStartupBanner(banner.BannerConfig{
		AppName:    s.Config.Server.ApplicationName,
		Version:    version.Version(),
		AppMode:    string(s.Mode.Mode),
		Debug:      s.Mode.Debug,
		URLs:       urls,
		ShowSetup:  s.SetupToken != "",
		SetupToken: s.SetupToken,
		ForceColor: s.forceColor,
		ForceEmoji: s.forceColor,
		Out:        streams.Out,
	})

	for _, url := range urls {
		s.Log.Infof("Listening on %s", url)
	}
	s.Log.Infof("Mode: %s", s.Mode.Mode)
}

// reopenLogs backs SIGUSR1.
func (s *Server) reopenLogs() {
	if err := s.Log.Reopen(); err != nil {
		s.Log.Errorf("Reopening log files: %v", err)
		return
	}
	s.Log.Status("Log files reopened")
}

// dumpStatus backs SIGUSR2. Every value it writes is already public in
// the health response, so the dump discloses nothing new.
func (s *Server) dumpStatus() {
	s.Log.Status(fmt.Sprintf("Status: version %s, mode %s, pid %d",
		version.Version(), s.Mode.Mode, os.Getpid()))
}

// Shutdown takes every started subsystem down in reverse order. It
// reports every failure rather than stopping at the first, so one stuck
// subsystem cannot strand the rest.
func (s *Server) Shutdown() error {
	var errs []error

	// The listeners go first: no new request may arrive while the
	// databases that would serve it are closing.
	s.shuttingDown = true
	if err := s.stopHTTP(); err != nil {
		errs = append(errs, err)
	}
	s.SSL = nil

	if s.UsersDB != nil {
		errs = append(errs, s.UsersDB.Close())
		s.UsersDB = nil
	}
	if s.ServerDB != nil {
		errs = append(errs, s.ServerDB.Close())
		s.ServerDB = nil
	}
	if s.pidPath != "" {
		errs = append(errs, pidfile.Remove(s.pidPath))
		s.pidPath = ""
	}
	if s.Log != nil {
		s.Log.Infof("Shutdown complete")
		errs = append(errs, s.Log.Close())
		s.Log = nil
	}

	return errors.Join(errs...)
}

// unwind tears down a partially started server and returns the original
// failure, with any teardown failure joined onto it.
func (s *Server) unwind(cause error) error {
	if err := s.Shutdown(); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// overridesFrom maps the directory flags onto the path overrides.
func overridesFrom(opts *cli.Options) paths.Overrides {
	return paths.Overrides{
		Config:  opts.Config,
		Data:    opts.Data,
		Cache:   opts.Cache,
		Logs:    opts.Log,
		Backup:  opts.Backup,
		PIDFile: opts.PIDFile,
	}
}

// applyCLIOverrides puts the command line on top of the configuration,
// which is the highest priority in the PART 12 precedence chain.
func applyCLIOverrides(cfg *config.Config, opts *cli.Options) {
	if opts.Address != "" {
		cfg.Server.Listen = opts.Address
	}
	if ports, err := opts.Ports(); err == nil && len(ports) > 0 {
		cfg.Server.Port = ports[0]
		// A second --port value names the TLS listener, which PART 15
		// keeps separate from the plaintext one.
		if len(ports) > 1 {
			cfg.Server.SSL.Port = ports[1]
		}
	}
	if opts.BaseURL != "" {
		cfg.Server.BaseURL = opts.BaseURL
	}
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
