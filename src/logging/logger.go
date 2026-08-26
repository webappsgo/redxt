// Package logging implements redxt's logging subsystem: the six log
// destinations, their output formats, built-in file rotation and
// retention, the audit log, and the dependency-free ULID generator the
// audit entries are keyed by.
//
// It implements AI.md PART 11 ("Logging" and "Audit Log"): the log
// file table and their default formats, health-check log suppression,
// the apache, nginx, json, text, fail2ban, syslog, cef, and custom
// formats, the rotation and retention option tables, and the audit
// entry shape.
//
// The governing rule of PART 11 is that log FILES are plain ASCII with
// no emojis, no ANSI color, and no control characters, while console
// output may be pretty. This package enforces that split in the type
// system: everything written to a file passes through plainLine, whose
// only constructors sanitize their input, so a colored or multi-line
// value cannot reach a log file by accident.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/security"
)

// Log format names from the PART 11 format tables.
const (
	FormatApacheName   = "apache"
	FormatNginxName    = "nginx"
	FormatJSONName     = "json"
	FormatTextName     = "text"
	FormatCustomName   = "custom"
	FormatFail2banName = "fail2ban"
	FormatSyslogName   = "syslog"
	FormatCEFName      = "cef"
)

// healthCheckEndpoints are the health endpoint names PART 13 exposes
// and PART 11 suppresses from the access log when they succeed.
var healthCheckEndpoints = []string{"healthz", "readyz", "livez"}

// cefDefaultSeverity is the CEF severity assigned to security events,
// mid-scale on the 0-10 CEF range.
const cefDefaultSeverity = 5

// Options carries the process-level settings the configuration tree
// does not hold.
type Options struct {
	// Debug forces debug-level logging and disables health-check
	// suppression, matching the PART 11 rule that no health-check
	// request is suppressed while debug is enabled.
	Debug bool
	// Console receives a human-readable mirror of every server
	// message. It is normally os.Stdout and may be nil, which
	// disables console output entirely.
	Console io.Writer
	// ConsoleColor allows ANSI color and emojis on the console mirror.
	// It must be false when NO_COLOR is set, TERM is dumb, or the
	// output is not a terminal. It never affects file output.
	ConsoleColor bool
	// Hostname is the HOSTNAME field of syslog lines. When empty the
	// operating system hostname is used.
	Hostname string
	// NodeID fills the node_id field of audit entries that do not set
	// one themselves.
	NodeID string
	// Version is the product version written into the CEF header.
	Version string
}

// plainLine is a log line that is guaranteed safe for a log file: no
// ANSI escapes, no control characters, no non-ASCII bytes. It can only
// be produced by newPlainLine, which sanitizes, so the "files are
// plain ASCII" rule cannot be broken by a caller that forgets.
type plainLine string

// newPlainLine sanitizes s into a line safe to append to a log file.
func newPlainLine(s string) plainLine {
	return plainLine(sanitize(s))
}

// destination is one open log file together with the format it is
// written in.
type destination struct {
	file   *File
	format string
	custom string
}

// write appends one sanitized line, terminated by a newline. A nil or
// disabled destination silently discards its input, so callers do not
// need to test whether a log is enabled.
func (d *destination) write(line plainLine) error {
	if d == nil || d.file == nil {
		return nil
	}
	_, err := d.file.Write([]byte(string(line) + "\n"))
	return err
}

// Logger is the process-wide logging facade. It owns the six PART 11
// destinations and is safe for concurrent use.
type Logger struct {
	level slog.Level
	debug bool

	consoleOut   io.Writer
	consoleColor bool
	consoleMu    sync.Mutex

	hostname string
	nodeID   string
	version  string

	logHealthChecks bool
	auditEnabled    bool

	access   *destination
	server   *destination
	errorLog *destination
	audit    *destination
	security *destination
	debugLog *destination

	closeMu sync.Mutex
	closed  bool
}

// New opens every enabled log file in dir and returns the logger. The
// debug log is opened only when the configuration enables it; every
// other destination is always open, matching the PART 11 log file
// table.
func New(cfg config.Logs, dir string, opts Options) (*Logger, error) {
	l := &Logger{
		level:           parseLevel(cfg.Level),
		debug:           opts.Debug,
		consoleOut:      opts.Console,
		consoleColor:    opts.ConsoleColor,
		hostname:        opts.Hostname,
		nodeID:          opts.NodeID,
		version:         opts.Version,
		logHealthChecks: cfg.Access.LogHealthChecks,
		auditEnabled:    cfg.Audit.Enabled,
	}
	if opts.Debug {
		l.level = slog.LevelDebug
	}
	if l.hostname == "" {
		if name, err := os.Hostname(); err == nil {
			l.hostname = name
		}
	}

	var opened []*File
	fail := func(err error) (*Logger, error) {
		for _, f := range opened {
			_ = f.Close()
		}
		return nil, err
	}

	open := func(lf config.LogFile, compress bool) (*destination, error) {
		file, err := OpenCompressed(dir, lf.Filename, lf.Rotate, lf.Keep, compress)
		if err != nil {
			return nil, err
		}
		opened = append(opened, file)
		return &destination{file: file, format: strings.ToLower(lf.Format), custom: lf.Custom}, nil
	}

	var err error
	if l.access, err = open(cfg.Access.LogFile, false); err != nil {
		return fail(err)
	}
	if l.server, err = open(cfg.Server, false); err != nil {
		return fail(err)
	}
	if l.errorLog, err = open(cfg.Error, false); err != nil {
		return fail(err)
	}
	if l.audit, err = open(cfg.Audit.LogFile, cfg.Audit.Compress); err != nil {
		return fail(err)
	}
	if l.security, err = open(cfg.Security, false); err != nil {
		return fail(err)
	}
	if cfg.Debug.Enabled {
		if l.debugLog, err = open(cfg.Debug.LogFile, false); err != nil {
			return fail(err)
		}
	}

	return l, nil
}

// Level returns the configured minimum level.
func (l *Logger) Level() slog.Level {
	return l.level
}

// Debugf logs a formatted message at debug level.
func (l *Logger) Debugf(format string, args ...any) {
	l.Log(context.Background(), slog.LevelDebug, fmt.Sprintf(format, args...))
}

// Infof logs a formatted message at info level.
func (l *Logger) Infof(format string, args ...any) {
	l.Log(context.Background(), slog.LevelInfo, fmt.Sprintf(format, args...))
}

// Warnf logs a formatted message at warn level.
func (l *Logger) Warnf(format string, args ...any) {
	l.Log(context.Background(), slog.LevelWarn, fmt.Sprintf(format, args...))
}

// Errorf logs a formatted message at error level.
func (l *Logger) Errorf(format string, args ...any) {
	l.Log(context.Background(), slog.LevelError, fmt.Sprintf(format, args...))
}

// Log writes one structured message to the application logs.
//
// Routing follows the PART 11 log file table. Every message at or
// above the configured level goes to server.log; warn and error
// messages are mirrored to error.log as well, so error.log is a
// complete record of everything that went wrong without having to be
// read alongside server.log. When the debug log is enabled it receives
// every message regardless of level, because it exists purely for
// troubleshooting. The context is accepted for call-site compatibility
// with log/slog and carries no logging state of its own.
func (l *Logger) Log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	_ = ctx
	now := time.Now()

	if level >= l.level {
		l.emit(l.server, now, level, msg, attrs)
		if level >= slog.LevelWarn {
			l.emit(l.errorLog, now, level, msg, attrs)
		}
		l.writeConsole(level, msg, attrs)
	}
	l.emit(l.debugLog, now, level, msg, attrs)
}

// Status writes an operator-requested status dump. It bypasses the
// level gate because the dump exists only in response to an explicit
// SIGUSR2, so discarding it would make the signal useless on the
// default warn level, and it is not mirrored to error.log because a
// requested dump is not a failure.
func (l *Logger) Status(msg string, attrs ...slog.Attr) {
	now := time.Now()
	l.emit(l.server, now, slog.LevelInfo, msg, attrs)
	l.emit(l.debugLog, now, slog.LevelInfo, msg, attrs)
	l.writeConsole(slog.LevelInfo, msg, attrs)
}

// Access writes one access-log record in the configured format,
// applying health-check suppression first.
func (l *Logger) Access(e Entry) {
	if l.access == nil || l.access.file == nil {
		return
	}
	if l.suppressAccess(e) {
		return
	}

	var line string
	switch l.access.format {
	case FormatNginxName:
		line = FormatNginx(e)
	case FormatJSONName:
		encoded, err := FormatAccessJSON(e)
		if err != nil {
			l.Errorf("access log encode failed: %v", err)
			return
		}
		line = string(encoded)
	case FormatCustomName:
		line = FormatCustom(l.access.custom, e)
	default:
		line = FormatApache(e)
	}
	if err := l.access.write(newPlainLine(line)); err != nil {
		l.reportWriteFailure("access", err)
	}
}

// suppressAccess reports whether an access record is a successful
// health check that PART 11 keeps out of access.log. A non-2xx health
// response is never suppressed, and no request at all is suppressed
// while debug is enabled.
func (l *Logger) suppressAccess(e Entry) bool {
	if l.logHealthChecks || l.debug {
		return false
	}
	if e.Status < 200 || e.Status > 299 {
		return false
	}
	return IsHealthCheckPath(e.Path)
}

// IsHealthCheckPath reports whether a request path addresses a health
// endpoint: the canonical "/api/{version}/server/healthz" form, the
// root aliases "/healthz", "/readyz", and "/livez", and the base-URL
// prefixed form of those aliases such as "/redxt/healthz".
//
// The match is on the final path segment plus its position, because
// the API version and the configured base URL are not known here. A
// deeper path only matches when the preceding segment is "server",
// which is the canonical API layout.
func IsHealthCheckPath(path string) bool {
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return false
	}

	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	last := strings.ToLower(segments[len(segments)-1])
	if !isHealthEndpoint(last) {
		return false
	}
	if len(segments) <= 2 {
		return true
	}
	return strings.EqualFold(segments[len(segments)-2], "server")
}

// isHealthEndpoint reports whether a path segment names a health
// endpoint.
func isHealthEndpoint(segment string) bool {
	for _, name := range healthCheckEndpoints {
		if segment == name {
			return true
		}
	}
	return false
}

// Security writes one security-log record in the configured format.
// Attributes are redacted before they are rendered, so an event may
// carry a field such as "token" without leaking its value.
func (l *Logger) Security(event string, attrs ...slog.Attr) {
	if l.security == nil || l.security.file == nil {
		return
	}
	now := time.Now()

	var line string
	switch l.security.format {
	case FormatJSONName:
		encoded, err := FormatJSON(now, slog.LevelWarn, event, attrs)
		if err != nil {
			l.Errorf("security log encode failed: %v", err)
			return
		}
		line = string(encoded)
	case FormatTextName:
		line = FormatText(now, slog.LevelWarn, event, attrs)
	case FormatSyslogName:
		line = FormatSyslog(now, slog.LevelWarn, l.hostname, CEFProduct, joinMessage(event, attrs))
	case FormatCEFName:
		line = FormatCEF(l.version, event, event, cefDefaultSeverity, cefExtensions(attrs))
	default:
		line = FormatFail2ban(now, joinMessage(event, attrs))
	}
	if err := l.security.write(newPlainLine(line)); err != nil {
		l.reportWriteFailure("security", err)
	}
}

// Slog returns a *slog.Logger whose records are routed through this
// logger, so the rest of the codebase can use standard structured
// logging without ever touching a raw log writer.
func (l *Logger) Slog() *slog.Logger {
	return slog.New(&handler{logger: l})
}

// Reopen reopens every open log file in place. It backs the SIGUSR1
// handler and returns the joined errors of all destinations, so one
// failing file never hides another. A closed logger reopens nothing.
func (l *Logger) Reopen() error {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()

	if l.closed {
		return nil
	}

	var errs []error
	for _, d := range []*destination{l.access, l.server, l.errorLog, l.audit, l.security, l.debugLog} {
		if d == nil || d.file == nil {
			continue
		}
		if err := d.file.Reopen(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close closes every open log file. It is idempotent and returns the
// joined errors of all destinations, so one failing file never hides
// another.
func (l *Logger) Close() error {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	var errs []error
	for _, d := range []*destination{l.access, l.server, l.errorLog, l.audit, l.security, l.debugLog} {
		if d == nil || d.file == nil {
			continue
		}
		if err := d.file.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// emit renders a message for one destination and writes it. A write
// failure is reported to the console only: reporting it to a log file
// could fail the same way and loop.
func (l *Logger) emit(d *destination, t time.Time, level slog.Level, msg string, attrs []slog.Attr) {
	if d == nil || d.file == nil {
		return
	}

	var line string
	if d.format == FormatJSONName {
		encoded, err := FormatJSON(t, level, msg, attrs)
		if err != nil {
			l.consolef("[ERROR] log encode failed: %v", err)
			return
		}
		line = string(encoded)
	} else {
		line = FormatText(t, level, msg, attrs)
	}

	if err := d.write(newPlainLine(line)); err != nil {
		l.consolef("[ERROR] log write failed: %v", err)
	}
}

// reportWriteFailure records a failed write to a non-application log
// on error.log, so a broken access, security, or audit file is visible
// rather than silent.
func (l *Logger) reportWriteFailure(name string, err error) {
	l.Errorf("%s log write failed: %v", name, err)
}

// writeConsole mirrors a message to the console. Console output is the
// only place emojis and ANSI color are allowed, and both are dropped
// when ConsoleColor is false.
func (l *Logger) writeConsole(level slog.Level, msg string, attrs []slog.Attr) {
	if l.consoleOut == nil {
		return
	}

	text := sanitize(msg)
	if rendered := renderAttrs(attrs); rendered != "" {
		text += " " + rendered
	}

	if !l.consoleColor {
		l.consolef("%s %s", consolePlainTag(level), text)
		return
	}
	color, icon := consoleDecoration(level)
	l.consolef("%s%s %s\x1b[0m", color, icon, text)
}

// consolef writes one formatted line to the console under its own
// lock, so concurrent messages do not interleave.
func (l *Logger) consolef(format string, args ...any) {
	if l.consoleOut == nil {
		return
	}
	l.consoleMu.Lock()
	defer l.consoleMu.Unlock()
	_, _ = fmt.Fprintf(l.consoleOut, format+"\n", args...)
}

// consolePlainTag returns the PART 11 plain-text fallback tag used
// when color and emojis are disabled.
func consolePlainTag(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "[ERROR]"
	case level >= slog.LevelWarn:
		return "[WARN]"
	case level >= slog.LevelInfo:
		return "[INFO]"
	default:
		return "[DEBUG]"
	}
}

// consoleDecoration returns the ANSI color prefix and emoji for a
// level. These never reach a log file.
func consoleDecoration(level slog.Level) (string, string) {
	switch {
	case level >= slog.LevelError:
		return "\x1b[31m", "❌"
	case level >= slog.LevelWarn:
		return "\x1b[33m", "⚠️"
	case level >= slog.LevelInfo:
		return "\x1b[32m", "✅"
	default:
		return "\x1b[36m", "ℹ️"
	}
}

// joinMessage appends rendered attributes to a message, for the
// formats that carry a single free-text message rather than fields.
func joinMessage(msg string, attrs []slog.Attr) string {
	rendered := renderAttrs(attrs)
	if rendered == "" {
		return msg
	}
	return msg + " " + rendered
}

// cefExtensions flattens a redacted attribute set into CEF extension
// pairs.
func cefExtensions(attrs []slog.Attr) map[string]string {
	out := make(map[string]string)
	flattenFields("", security.RedactMap(attrsToMap(attrs)), out)
	return out
}

// parseLevel maps a configured level name onto a slog level. An
// unrecognized value falls back to the PART 12 default of warn, which
// is what the config validator would have written anyway.
func parseLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "error":
		return slog.LevelError
	default:
		return slog.LevelWarn
	}
}

// handler is the slog.Handler that routes standard structured logging
// through the Logger, so no caller ever holds a raw file writer.
type handler struct {
	logger *Logger
	attrs  []slog.Attr
	groups []string
}

// Enabled reports whether a level is logged. The debug log, when
// enabled, wants every record, so nothing is filtered out below the
// configured level in that case.
func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	if h.logger.debugLog != nil {
		return true
	}
	return level >= h.logger.level
}

// Handle routes one slog record to the Logger.
func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.qualify(a))
		return true
	})
	h.logger.Log(ctx, r.Level, r.Message, attrs...)
	return nil
}

// WithAttrs returns a handler with additional pre-set attributes.
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	next := h.clone()
	for _, a := range attrs {
		next.attrs = append(next.attrs, h.qualify(a))
	}
	return next
}

// WithGroup returns a handler that nests later attributes in a group.
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := h.clone()
	next.groups = append(next.groups, name)
	return next
}

// clone copies the handler so a derived handler never mutates its
// parent's slices.
func (h *handler) clone() *handler {
	next := &handler{logger: h.logger}
	next.attrs = append(next.attrs, h.attrs...)
	next.groups = append(next.groups, h.groups...)
	return next
}

// qualify wraps an attribute in the handler's open groups, so grouped
// keys render as "group.key".
func (h *handler) qualify(a slog.Attr) slog.Attr {
	for i := len(h.groups) - 1; i >= 0; i-- {
		a = slog.Attr{Key: h.groups[i], Value: slog.GroupValue(a)}
	}
	return a
}
