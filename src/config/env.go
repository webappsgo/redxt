package config

import (
	"os"
	"strconv"
	"strings"
)

// InitOnlyVars lists the environment variables from AI.md PART 5
// "Init-Only Variables (First Run Only)" that this package consumes.
// The directory variables in the same table are consumed by the paths
// package, which resolves the layout before the config file is read.
var InitOnlyVars = []string{
	"PORT",
	"LISTEN",
	"APPLICATION_NAME",
	"APPLICATION_TAGLINE",
}

// RuntimeVars lists the environment variables from AI.md PART 5
// "Runtime Variables (Always Checked)" that this package consumes.
// NO_COLOR, TERM, and MODE are consumed by the color, display, and
// mode packages respectively.
var RuntimeVars = []string{
	"DOMAIN",
	"DATABASE_DRIVER",
	"DATABASE_URL",
	"SMTP_HOST",
	"SMTP_PORT",
	"SMTP_USERNAME",
	"SMTP_PASSWORD",
	"SMTP_TLS",
	"SMTP_FROM_NAME",
	"SMTP_FROM_EMAIL",
}

// applyInitEnv seeds unset values from the init-only environment
// variables. These are read once, when the setting still holds its
// zero or default value, and are ignored on every later start because
// the resolved value has been persisted to server.yml by then.
func (c *Config) applyInitEnv() {
	if c.Server.Port == 0 {
		if v := os.Getenv("PORT"); v != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 && n <= 65535 {
				c.Server.Port = n
			} else {
				c.warnf("invalid PORT %q, ignoring it", v)
			}
		}
	}
	if v := os.Getenv("LISTEN"); v != "" && c.Server.Listen == DefaultConfig().Server.Listen {
		c.Server.Listen = strings.TrimSpace(v)
	}
	if v := os.Getenv("APPLICATION_NAME"); v != "" && c.Server.ApplicationName == DefaultApplicationName {
		c.Server.ApplicationName = v
	}
	if v := os.Getenv("APPLICATION_TAGLINE"); v != "" && c.Server.ApplicationTagline == DefaultApplicationTagline {
		c.Server.ApplicationTagline = v
	}
}

// ApplyRuntimeEnv overlays the always-checked runtime environment
// variables. They outrank the config file but are outranked by CLI
// flags, matching the AI.md PART 12 precedence chain of CLI flags,
// then environment, then database, then file, then defaults. Unlike
// the init-only variables these are re-read on every start and are
// never persisted back to server.yml.
func (c *Config) ApplyRuntimeEnv() {
	if v := os.Getenv("DATABASE_DRIVER"); v != "" {
		c.Server.Database.Type = NormalizeDatabaseDriver(v)
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Server.Database.URL = v
	}

	smtp := &c.Server.Notifications.Email.SMTP
	if v := strings.TrimSpace(os.Getenv("SMTP_HOST")); v != "" {
		smtp.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("SMTP_PORT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 65535 {
			smtp.Port = n
		} else {
			c.warnf("invalid SMTP_PORT %q, ignoring it", v)
		}
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		smtp.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		smtp.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("SMTP_TLS")); v != "" {
		if isSMTPTLSMode(v) {
			smtp.TLS = v
		} else {
			c.warnf("invalid SMTP_TLS %q, ignoring it", v)
		}
	}
	from := &c.Server.Notifications.Email.From
	if v := os.Getenv("SMTP_FROM_NAME"); v != "" {
		from.Name = v
	}
	if v := os.Getenv("SMTP_FROM_EMAIL"); v != "" {
		from.Email = v
	}
}

// isSMTPTLSMode reports whether v is one of config.SMTPTLSModes.
func isSMTPTLSMode(v string) bool {
	for _, m := range SMTPTLSModes {
		if strings.EqualFold(v, m) {
			return true
		}
	}
	return false
}

// Domain returns the DOMAIN environment override, which is the
// highest-priority source for the server's own FQDN after
// reverse-proxy headers.
func Domain() string {
	return strings.TrimSpace(os.Getenv("DOMAIN"))
}
