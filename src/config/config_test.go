package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/paths"
)

// testPaths returns a paths value rooted in a temporary directory so
// that Load never touches the real system configuration.
func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	dir := t.TempDir()
	return paths.Paths{
		Config:     dir,
		ConfigFile: filepath.Join(dir, "server.yml"),
	}
}

func TestLoadFirstRunWritesDefaults(t *testing.T) {
	p := testPaths(t)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.ApplicationName != DefaultApplicationName {
		t.Fatalf("application_name = %q, want %q", cfg.Server.ApplicationName, DefaultApplicationName)
	}
	info, err := os.Stat(p.ConfigFile)
	if err != nil {
		t.Fatalf("server.yml was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("server.yml mode = %o, want 640 because it holds the encryption key", perm)
	}
}

func TestLoadKeepsExistingValues(t *testing.T) {
	p := testPaths(t)
	body := "server:\n  listen: 127.0.0.1\n  port: 8053\n  baseurl: /redxt/\n"
	if err := os.WriteFile(p.ConfigFile, []byte(body), 0o640); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Listen != "127.0.0.1" {
		t.Fatalf("listen = %q, want 127.0.0.1", cfg.Server.Listen)
	}
	if cfg.Server.Port != 8053 {
		t.Fatalf("port = %d, want 8053", cfg.Server.Port)
	}
	if cfg.Server.BaseURL != "/redxt" {
		t.Fatalf("base_url = %q, want the normalized /redxt", cfg.Server.BaseURL)
	}
	if cfg.Server.Logs.Level != DefaultConfig().Server.Logs.Level {
		t.Fatalf("an unset key did not fall back to its default: %q", cfg.Server.Logs.Level)
	}
}

func TestLoadRepairsInvalidValuesWithoutFailing(t *testing.T) {
	p := testPaths(t)
	body := "server:\n  logs:\n    level: chatty\n  database:\n    type: oracle\n"
	if err := os.WriteFile(p.ConfigFile, []byte(body), 0o640); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load must never fail on an invalid setting: %v", err)
	}
	if len(cfg.Warnings()) < 2 {
		t.Fatalf("warnings = %v, want one per invalid setting", cfg.Warnings())
	}
	saved, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(saved), "chatty") || strings.Contains(string(saved), "oracle") {
		t.Fatalf("repaired values were not persisted:\n%s", saved)
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.ConfigFile, []byte("server:\n  listen: [unterminated\n"), 0o640); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("Load accepted a syntactically broken config file")
	}
}

func TestLoadMigratesLegacyYAML(t *testing.T) {
	p := testPaths(t)
	legacy := filepath.Join(p.Config, "server.yaml")
	if err := os.WriteFile(legacy, []byte("server:\n  port: 9153\n"), 0o640); err != nil {
		t.Fatalf("seed legacy config: %v", err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9153 {
		t.Fatalf("port = %d, want the migrated 9153", cfg.Server.Port)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy server.yaml was left in place after migration")
	}
	if _, err := os.Stat(p.ConfigFile); err != nil {
		t.Fatalf("server.yml missing after migration: %v", err)
	}
}

func TestLoadPrefersExistingYmlOverLegacyYaml(t *testing.T) {
	p := testPaths(t)
	if err := os.WriteFile(p.ConfigFile, []byte("server:\n  port: 111\n"), 0o640); err != nil {
		t.Fatalf("seed yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(p.Config, "server.yaml"), []byte("server:\n  port: 222\n"), 0o640); err != nil {
		t.Fatalf("seed yaml: %v", err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 111 {
		t.Fatalf("port = %d, want 111 from the existing server.yml", cfg.Server.Port)
	}
}

func TestApplyInitEnvOnlySeedsUnsetValues(t *testing.T) {
	t.Setenv("PORT", "5353")
	t.Setenv("APPLICATION_NAME", "seeded")

	fresh := DefaultConfig()
	fresh.applyInitEnv()
	if fresh.Server.Port != 5353 {
		t.Fatalf("port = %d, want the seeded 5353", fresh.Server.Port)
	}
	if fresh.Server.ApplicationName != "seeded" {
		t.Fatalf("application_name = %q, want seeded", fresh.Server.ApplicationName)
	}

	existing := DefaultConfig()
	existing.Server.Port = 8053
	existing.Server.ApplicationName = "already-set"
	existing.applyInitEnv()
	if existing.Server.Port != 8053 {
		t.Fatalf("port = %d, want the configured 8053 to win over PORT", existing.Server.Port)
	}
	if existing.Server.ApplicationName != "already-set" {
		t.Fatalf("application_name = %q, want the configured value to win", existing.Server.ApplicationName)
	}
}

func TestApplyInitEnvWarnsOnBadPort(t *testing.T) {
	t.Setenv("PORT", "not-a-port")
	c := DefaultConfig()
	c.applyInitEnv()
	if c.Server.Port != 0 {
		t.Fatalf("port = %d, want it left unset", c.Server.Port)
	}
	if len(c.Warnings()) == 0 {
		t.Fatal("an unparseable PORT produced no warning")
	}
}

func TestApplyRuntimeEnvOverridesFile(t *testing.T) {
	t.Setenv("DATABASE_DRIVER", "mariadb")
	t.Setenv("DATABASE_URL", "mysql://db.example.com/redxt")
	c := DefaultConfig()
	c.Server.Database.Type = "sqlite"
	c.ApplyRuntimeEnv()
	if c.Server.Database.Type != "mysql" {
		t.Fatalf("driver = %q, want the canonicalized mysql", c.Server.Database.Type)
	}
	if c.Server.Database.URL != "mysql://db.example.com/redxt" {
		t.Fatalf("url = %q, want the environment value", c.Server.Database.URL)
	}
}

func TestDomain(t *testing.T) {
	t.Setenv("DOMAIN", "  dns.example.com  ")
	if got := Domain(); got != "dns.example.com" {
		t.Fatalf("Domain() = %q, want the trimmed value", got)
	}
}

func TestSaveDoesNotLeakOutsideItsPath(t *testing.T) {
	dir := t.TempDir()
	c := DefaultConfig()
	c.SetPath(filepath.Join(dir, "server.yml"))
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := c.Path(); got != filepath.Join(dir, "server.yml") {
		t.Fatalf("Path() = %q, want the path set for this config", got)
	}
}
