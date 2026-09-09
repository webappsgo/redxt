package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/webappsgo/redxt/src/apierror"
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

	client := NewHTTPClient(srv.URL, "usr_api_secret")
	var out map[string]any
	if _, err := client.Get("/ping", &out); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if gotUA != UserAgent() {
		t.Errorf("User-Agent = %q, want %q", gotUA, UserAgent())
	}
	if gotAuth != "Bearer usr_api_secret" {
		t.Errorf("Authorization = %q, want Bearer usr_api_secret", gotAuth)
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

func TestHTTPClientPostJSON(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "")
	var out map[string]any
	if _, err := client.PostJSON("/register", map[string]string{"hostname": "web-01"}, &out); err != nil {
		t.Fatalf("PostJSON() error: %v", err)
	}
	if gotBody["hostname"] != "web-01" {
		t.Errorf("server received body %v, want hostname web-01", gotBody)
	}
}

func TestHTTPClientTokenRevokedClearsCachedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apierror.Response{
			OK:    false,
			Error: apierror.CodeTokenRevoked,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yml")
	cfg := DefaultConfig()
	cfg.Auth.Token = "usr_api_dead"
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	client := NewHTTPClient(srv.URL, "usr_api_dead")
	client.ConfigPath = path

	var out map[string]any
	_, err := client.Get("/health", &out)
	if !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("Get() error = %v, want ErrTokenRevoked", err)
	}

	loaded, loadErr := LoadConfig(path)
	if loadErr != nil {
		t.Fatalf("LoadConfig() error: %v", loadErr)
	}
	if loaded.Auth.Token != "" {
		t.Errorf("cached token = %q after TOKEN_REVOKED, want cleared", loaded.Auth.Token)
	}
}

func TestHTTPClientTokenRevokedWithoutConfigPathStillReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apierror.Response{
			OK:    false,
			Error: apierror.CodeTokenRevoked,
		})
	}))
	defer srv.Close()

	client := NewHTTPClient(srv.URL, "usr_api_dead")

	var out map[string]any
	_, err := client.Get("/health", &out)
	if !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("Get() error = %v, want ErrTokenRevoked", err)
	}
}

// TestHTTPClientTokenExpiredClearsCachedToken verifies AI.md's "Same
// behavior on 401 TOKEN_EXPIRED" instruction in "CLI Token Revocation
// Handling": TOKEN_EXPIRED clears the cached token exactly like
// TOKEN_REVOKED does.
func TestHTTPClientTokenExpiredClearsCachedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apierror.Response{
			OK:    false,
			Error: apierror.CodeTokenExpired,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yml")
	cfg := DefaultConfig()
	cfg.Auth.Token = "usr_api_stale"
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	client := NewHTTPClient(srv.URL, "usr_api_stale")
	client.ConfigPath = path

	var out map[string]any
	_, err := client.Get("/health", &out)
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("Get() error = %v, want ErrTokenExpired", err)
	}

	loaded, loadErr := LoadConfig(path)
	if loadErr != nil {
		t.Fatalf("LoadConfig() error: %v", loadErr)
	}
	if loaded.Auth.Token != "" {
		t.Errorf("cached token = %q after TOKEN_EXPIRED, want cleared", loaded.Auth.Token)
	}
}

func TestHTTPClientOtherUnauthorizedVariantsAreUnaffected(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "unauthorized", code: apierror.CodeUnauthorized},
		{name: "token invalid", code: apierror.CodeTokenInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(apierror.Response{OK: false, Error: tt.code})
			}))
			defer srv.Close()

			dir := t.TempDir()
			path := filepath.Join(dir, "cli.yml")
			cfg := DefaultConfig()
			cfg.Auth.Token = "usr_api_still_here"
			if err := SaveConfig(path, cfg); err != nil {
				t.Fatalf("SaveConfig() error: %v", err)
			}

			client := NewHTTPClient(srv.URL, "usr_api_still_here")
			client.ConfigPath = path

			var out apierror.Response
			resp, err := client.Get("/health", &out)
			if errors.Is(err, ErrTokenRevoked) {
				t.Fatalf("Get() returned ErrTokenRevoked for code %s, want unaffected", tt.code)
			}
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
			if out.Error != tt.code {
				t.Fatalf("decoded error = %q, want %q", out.Error, tt.code)
			}

			loaded, loadErr := LoadConfig(path)
			if loadErr != nil {
				t.Fatalf("LoadConfig() error: %v", loadErr)
			}
			if loaded.Auth.Token != "usr_api_still_here" {
				t.Errorf("cached token = %q, want unchanged for code %s", loaded.Auth.Token, tt.code)
			}
		})
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
