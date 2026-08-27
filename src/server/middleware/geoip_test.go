package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// geoRequest builds a request from the given TCP peer.
func geoRequest(ip string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/records", nil)
	req.RemoteAddr = ip + ":41000"
	return req
}

func TestGeoIPNoLookupIsPassthrough(t *testing.T) {
	called := false
	mw := GeoIP(GeoIPOptions{})
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).
		ServeHTTP(rr, geoRequest("203.0.113.9"))
	if !called {
		t.Fatalf("passthrough did not reach next handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (httptest.NewRecorder default with no handler write)", rr.Code)
	}
}

func TestGeoIPAnnotatesWithoutBlocked(t *testing.T) {
	var got GeoResult
	var ok bool
	mw := GeoIP(GeoIPOptions{
		Lookup: func(string) (GeoResult, bool) {
			return GeoResult{Country: "US"}, true
		},
	})
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		got, ok = GeoFromContext(req.Context())
	})).ServeHTTP(rr, geoRequest("203.0.113.9"))

	if !ok || got.Country != "US" {
		t.Fatalf("GeoFromContext() = %+v, %v; want {Country: US}, true", got, ok)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; a nil Blocked seam must never gate", rr.Code)
	}
}

func TestGeoIPBlockedRefusesRequest(t *testing.T) {
	nextCalled := false
	mw := GeoIP(GeoIPOptions{
		Lookup:  func(string) (GeoResult, bool) { return GeoResult{Country: "CN"}, true },
		Blocked: func(ip string) bool { return ip == "203.0.113.9" },
	})
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true })).
		ServeHTTP(rr, geoRequest("203.0.113.9"))

	if nextCalled {
		t.Fatalf("next handler ran for a blocked address")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestGeoIPBlockedAllowsUnlistedAddress(t *testing.T) {
	nextCalled := false
	mw := GeoIP(GeoIPOptions{
		Lookup:  func(string) (GeoResult, bool) { return GeoResult{}, false },
		Blocked: func(ip string) bool { return ip == "203.0.113.9" },
	})
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true })).
		ServeHTTP(rr, geoRequest("198.51.100.4"))

	if !nextCalled {
		t.Fatalf("next handler did not run for an address Blocked reports false for")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestGeoIPBlockedRespectsAllowlist(t *testing.T) {
	blockedCalls := 0
	nextCalled := false
	mw := GeoIP(GeoIPOptions{
		Lookup: func(string) (GeoResult, bool) { return GeoResult{}, false },
		Blocked: func(string) bool {
			blockedCalls++
			return true
		},
	})
	req := withValue(geoRequest("203.0.113.9"), allowlistKey, true)
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true })).
		ServeHTTP(rr, req)

	if blockedCalls != 0 {
		t.Fatalf("Blocked was consulted for an allowlisted request")
	}
	if !nextCalled || rr.Code != http.StatusOK {
		t.Fatalf("allowlisted request was refused: called=%v status=%d", nextCalled, rr.Code)
	}
}
