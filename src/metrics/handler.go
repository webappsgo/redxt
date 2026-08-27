// Package metrics also implements the HTTP surface AI.md PART 21
// requires: /server/metrics dispatches to one of three services
// (prometheus, grafana, loki) by an optional {service} path segment,
// each gated by its own bearer token per PART 21's "Authentication"
// table.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

// Handler serves the prometheus, grafana, and loki metrics services
// behind their own per-service bearer tokens.
type Handler struct {
	registry *Registry
	cfg      func() config.MetricsAuth
	prefix   string
	title    string
	loki     *LokiBuffer
}

// NewHandler builds the metrics HTTP handler. cfg is read on every
// request (not just at startup) so a config reload or admin-panel
// token rotation takes effect immediately without a restart.
func NewHandler(registry *Registry, prefix, title string, loki *LokiBuffer, authCfg func() config.MetricsAuth) *Handler {
	return &Handler{registry: registry, cfg: authCfg, prefix: prefix, title: title, loki: loki}
}

// ServeHTTP implements the routing table from AI.md PART 21: the
// trailing path segment selects the service, defaulting to
// "prometheus" when absent (so /server/metrics === /server/metrics/prometheus).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	service := serviceFromPath(r.URL.Path)

	auth := h.cfg()
	token := tokenFor(auth, service)

	if !auth.AllowUnauthenticated {
		if token == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !bearerMatches(r, token) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	switch service {
	case "grafana":
		body, err := Dashboard(h.prefix, h.title)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	case "loki":
		body, err := h.loki.JSON(time.Now())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	default:
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(h.registry.WriteText()))
	}
}

// serviceFromPath returns the trailing path segment ("prometheus",
// "grafana", "loki"), defaulting to "prometheus" when the request
// came in on a bare /server/metrics, /metrics, or /api/metrics route.
func serviceFromPath(path string) string {
	segs := splitPath(path)
	if len(segs) == 0 {
		return "prometheus"
	}
	last := segs[len(segs)-1]
	switch last {
	case "prometheus", "grafana", "loki":
		return last
	default:
		return "prometheus"
	}
}

func tokenFor(auth config.MetricsAuth, service string) string {
	switch service {
	case "grafana":
		return auth.Tokens.Grafana
	case "loki":
		return auth.Tokens.Loki
	default:
		return auth.Tokens.Prometheus
	}
}

// bearerMatches reports whether the request carries
// "Authorization: Bearer {token}" matching token, using a
// constant-time comparison. Query-string tokens are never accepted
// per PART 21 (they leak into access logs and proxies).
func bearerMatches(r *http.Request, token string) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	presented := strings.TrimPrefix(h, prefix)
	return subtle.ConstantTimeCompare([]byte(presented), []byte(token)) == 1
}
