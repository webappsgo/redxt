package middleware

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/webappsgo/redxt/src/logging"
	"github.com/webappsgo/redxt/src/urlvars"
)

// LoggingOptions configures the PART 12 access-log stage.
type LoggingOptions struct {
	// Sink receives one Entry per completed request. Nil disables the
	// stage entirely — the recorder is not even installed, so a server
	// with logging off pays nothing for it.
	Sink func(logging.Entry)

	// ClientIP resolves the trusted-proxy-aware client address. Nil
	// falls back to the request's TCP peer.
	ClientIP func(*http.Request) string

	// Now supplies the current time. Nil uses time.Now. Tests inject a
	// clock here so a latency assertion is exact rather than raced.
	Now func() time.Time

	// SkipPaths lists paths that are not logged. Health checks polled
	// every few seconds otherwise drown the log in noise that says
	// nothing.
	SkipPaths []string
}

// Logging returns the PART 12 access-log middleware.
//
// It is the innermost stage, so its timing covers the router and the
// handler and nothing else — the chain's own overhead is not billed to
// the request. That placement also means a request refused earlier (a
// traversal attempt, a blocked address, a rate-limited client) is not in
// the access log; those rejections belong to the security log, which
// records the reason the access log has no field for.
//
// The query string is recorded with its sensitive parameters redacted,
// so a credential that arrived as ?token= does not survive into a log
// file, per PART 11.
func Logging(opts LoggingOptions) Middleware {
	if opts.Sink == nil {
		return passthrough
	}
	clientIP := opts.ClientIP
	if clientIP == nil {
		clientIP = remoteHost
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if matchesAnyPath(req.URL.Path, opts.SkipPaths) {
				next.ServeHTTP(w, req)
				return
			}

			started := now()
			recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, req)
			finished := now()

			entry := logging.Entry{
				Time:       finished,
				RemoteIP:   clientIP(req),
				Method:     req.Method,
				Path:       req.URL.Path,
				Query:      RedactQueryParams(req.URL.RawQuery),
				Status:     recorder.status,
				Bytes:      recorder.bytes,
				Latency:    finished.Sub(started),
				UserAgent:  req.UserAgent(),
				Referer:    req.Referer(),
				RequestID:  urlvars.RequestIDFromContext(req.Context()),
				FQDN:       req.Host,
				Protocol:   req.Proto,
				TLSVersion: tlsVersion(req),
			}
			if geo, ok := GeoFromContext(req.Context()); ok {
				entry.Country = geo.Country
				entry.ASN = geo.ASN
			}

			opts.Sink(entry)
		})
	}
}

// tlsVersion names the negotiated TLS version, or "" for plain HTTP.
func tlsVersion(req *http.Request) string {
	if req == nil || req.TLS == nil {
		return ""
	}
	return tls.VersionName(req.TLS.Version)
}

// responseRecorder wraps a ResponseWriter to capture the status code and
// the body size the access log needs.
//
// It forwards Flush and Hijack rather than swallowing them: a
// ResponseWriter that loses Flush breaks server-sent events, and one
// that loses Hijack breaks the WebSocket handshake. Unwrap lets
// http.ResponseController reach any other optional method on the
// underlying writer without this type having to know about it.
type responseRecorder struct {
	http.ResponseWriter
	// status is the code passed to WriteHeader, defaulting to 200 for a
	// handler that wrote a body without setting one.
	status int
	// bytes counts the body bytes written.
	bytes int64
	// wroteHeader guards against a second WriteHeader overwriting the
	// recorded status, matching what the client actually received.
	wroteHeader bool
}

// WriteHeader records the status and forwards it.
func (r *responseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

// Write counts the body bytes and forwards them.
func (r *responseRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Flush forwards a flush when the underlying writer supports one.
func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack forwards a connection hijack, which the WebSocket handshake
// needs, and reports a clear error when the underlying writer cannot.
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("middleware: underlying ResponseWriter does not support hijacking")
	}
	return hijacker.Hijack()
}
