package main

import (
	"os"
	"path/filepath"

	"github.com/webappsgo/redxt/src/common/credfile"
	"github.com/webappsgo/redxt/src/paths"
	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape of cli.yml, AI.md PART 33 "cli.yml
// Configuration". Only the fields this pass implements are modeled;
// unimplemented sections (per-command defaults beyond color/lang) are
// intentionally absent rather than stubbed, per TODO.AI.md.
type Config struct {
	// Lang is the default --lang value: "auto" or a language code.
	Lang string `yaml:"lang"`

	Server ServerConfig `yaml:"server"`
	Auth   AuthConfig   `yaml:"auth"`

	Defaults DefaultsConfig `yaml:"defaults"`
}

// ServerConfig holds the redxt-cli server connection settings.
type ServerConfig struct {
	// URL is the server this CLI talks to.
	URL string `yaml:"url"`
}

// AuthConfig holds the redxt-cli authentication settings.
type AuthConfig struct {
	// Token is the API token, stored only when the user did not
	// already provide one via --token or the environment.
	Token string `yaml:"token"`
}

// DefaultsConfig holds default flag values sourced from the config
// file, per AI.md PART 33 "Flag Defaults from Config".
type DefaultsConfig struct {
	// Color is the default --color value.
	Color string `yaml:"color"`
}

// DefaultConfig returns the sane, zero-config defaults cli.yml is
// created with on first run.
func DefaultConfig() *Config {
	return &Config{
		Lang:     "auto",
		Defaults: DefaultsConfig{Color: "auto"},
	}
}

// ConfigPath returns the path to cli.yml, honoring the --config
// profile-name flag when set.
func ConfigPath(profile string) string {
	name := "cli.yml"
	if profile != "" {
		name = profile
	}
	return filepath.Join(paths.Resolve().Config, name)
}

// LoadConfig reads cli.yml from path, returning DefaultConfig() when the
// file does not exist yet.
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

// ResolveToken applies the AI.md PART 33 "Authentication" priority:
// --token flag, then {PROJECT_NAME}_TOKEN environment variable, then
// auth.token in cli.yml.
func ResolveToken(flagToken, envToken string, cfg *Config) string {
	if flagToken != "" {
		return flagToken
	}
	if envToken != "" {
		return envToken
	}
	return cfg.Auth.Token
}

// ResolveTokenFile reads a token from a file when tokenFile is set,
// trimming trailing whitespace/newlines.
func ResolveTokenFile(tokenFile string) (string, error) {
	if tokenFile == "" {
		return "", nil
	}
	if err := credfile.CheckPerms(tokenFile); err != nil {
		return "", err
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}
	return trimNewline(string(data)), nil
}

// trimNewline strips a single trailing newline sequence, the common
// shape of a token written by "echo TOKEN > file".
func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// DeleteCachedToken clears auth.token in cli.yml at path, per AI.md PART 33
// "CLI Token Revocation Handling": on 401 TOKEN_REVOKED the cached token
// MUST be dropped so the next invocation prompts for fresh credentials
// instead of replaying the dead token. It is a no-op if the file does not
// exist or already carries no token.
func DeleteCachedToken(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return err
	}
	if cfg.Auth.Token == "" {
		return nil
	}
	cfg.Auth.Token = ""
	return SaveConfig(path, cfg)
}
