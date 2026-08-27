package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
)

func TestHandlerRequiresTokenWhenNotAllowingUnauthenticated(t *testing.T) {
	r := New("redxt")
	loki := NewLokiBuffer(10, time.Hour)
	h := NewHandler(r, "redxt", "redxt", loki, func() config.MetricsAuth {
		return config.MetricsAuth{Tokens: config.MetricsTokens{Prometheus: "secret"}}
	})

	req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct token: status = %d, want 200", rec.Code)
	}
}

func TestHandlerEmptyTokenDisablesService(t *testing.T) {
	r := New("redxt")
	loki := NewLokiBuffer(10, time.Hour)
	h := NewHandler(r, "redxt", "redxt", loki, func() config.MetricsAuth {
		return config.MetricsAuth{} // every token empty
	})

	req := httptest.NewRequest(http.MethodGet, "/server/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body on disabled service, got %q", rec.Body.String())
	}
}

func TestHandlerAllowUnauthenticatedServesEachService(t *testing.T) {
	r := New("redxt")
	r.RegisterApp(AppInfo{Version: "1.0.0"}, time.Now())
	loki := NewLokiBuffer(10, time.Hour)
	loki.Add(map[string]string{"app": "redxt"}, "hello", time.Now())
	h := NewHandler(r, "redxt", "redxt", loki, func() config.MetricsAuth {
		return config.MetricsAuth{AllowUnauthenticated: true}
	})

	for _, path := range []string{"/server/metrics", "/server/metrics/prometheus", "/server/metrics/grafana", "/server/metrics/loki"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s: expected non-empty body", path)
		}
	}
}

func TestServiceFromPath(t *testing.T) {
	tests := map[string]string{
		"/server/metrics":         "prometheus",
		"/server/metrics/grafana": "grafana",
		"/server/metrics/loki":    "loki",
		"/server/metrics/bogus":   "prometheus",
		"/metrics":                "prometheus",
	}
	for path, want := range tests {
		if got := serviceFromPath(path); got != want {
			t.Errorf("serviceFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}
