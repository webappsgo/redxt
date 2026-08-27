package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadConfigMissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, "cli.yml"))
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Lang != "auto" || cfg.Defaults.Color != "auto" {
		t.Errorf("LoadConfig() default = %+v, want lang/color auto", cfg)
	}
}

func TestSaveThenLoadConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yml")

	cfg := DefaultConfig()
	cfg.Server.URL = "https://example.com"
	cfg.Auth.Token = "usr_api_abc123"

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if loaded.Server.URL != cfg.Server.URL || loaded.Auth.Token != cfg.Auth.Token {
		t.Errorf("LoadConfig() = %+v, want %+v", loaded, cfg)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yml")
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o600); err != nil {
		t.Fatalf("write test fixture: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig() expected error for invalid YAML")
	}
}

func TestConfigPathDefaultAndProfile(t *testing.T) {
	if got := ConfigPath(""); filepath.Base(got) != "cli.yml" {
		t.Errorf("ConfigPath(\"\") = %q, want basename cli.yml", got)
	}
	if got := ConfigPath("work.yml"); filepath.Base(got) != "work.yml" {
		t.Errorf("ConfigPath(profile) = %q, want basename work.yml", got)
	}
}

func TestResolveToken(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{Token: "from-config"}}

	tests := []struct {
		name      string
		flagToken string
		envToken  string
		want      string
	}{
		{"flag wins", "from-flag", "from-env", "from-flag"},
		{"env wins over config", "", "from-env", "from-env"},
		{"config is fallback", "", "", "from-config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveToken(tt.flagToken, tt.envToken, cfg); got != tt.want {
				t.Errorf("ResolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("usr_api_secret\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := ResolveTokenFile(path)
	if err != nil {
		t.Fatalf("ResolveTokenFile() error: %v", err)
	}
	if got != "usr_api_secret" {
		t.Errorf("ResolveTokenFile() = %q, want usr_api_secret (trailing newline stripped)", got)
	}

	empty, err := ResolveTokenFile("")
	if err != nil || empty != "" {
		t.Errorf("ResolveTokenFile(\"\") = (%q, %v), want (\"\", nil)", empty, err)
	}

	if _, err := ResolveTokenFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("ResolveTokenFile() expected error for missing file")
	}
}

func TestLoadConfigRejectsPermissiveMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yml")
	if err := os.WriteFile(path, []byte("lang: auto\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig() expected error for world-readable cli.yml")
	}
}

func TestResolveTokenFileRejectsPermissiveMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("usr_api_secret\n"), 0o666); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := ResolveTokenFile(path); err == nil {
		t.Fatal("ResolveTokenFile() expected error for world-readable token file")
	}
}
