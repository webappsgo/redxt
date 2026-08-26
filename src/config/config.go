// Package config loads and persists redxt's server.yml configuration
// per AI.md PART 5 (Configuration): file > environment variable >
// CLI flag priority for persisted settings, init-only environment
// variables consulted only on first run, and the random 64000-64999
// default port range.
package config

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/webappsgo/redxt/src/paths"
)

// Server holds the top-level server.yml document. Feature-specific
// sections (DNS zones, DNSSEC, clustering, etc.) are added by their
// owning PART implementations and must not be stubbed here.
type Server struct {
	// Listen is the bind address for the HTTP admin/API surface.
	Listen string `yaml:"listen"`
	// Port is the HTTP admin/API port. Persisted after first-run
	// random selection (see PART 5, Port Rules).
	Port int `yaml:"port"`
	// BaseURL is the path prefix when served behind a reverse proxy.
	BaseURL string `yaml:"baseurl"`
	// ApplicationName is the display title (init-only default source:
	// APPLICATION_NAME).
	ApplicationName string `yaml:"application_name"`
	// ApplicationTagline is the display description (init-only
	// default source: APPLICATION_TAGLINE).
	ApplicationTagline string `yaml:"application_tagline"`
}

// Config is the full in-memory configuration document.
type Config struct {
	Server Server `yaml:"server"`

	// path is the resolved location server.yml was loaded from / will
	// be saved to.
	path string
}

// defaultPortRangeLow and defaultPortRangeHigh bound the random
// first-run port selection (PART 5, Port Rules).
const (
	defaultPortRangeLow  = 64000
	defaultPortRangeHigh = 64999
)

// Load reads server.yml from the resolved config path, migrating a
// legacy server.yaml if present, and applying defaults (including
// first-run random port selection) for any unset persisted values.
// The returned Config is saved back to disk if defaults were applied.
func Load(p paths.Paths) (*Config, error) {
	cfg := &Config{path: p.ConfigFile}

	if err := os.MkdirAll(filepath.Dir(p.ConfigFile), 0o750); err != nil {
		return nil, fmt.Errorf("config: create config dir: %w", err)
	}

	migrateLegacyYAML(p.ConfigFile)

	data, err := os.ReadFile(p.ConfigFile)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", p.ConfigFile, err)
		}
	case os.IsNotExist(err):
		// First run: cfg stays zero-valued; defaults applied below.
	default:
		return nil, fmt.Errorf("config: read %s: %w", p.ConfigFile, err)
	}

	changed := cfg.applyDefaults()
	if changed {
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("config: save defaults: %w", err)
		}
	}

	return cfg, nil
}

// applyDefaults fills unset fields, performing first-run random port
// selection when Server.Port is zero. Returns true if any field was
// changed and must be persisted.
func (c *Config) applyDefaults() bool {
	changed := false

	if c.Server.Listen == "" {
		c.Server.Listen = "0.0.0.0"
		changed = true
	}
	if c.Server.Port == 0 {
		if v := os.Getenv("PORT"); v != "" {
			if n, err := parsePort(v); err == nil {
				c.Server.Port = n
				changed = true
			}
		}
	}
	if c.Server.Port == 0 {
		c.Server.Port = randomUnusedPort()
		changed = true
	}
	if c.Server.ApplicationName == "" {
		if v := os.Getenv("APPLICATION_NAME"); v != "" {
			c.Server.ApplicationName = v
		} else {
			c.Server.ApplicationName = "redxt"
		}
		changed = true
	}
	if c.Server.ApplicationTagline == "" {
		if v := os.Getenv("APPLICATION_TAGLINE"); v != "" {
			c.Server.ApplicationTagline = v
		} else {
			c.Server.ApplicationTagline = "The Complete DNS Server"
		}
		changed = true
	}

	return changed
}

func parsePort(v string) (int, error) {
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n < 0 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", v)
	}
	return n, nil
}

// randomUnusedPort selects a random unused port in the 64000-64999
// range, retrying until a free port is found.
func randomUnusedPort() int {
	for {
		port := defaultPortRangeLow + rand.Intn(defaultPortRangeHigh-defaultPortRangeLow+1)
		if isPortFree(port) {
			return port
		}
	}
}

func isPortFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// migrateLegacyYAML renames a legacy server.yaml to server.yml on
// startup, per PART 5's Configuration File > Migration rule.
func migrateLegacyYAML(ymlPath string) {
	legacy := ymlPath[:len(ymlPath)-len("yml")] + "yaml"
	if _, err := os.Stat(ymlPath); err == nil {
		return
	}
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	_ = os.Rename(legacy, ymlPath)
}

// Save writes the current configuration back to its resolved path.
func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o640); err != nil {
		return fmt.Errorf("config: write %s: %w", c.path, err)
	}
	return nil
}

// Path returns the resolved server.yml location this Config was
// loaded from.
func (c *Config) Path() string {
	return c.path
}
