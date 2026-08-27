package main

import (
	"os"
	"path/filepath"

	"github.com/webappsgo/redxt/src/common/credfile"
	"github.com/webappsgo/redxt/src/paths"
	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape of agent.yml, AI.md PART 33 "agent.yml
// Configuration". The full documented schema is modeled here (it is a
// pure data contract with sane defaults, so nothing about it is a
// stub); the collection loop and health reporting that would consume
// Collection/Health at runtime are the deferred items tracked in
// TODO.AI.md, not this struct.
type Config struct {
	// Lang is the default --lang value: "auto" or a language code.
	Lang string `yaml:"lang"`

	Server     ServerConfig     `yaml:"server"`
	Auth       AuthConfig       `yaml:"auth"`
	Identity   IdentityConfig   `yaml:"identity"`
	Collection CollectionConfig `yaml:"collection"`
	Logging    LoggingConfig    `yaml:"logging"`
	Health     HealthConfig     `yaml:"health"`

	// Debug enables debug mode, same as --debug.
	Debug bool `yaml:"debug"`
	// Mode overrides auto-detection: production, development, debug.
	// Empty means auto-detect.
	Mode string `yaml:"mode"`
}

// ServerConfig holds the redxt-agent server connection settings.
type ServerConfig struct {
	// Primary is the server URL, set during registration.
	Primary string `yaml:"primary"`
	// Cluster is the auto-discovered cluster node list.
	Cluster []string `yaml:"cluster"`
	// APIVersion is the API version prefix; must match the server.
	APIVersion string `yaml:"api_version"`
	// AdminPath must match the server's configured admin path.
	AdminPath string `yaml:"admin_path"`
	// Timeout is the per-request timeout, e.g. "30s".
	Timeout string `yaml:"timeout"`
	// Retry is the number of retry attempts on failure.
	Retry int `yaml:"retry"`
	// RetryDelay is the delay between retries, e.g. "5s".
	RetryDelay string `yaml:"retry_delay"`
	// ReconnectDelay is the delay before a reconnect attempt, e.g. "10s".
	ReconnectDelay string `yaml:"reconnect_delay"`
}

// AuthConfig holds the redxt-agent authentication settings.
type AuthConfig struct {
	// Token is the agent token ({scope}_agt_xxx).
	Token string `yaml:"token"`
	// TokenFile reads the token from a file instead.
	TokenFile string `yaml:"token_file"`
}

// IdentityConfig holds how this agent identifies itself to the server.
type IdentityConfig struct {
	// Hostname is auto-detected when empty.
	Hostname string `yaml:"hostname"`
	// DisplayName defaults to Hostname when empty.
	DisplayName string `yaml:"display_name"`
	// Tags groups agents, e.g. ["production", "web-tier"].
	Tags []string `yaml:"tags"`
	// Labels are free-form key-value metadata.
	Labels map[string]string `yaml:"labels"`
}

// CollectionConfig controls the (not yet implemented) data collection
// loop's parameters.
type CollectionConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Interval   string `yaml:"interval"`
	BatchSize  int    `yaml:"batch_size"`
	BufferSize int    `yaml:"buffer_size"`
}

// LoggingConfig controls agent log output.
type LoggingConfig struct {
	Level    string `yaml:"level"`
	File     string `yaml:"file"`
	MaxSize  string `yaml:"max_size"`
	MaxFiles int    `yaml:"max_files"`
}

// HealthConfig controls the (not yet implemented) periodic health
// report to the server.
type HealthConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Interval string `yaml:"interval"`
}

// DefaultConfig returns the sane, zero-config defaults agent.yml is
// created with on first run, matching AI.md PART 33's documented
// defaults exactly.
func DefaultConfig() *Config {
	return &Config{
		Lang: "auto",
		Server: ServerConfig{
			APIVersion:     "v1",
			AdminPath:      "administration",
			Timeout:        "30s",
			Retry:          3,
			RetryDelay:     "5s",
			ReconnectDelay: "10s",
		},
		Collection: CollectionConfig{
			Enabled:    true,
			Interval:   "60s",
			BatchSize:  100,
			BufferSize: 1000,
		},
		Logging: LoggingConfig{
			Level:    "info",
			MaxSize:  "10MB",
			MaxFiles: 5,
		},
		Health: HealthConfig{
			Enabled:  true,
			Interval: "30s",
		},
	}
}

// ConfigPath returns the path to agent.yml, honoring the --config
// directory override when set (unlike redxt-cli's --config, this
// names a directory, not a profile file).
func ConfigPath(configDir string) string {
	dir := configDir
	if dir == "" {
		dir = paths.Resolve().Config
	}
	return filepath.Join(dir, "agent.yml")
}

// LoadConfig reads agent.yml from path, returning DefaultConfig() when
// the file does not exist yet.
func LoadConfig(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err := credfile.CheckPerms(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveConfig writes cfg to path as YAML, creating parent directories as
// needed.
func SaveConfig(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ResolveToken applies the AI.md PART 33 "Config precedence" priority
// for the agent token: --token flag, then {PROJECT_NAME}_AGENT_TOKEN
// environment variable, then auth.token in agent.yml, then
// auth.token_file in agent.yml.
func ResolveToken(flagToken, envToken string, cfg *Config) (string, error) {
	if flagToken != "" {
		return flagToken, nil
	}
	if envToken != "" {
		return envToken, nil
	}
	if cfg.Auth.Token != "" {
		return cfg.Auth.Token, nil
	}
	if cfg.Auth.TokenFile != "" {
		if err := credfile.CheckPerms(cfg.Auth.TokenFile); err != nil {
			return "", err
		}
		data, err := os.ReadFile(cfg.Auth.TokenFile)
		if err != nil {
			return "", err
		}
		return trimNewline(string(data)), nil
	}
	return "", nil
}

// trimNewline strips a single trailing newline sequence, the common
// shape of a token written by "echo TOKEN > file".
func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
