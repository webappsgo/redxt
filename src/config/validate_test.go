package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "/"},
		{name: "root", in: "/", want: "/"},
		{name: "leading slash added", in: "redxt", want: "/redxt"},
		{name: "trailing slash removed", in: "/redxt/", want: "/redxt"},
		{name: "nested", in: "/a/b/", want: "/a/b"},
		{name: "space trimmed", in: "  /redxt  ", want: "/redxt"},
		{name: "only slashes", in: "///", want: "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeBaseURL(tt.in); got != tt.want {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidRotate(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"daily", true},
		{"weekly", true},
		{"monthly", true},
		{"yearly", true},
		{"never", true},
		{"50MB", true},
		{"weekly,50MB", true},
		{"WEEKLY,50mb", true},
		{"", false},
		{"hourly", false},
		{"weekly,", false},
		{"weekly,huge", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ValidRotate(tt.in); got != tt.want {
				t.Fatalf("ValidRotate(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidKeep(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"none", true},
		{"forever", true},
		{"5", true},
		{"30d", true},
		{"4w", true},
		{"6m", true},
		{"", false},
		{"d", false},
		{"5y", false},
		{"many", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ValidKeep(tt.in); got != tt.want {
				t.Fatalf("ValidKeep(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLogRotationInterval(t *testing.T) {
	tests := []struct {
		in    string
		want  time.Duration
		found bool
	}{
		{"daily", 24 * time.Hour, true},
		{"weekly,50MB", 7 * 24 * time.Hour, true},
		{"monthly", 30 * 24 * time.Hour, true},
		{"yearly", 365 * 24 * time.Hour, true},
		{"50MB", 0, false},
		{"never", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := LogRotationInterval(tt.in)
			if ok != tt.found || got != tt.want {
				t.Fatalf("LogRotationInterval(%q) = %v,%v want %v,%v", tt.in, got, ok, tt.want, tt.found)
			}
		})
	}
}

func TestLogRotationSize(t *testing.T) {
	tests := []struct {
		in    string
		want  int64
		found bool
	}{
		{"50MB", 50 * 1024 * 1024, true},
		{"weekly,50MB", 50 * 1024 * 1024, true},
		{"weekly", 0, false},
		{"never", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := LogRotationSize(tt.in)
			if ok != tt.found || got != tt.want {
				t.Fatalf("LogRotationSize(%q) = %v,%v want %v,%v", tt.in, got, ok, tt.want, tt.found)
			}
		})
	}
}

func TestNormalizeDatabaseDriver(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"sqlite", "sqlite"},
		{"SQLite3", "sqlite"},
		{"sqlite2", "sqlite"},
		{"turso", "libsql"},
		{"libsql", "libsql"},
		{"PostgreSQL", "postgres"},
		{"pgsql", "postgres"},
		{"mariadb", "mysql"},
		{"MySQL", "mysql"},
		{"mssql", "mssql"},
		{"mongo", "mongodb"},
		{" oracle ", "oracle"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizeDatabaseDriver(tt.in); got != tt.want {
				t.Fatalf("NormalizeDatabaseDriver(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSessionSecure(t *testing.T) {
	tests := []struct {
		setting string
		overTLS bool
		want    bool
	}{
		{"true", false, true},
		{"false", true, false},
		{"auto", true, true},
		{"auto", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.setting, func(t *testing.T) {
			c := DefaultConfig()
			c.Server.Session.Secure = tt.setting
			if got := c.SessionSecure(tt.overTLS); got != tt.want {
				t.Fatalf("SessionSecure(%v) with %q = %v, want %v", tt.overTLS, tt.setting, got, tt.want)
			}
		})
	}
}

func TestEnsureEncryptionKey(t *testing.T) {
	c := DefaultConfig()
	created, err := EnsureEncryptionKey(c, func() (string, error) { return "generated-key", nil })
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}
	if !created {
		t.Fatal("first call did not report the key as created")
	}
	if c.Server.Security.EncryptionKey != "generated-key" {
		t.Fatalf("key = %q, want the generated value", c.Server.Security.EncryptionKey)
	}

	created, err = EnsureEncryptionKey(c, func() (string, error) { return "second-key", nil })
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}
	if created {
		t.Fatal("an existing key was regenerated")
	}
	if c.Server.Security.EncryptionKey != "generated-key" {
		t.Fatal("an existing key was overwritten")
	}
}

func TestEnsureEncryptionKeyPropagatesGeneratorError(t *testing.T) {
	sentinel := errors.New("no entropy")
	c := DefaultConfig()
	if _, err := EnsureEncryptionKey(c, func() (string, error) { return "", sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to wrap %v", err, sentinel)
	}
	if c.Server.Security.EncryptionKey != "" {
		t.Fatal("a key was stored despite the generator failing")
	}
}

// TestDefaultConfigValidatesClean checks that nothing in the shipped
// defaults is itself rejected by validation. The one expected warning
// is the first-run port selection, which reports the random 64xxx port
// chosen because no port has been configured yet.
func TestDefaultConfigValidatesClean(t *testing.T) {
	c := DefaultConfig()
	c.Validate()
	for _, w := range c.Warnings() {
		if strings.Contains(w, "server.port") {
			continue
		}
		t.Fatalf("defaults produced an unexpected warning: %s", w)
	}
	if c.Server.Port <= 0 {
		t.Fatal("validation left the port unset")
	}
}

func TestValidateWarnsAndReplaces(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		check  func(*testing.T, *Config)
	}{
		{
			name:   "unknown log level",
			mutate: func(c *Config) { c.Server.Logs.Level = "chatty" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Logs.Level != DefaultConfig().Server.Logs.Level {
					t.Fatalf("level = %q, want the default", c.Server.Logs.Level)
				}
			},
		},
		{
			name:   "unknown access log format",
			mutate: func(c *Config) { c.Server.Logs.Access.Format = "w3c" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Logs.Access.Format != DefaultConfig().Server.Logs.Access.Format {
					t.Fatalf("format = %q, want the default", c.Server.Logs.Access.Format)
				}
			},
		},
		{
			name:   "audit format is forced to json",
			mutate: func(c *Config) { c.Server.Logs.Audit.Format = "text" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Logs.Audit.Format != "json" {
					t.Fatalf("audit format = %q, want json", c.Server.Logs.Audit.Format)
				}
			},
		},
		{
			name:   "log filename may not carry a path",
			mutate: func(c *Config) { c.Server.Logs.Server.Filename = "../../etc/passwd" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Logs.Server.Filename != DefaultConfig().Server.Logs.Server.Filename {
					t.Fatalf("filename = %q, want the default", c.Server.Logs.Server.Filename)
				}
			},
		},
		{
			name:   "invalid rotation policy",
			mutate: func(c *Config) { c.Server.Logs.Server.Rotate = "hourly" },
			check: func(t *testing.T, c *Config) {
				if !ValidRotate(c.Server.Logs.Server.Rotate) {
					t.Fatalf("rotate = %q, want a valid policy", c.Server.Logs.Server.Rotate)
				}
			},
		},
		{
			name:   "invalid retention policy",
			mutate: func(c *Config) { c.Server.Logs.Server.Keep = "always" },
			check: func(t *testing.T, c *Config) {
				if !ValidKeep(c.Server.Logs.Server.Keep) {
					t.Fatalf("keep = %q, want a valid policy", c.Server.Logs.Server.Keep)
				}
			},
		},
		{
			name:   "unknown database driver",
			mutate: func(c *Config) { c.Server.Database.Type = "oracle" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Database.Type != "sqlite" {
					t.Fatalf("driver = %q, want sqlite", c.Server.Database.Type)
				}
			},
		},
		{
			name:   "idle connections are clamped to open connections",
			mutate: func(c *Config) { c.Server.Database.MaxOpenConns = 4; c.Server.Database.MaxIdleConns = 40 },
			check: func(t *testing.T, c *Config) {
				if c.Server.Database.MaxIdleConns > c.Server.Database.MaxOpenConns {
					t.Fatalf("max_idle_conns %d exceeds max_open_conns %d", c.Server.Database.MaxIdleConns, c.Server.Database.MaxOpenConns)
				}
			},
		},
		{
			name:   "unknown cache type",
			mutate: func(c *Config) { c.Server.Cache.Type = "memcached" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Cache.Type != DefaultConfig().Server.Cache.Type {
					t.Fatalf("cache type = %q, want the default", c.Server.Cache.Type)
				}
			},
		},
		{
			name:   "min idle is clamped to pool size",
			mutate: func(c *Config) { c.Server.Cache.PoolSize = 4; c.Server.Cache.MinIdle = 40 },
			check: func(t *testing.T, c *Config) {
				if c.Server.Cache.MinIdle > c.Server.Cache.PoolSize {
					t.Fatalf("min_idle %d exceeds pool_size %d", c.Server.Cache.MinIdle, c.Server.Cache.PoolSize)
				}
			},
		},
		{
			name:   "default language is added to the supported list",
			mutate: func(c *Config) { c.Server.I18n.DefaultLanguage = "de"; c.Server.I18n.Supported = []string{"en", "fr"} },
			check: func(t *testing.T, c *Config) {
				if !contains(c.Server.I18n.Supported, "de") {
					t.Fatalf("supported = %v, want it to include the default language", c.Server.I18n.Supported)
				}
			},
		},
		{
			name:   "session idle timeout is clamped to max age",
			mutate: func(c *Config) { c.Server.Session.Admin.IdleTimeout = Duration(365 * 24 * time.Hour) },
			check: func(t *testing.T, c *Config) {
				if c.Server.Session.Admin.IdleTimeout > c.Server.Session.Admin.MaxAge {
					t.Fatalf("idle_timeout %v exceeds max_age %v", c.Server.Session.Admin.IdleTimeout, c.Server.Session.Admin.MaxAge)
				}
			},
		},
		{
			name:   "non positive read timeout",
			mutate: func(c *Config) { c.Server.Limits.ReadTimeout = 0 },
			check: func(t *testing.T, c *Config) {
				if c.Server.Limits.ReadTimeout <= 0 {
					t.Fatal("read_timeout was left non positive")
				}
			},
		},
		{
			name:   "compression level out of range",
			mutate: func(c *Config) { c.Server.Compression.Level = 42 },
			check: func(t *testing.T, c *Config) {
				if c.Server.Compression.Level < 1 || c.Server.Compression.Level > 9 {
					t.Fatalf("level = %d, want 1 to 9", c.Server.Compression.Level)
				}
			},
		},
		{
			name:   "rate limit bucket without a window",
			mutate: func(c *Config) { c.Server.RateLimit.Read.Window = 0 },
			check: func(t *testing.T, c *Config) {
				if c.Server.RateLimit.Read.Window <= 0 {
					t.Fatal("read rate limit window was left non positive")
				}
			},
		},
		{
			name:   "invalid same site value",
			mutate: func(c *Config) { c.Server.Session.SameSite = "sideways" },
			check: func(t *testing.T, c *Config) {
				if c.Server.Session.SameSite != DefaultConfig().Server.Session.SameSite {
					t.Fatalf("same_site = %q, want the default", c.Server.Session.SameSite)
				}
			},
		},
		{
			name:   "unparseable trusted proxy is dropped",
			mutate: func(c *Config) { c.Server.TrustedProxies.Additional = []string{"10.0.0.0/8", "not a proxy!!"} },
			check: func(t *testing.T, c *Config) {
				if len(c.Server.TrustedProxies.Additional) != 1 || c.Server.TrustedProxies.Additional[0] != "10.0.0.0/8" {
					t.Fatalf("trusted proxies = %v, want only the valid entry", c.Server.TrustedProxies.Additional)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			// A configured port keeps the first-run port-selection
			// warning out of the way, so the only warning that can
			// appear is the one this case is asserting on.
			c.Server.Port = 8053
			tt.mutate(c)
			c.Validate()
			if len(c.Warnings()) == 0 {
				t.Fatal("invalid setting produced no warning")
			}
			tt.check(t, c)
		})
	}
}

// TestValidateCanonicalizesDriverAlias checks that an accepted alias
// spelling is rewritten silently. Only a genuinely unusable value is
// worth warning about.
func TestValidateCanonicalizesDriverAlias(t *testing.T) {
	c := DefaultConfig()
	c.Server.Port = 8053
	c.Server.Database.Type = "mariadb"
	c.Validate()
	if c.Server.Database.Type != "mysql" {
		t.Fatalf("driver = %q, want mysql", c.Server.Database.Type)
	}
	if len(c.Warnings()) != 0 {
		t.Fatalf("alias canonicalization warned: %v", c.Warnings())
	}
}

func TestValidateNeverFailsStartup(t *testing.T) {
	c := &Config{}
	c.Server.Logs.Level = "\x00"
	c.Server.Database.Type = "!!"
	c.Server.Cache.Type = "??"
	c.Server.Session.SameSite = "??"
	c.Validate()
	def := DefaultConfig()
	if c.Server.Logs.Level != def.Server.Logs.Level {
		t.Fatalf("level = %q, want the default", c.Server.Logs.Level)
	}
	if c.Server.Database.Type != def.Server.Database.Type {
		t.Fatalf("driver = %q, want the default", c.Server.Database.Type)
	}
	if c.Server.Cache.Type != def.Server.Cache.Type {
		t.Fatalf("cache type = %q, want the default", c.Server.Cache.Type)
	}
	if c.Server.Limits.ReadTimeout <= 0 {
		t.Fatal("zero-value limits were not repaired")
	}
}

// TestRandomUnusedPortIsUsable checks the documented contract rather
// than a fixed range: the 64xxx range is preferred, but a fully
// occupied range legitimately falls back to a kernel-assigned port so
// that startup still succeeds.
func TestRandomUnusedPortIsUsable(t *testing.T) {
	for i := 0; i < 5; i++ {
		port := RandomUnusedPort()
		if port < 1 || port > 65535 {
			t.Fatalf("RandomUnusedPort() = %d, want a usable TCP port", port)
		}
	}
}
