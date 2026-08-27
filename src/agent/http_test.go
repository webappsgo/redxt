package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientGetSendsIdentityHeaders(t *testing.T) {
	var gotUA, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "adm_agt_secret")
	var out map[string]any
	if _, err := client.Get("/ping", &out); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if gotUA != UserAgent() {
		t.Errorf("User-Agent = %q, want %q", gotUA, UserAgent())
	}
	if gotAuth != "Bearer adm_agt_secret" {
		t.Errorf("Authorization = %q, want Bearer adm_agt_secret", gotAuth)
	}
	if out["ok"] != true {
		t.Errorf("decoded body = %v, want ok:true", out)
	}
}

func TestHTTPClientNoServerConfigured(t *testing.T) {
	client := NewHTTPClient("", "")
	if _, err := client.Get("/ping", nil); err == nil {
		t.Fatal("Get() expected error when no server is configured")
	}
}

func TestHTTPClientTrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL+"/", "")
	var out map[string]any
	if _, err := client.Get("/ping", &out); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if gotPath != "/ping" {
		t.Errorf("request path = %q, want /ping (no double slash)", gotPath)
	}
}
