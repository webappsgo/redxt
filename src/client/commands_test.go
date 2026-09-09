package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/health"
)

func TestRunHealthHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health.Response{
			Project: health.ProjectInfo{Name: "redxt"},
			Status:  health.StatusHealthy,
			Version: "1.0.0",
			Mode:    "production",
			Uptime:  "1h",
		})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := RunHealth(NewHTTPClient(srv.URL, ""), &out, &errOut)
	if code != 0 {
		t.Fatalf("RunHealth() = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Status:   healthy")) {
		t.Errorf("RunHealth() output missing status line: %s", out.String())
	}
}

func TestRunHealthUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health.Response{Status: health.StatusUnhealthy})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := RunHealth(NewHTTPClient(srv.URL, ""), &out, &errOut)
	if code != 1 {
		t.Fatalf("RunHealth() = %d, want 1", code)
	}
}

func TestRunHealthTokenRevokedExitsWithAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apierror.Response{OK: false, Error: apierror.CodeTokenRevoked})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := RunHealth(NewHTTPClient(srv.URL, "usr_api_dead"), &out, &errOut)
	if code != ExitAuthError {
		t.Fatalf("RunHealth() = %d, want ExitAuthError (%d)", code, ExitAuthError)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("token has been revoked")) {
		t.Errorf("RunHealth() stderr = %q, want a token-revoked message", errOut.String())
	}
}

// TestRunHealthTokenExpiredExitsWithAuthError verifies AI.md's "Same
// behavior on 401 TOKEN_EXPIRED" instruction applies through RunHealth too.
func TestRunHealthTokenExpiredExitsWithAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apierror.Response{OK: false, Error: apierror.CodeTokenExpired})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := RunHealth(NewHTTPClient(srv.URL, "usr_api_stale"), &out, &errOut)
	if code != ExitAuthError {
		t.Fatalf("RunHealth() = %d, want ExitAuthError (%d)", code, ExitAuthError)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("token has expired")) {
		t.Errorf("RunHealth() stderr = %q, want a token-expired message", errOut.String())
	}
}

func TestRunHealthUnreachable(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunHealth(NewHTTPClient("", ""), &out, &errOut)
	if code != 1 {
		t.Fatalf("RunHealth() = %d, want 1", code)
	}
	if errOut.Len() == 0 {
		t.Error("RunHealth() should write an error message on failure")
	}
}
