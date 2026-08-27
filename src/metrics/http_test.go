package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizePathCollapsesIDs(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/users/42", "/users/:id"},
		{"/users/550e8400-e29b-41d4-a716-446655440000", "/users/:id"},
		{"/api/v1/zones", "/api/v1/zones"},
		{"", "/"},
		{"/", "/"},
	}
	for _, tc := range tests {
		if got := NormalizePath(tc.in); got != tc.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMiddlewareRecordsRequest(t *testing.T) {
	r := New("redxt")
	handler := Middleware(r, []float64{0.1, 1}, []float64{100, 1000})(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	r.mu.Lock()
	f := r.counters["http_requests_total"]
	r.mu.Unlock()
	key := seriesKey(map[string]string{"method": "GET", "path": "/users/:id", "status": "201"})
	if got := f.series[key].get(); got != 1 {
		t.Fatalf("http_requests_total = %v, want 1", got)
	}

	if got := r.activeRequestsDelta(0); got != 0 {
		t.Fatalf("active requests after completion = %v, want 0", got)
	}
}

func TestStatusOrDefault(t *testing.T) {
	if got := statusOrDefault(0); got != http.StatusOK {
		t.Fatalf("statusOrDefault(0) = %d, want 200", got)
	}
	if got := statusOrDefault(404); got != 404 {
		t.Fatalf("statusOrDefault(404) = %d, want 404", got)
	}
}
