package metrics

import (
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// idSegment matches path segments that look like an opaque
// identifier (UUID, numeric ID, or long token) so they can be
// collapsed to ":id" before being used as a label value. Without
// this, per-resource paths would blow up label cardinality, which
// AI.md PART 21 explicitly forbids.
var idSegment = regexp.MustCompile(`^([0-9]+|[0-9a-fA-F-]{8,}|[A-Za-z0-9_-]{20,})$`)

// NormalizePath collapses identifier-shaped path segments to ":id" so
// the http_* metrics stay low-cardinality regardless of how many
// distinct resources exist.
func NormalizePath(path string) string {
	if path == "" {
		return "/"
	}
	segs := splitPath(path)
	for i, s := range segs {
		if idSegment.MatchString(s) {
			segs[i] = ":id"
		}
	}
	return joinPath(segs)
}

func splitPath(path string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				out = append(out, path[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func joinPath(segs []string) string {
	if len(segs) == 0 {
		return "/"
	}
	out := ""
	for _, s := range segs {
		out += "/" + s
	}
	return out
}

// countingResponseWriter captures the status code and body size
// written by the wrapped handler.
type countingResponseWriter struct {
	http.ResponseWriter
	status int
	size   int64
}

func (w *countingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *countingResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += int64(n)
	return n, err
}

// Middleware returns HTTP middleware that records the PART 21
// required HTTP metrics: requests_total, request_duration_seconds,
// request_size_bytes, response_size_bytes, and active_requests.
func Middleware(r *Registry, durationBuckets, sizeBuckets []float64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			r.Gauge("http_active_requests", "In-flight HTTP requests.", nil, r.activeRequestsDelta(1))

			start := time.Now()
			cw := &countingResponseWriter{ResponseWriter: w}
			next.ServeHTTP(cw, req)
			duration := time.Since(start).Seconds()

			r.Gauge("http_active_requests", "In-flight HTTP requests.", nil, r.activeRequestsDelta(-1))

			path := NormalizePath(req.URL.Path)
			labels := map[string]string{"method": req.Method, "path": path}
			r.Counter("http_requests_total", "Total HTTP requests.",
				map[string]string{"method": req.Method, "path": path, "status": strconv.Itoa(statusOrDefault(cw.status))}, 1)
			r.Histogram("http_request_duration_seconds", "HTTP request latency.", durationBuckets, labels, duration)
			r.Histogram("http_request_size_bytes", "HTTP request body size.", sizeBuckets, labels, float64(req.ContentLength))
			r.Histogram("http_response_size_bytes", "HTTP response body size.", sizeBuckets, labels, float64(cw.size))
		})
	}
}

func statusOrDefault(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}

// activeRequestsDelta adjusts and returns the in-flight request
// count. It uses the registry's own counter storage rather than a
// separate field so Middleware needs no extra synchronization beyond
// what Registry already provides.
func (r *Registry) activeRequestsDelta(delta float64) float64 {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	r.active += delta
	if r.active < 0 {
		r.active = 0
	}
	return r.active
}
