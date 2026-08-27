package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/redxt/src/health"
)

func TestRunStatusHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health.Response{Status: health.StatusHealthy, Version: "1.0.0", Mode: "production"})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := RunStatus(NewHTTPClient(srv.URL, ""), &out, &errOut)
	if code != 0 {
		t.Fatalf("RunStatus() = %d, want 0 (stderr: %s)", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Status:   healthy")) {
		t.Errorf("RunStatus() output missing status line: %s", out.String())
	}
}

func TestRunStatusUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health.Response{Status: health.StatusUnhealthy})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := RunStatus(NewHTTPClient(srv.URL, ""), &out, &errOut)
	if code != 1 {
		t.Fatalf("RunStatus() = %d, want 1", code)
	}
}

func TestRunStatusUnreachable(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunStatus(NewHTTPClient("", ""), &out, &errOut)
	if code != 1 {
		t.Fatalf("RunStatus() = %d, want 1", code)
	}
	if errOut.Len() == 0 {
		t.Error("RunStatus() should write an error message on failure")
	}
}
