package config

import "strings"

// This file is the single source of truth for the value sets the
// configuration accepts. AI.md PART 14's /api/autodiscover endpoint
// publishes them so a CLI or an agent can validate a setting before
// sending it, and the validator here rejects anything outside them;
// both read the same variables so the two can never disagree.

// DatabaseDrivers lists the canonical database driver names.
var DatabaseDrivers = []string{
	"sqlite",
	"libsql",
	"postgres",
	"mysql",
	"mssql",
	"mongodb",
}

// DatabaseDriverAliases maps every accepted alternate spelling of a
// driver onto its canonical name, per the AI.md PART 5 DATABASE_DRIVER
// table.
var DatabaseDriverAliases = map[string]string{
	"sqlite2":    "sqlite",
	"sqlite3":    "sqlite",
	"turso":      "libsql",
	"pgsql":      "postgres",
	"postgresql": "postgres",
	"mariadb":    "mysql",
	"mongo":      "mongodb",
}

// DatabaseSSLModes lists the accepted database TLS modes.
var DatabaseSSLModes = []string{"disable", "require", "verify-full"}

// CacheTypes lists the accepted cache backends.
var CacheTypes = []string{"none", "memory", "valkey", "redis"}

// LogLevels lists the accepted log levels.
var LogLevels = []string{"debug", "info", "warn", "error"}

// SMTPTLSModes lists the accepted SMTP transport security modes.
var SMTPTLSModes = []string{"auto", "starttls", "tls", "none"}

// DurationUnits lists the suffixes accepted by a duration setting.
var DurationUnits = []string{"s", "m", "h", "d"}

// SizeUnits lists the suffixes accepted by a byte-size setting.
var SizeUnits = []string{"KB", "MB", "GB"}

// NormalizeDatabaseDriver maps every accepted spelling of a database
// driver onto its canonical name. An unrecognized value is returned
// lowercased so validation can reject it and warn.
func NormalizeDatabaseDriver(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if canonical, ok := DatabaseDriverAliases[name]; ok {
		return canonical
	}
	return name
}
