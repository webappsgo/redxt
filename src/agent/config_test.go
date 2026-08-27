package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadConfigMissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, "agent.yml"))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Lang != "auto" || cfg.Server.APIVersion != "v1" || cfg.Server.AdminPath != "administration" {
		t.Errorf("LoadConfig() default = %+v, want documented defaults", cfg)
	}
	if !cfg.Collection.Enabled || cfg.Collection.Interval != "60s" {
		t.Errorf("LoadConfig() default collection = %+v", cfg.Collection)
	}
	if !cfg.Health.Enabled || cfg.Health.Interval != "30s" {
		t.Errorf("LoadConfig() default health = %+v", cfg.Health)
	}
}

func TestSaveThenLoadConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yml")

	cfg := DefaultConfig()
	cfg.Server.Primary = "https://example.com"
	cfg.Auth.Token = "adm_agt_abc123"
	cfg.Identity.Hostname = "web-01"
	cfg.Identity.Tags = []string{"production", "web-tier"}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if loaded.Server.Primary != cfg.Server.Primary || loaded.Auth.Token != cfg.Auth.Token {
		t.Errorf("LoadConfig() = %+v, want %+v", loaded, cfg)
	}
	if loaded.Identity.Hostname != "web-01" || len(loaded.Identity.Tags) != 2 {
		t.Errorf("LoadConfig() identity = %+v", loaded.Identity)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yml")
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o600); err != nil {
		t.Fatalf("write test fixture: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig() expected error for invalid YAML")
	}
}

func TestConfigPathDefaultAndOverride(t *testing.T) {
	if got := ConfigPath(""); filepath.Base(got) != "agent.yml" {
		t.Errorf("ConfigPath(\"\") = %q, want basename agent.yml", got)
	}
	dir := t.TempDir()
	if got := ConfigPath(dir); got != filepath.Join(dir, "agent.yml") {
		t.Errorf("ConfigPath(dir) = %q, want %q", got, filepath.Join(dir, "agent.yml"))
	}
}

func TestResolveToken(t *testing.T) {
	tests := []struct {
		name      string
		flagToken string
		envToken  string
		cfg       *Config
		want      string
	}{
		{"flag wins", "from-flag", "from-env", &Config{Auth: AuthConfig{Token: "from-config"}}, "from-flag"},
		{"env wins over config", "", "from-env", &Config{Auth: AuthConfig{Token: "from-config"}}, "from-env"},
		{"config is fallback", "", "", &Config{Auth: AuthConfig{Token: "from-config"}}, "from-config"},
		{"none set", "", "", &Config{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveToken(tt.flagToken, tt.envToken, tt.cfg)
			if err != nil {
				t.Fatalf("ResolveToken() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("adm_agt_secret\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := &Config{Auth: AuthConfig{TokenFile: path}}
	got, err := ResolveToken("", "", cfg)
	if err != nil {
		t.Fatalf("ResolveToken() error: %v", err)
	}
	if got != "adm_agt_secret" {
		t.Errorf("ResolveToken() = %q, want adm_agt_secret (trailing newline stripped)", got)
	}
}

func TestResolveTokenFromMissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Auth: AuthConfig{TokenFile: filepath.Join(dir, "missing")}}
	if _, err := ResolveToken("", "", cfg); err == nil {
		t.Fatal("ResolveToken() expected error for missing token file")
	}
}

func TestLoadConfigRejectsPermissiveMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yml")
	if err := os.WriteFile(path, []byte("lang: auto\n"), 0o640); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig() expected error for group-readable agent.yml")
	}
}

func TestResolveTokenFromFileRejectsPermissiveMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("adm_agt_secret\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &Config{Auth: AuthConfig{TokenFile: path}}
	if _, err := ResolveToken("", "", cfg); err == nil {
		t.Fatal("ResolveToken() expected error for world-readable token file")
	}
}
