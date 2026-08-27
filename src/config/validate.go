package config

import (
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"time"
)

// Valid enumerations for the settings that accept a fixed set of
// values. A value outside its set is replaced with the default and a
// warning is recorded.
var (
	validLogLevels       = LogLevels
	validAccessFormats   = []string{"apache", "nginx", "json", "custom"}
	validTextFormats     = []string{"text", "json", "custom"}
	validSecurityFormats = []string{"fail2ban", "syslog", "cef", "json", "text", "custom"}
	validRotate          = []string{"never", "daily", "weekly", "monthly", "yearly"}
	validCacheTypes      = CacheTypes
	validDatabaseTypes   = DatabaseDrivers
	validSameSite        = []string{"strict", "lax", "none"}
	validSecure          = []string{"auto", "true", "false"}
	validTLSVersions     = []string{"TLS1.2", "TLS1.3"}
	validACMEChallenges  = []string{"http-01", "tls-alpn-01", "dns-01"}
	validAccountModes    = AccountModes
	validVisibilities    = ProfileVisibilities
	validOrgRoles        = OrgRoles
)

// MinPasswordLength is the shortest password AI.md PART 34 permits a
// deployment to configure.
const MinPasswordLength = 8

// Validate checks every configuration value, replacing anything
// invalid with its documented default and recording a warning.
//
// This implements the governing rule of AI.md PART 12: an invalid
// setting warns and falls back to the default; it never fails
// startup. Validate returns true when it changed anything, which
// tells Load to persist the corrected document.
func (c *Config) Validate() bool {
	before := len(c.warnings)
	def := DefaultConfig()

	c.validateServer(def)
	c.validateLimits(def)
	c.validateCompression(def)
	c.validateSession(def)
	c.validateRateLimit(def)
	c.validateI18n(def)
	c.validateCache(def)
	c.validateLogs(def)
	c.validateDatabase(def)
	c.validateTrustedProxies()
	c.validateSSL(def)
	c.validateWeb(def)
	c.validateUsers(def)
	c.validateOrgs(def)
	c.validateCustomDomains(def)
	c.validateNotifications(def)
	c.validateScheduler(def)
	c.validateGeoIP(def)
	c.validateMetrics(def)
	c.validateBackup(def)
	c.validateUpdate(def)
	c.validateTor(def)
	c.validateI2P(def)

	return len(c.warnings) != before
}

// validateScheduler checks server.scheduler.* per AI.md PART 19. The
// scheduler itself has no on/off switch, so this only normalizes the
// timezone, catch-up window, and each task's retention numbers.
func (c *Config) validateScheduler(def *Config) {
	s := &c.Server.Scheduler

	if s.Timezone == "" {
		s.Timezone = def.Server.Scheduler.Timezone
	} else if _, err := time.LoadLocation(s.Timezone); err != nil {
		c.warnf("invalid scheduler.timezone %q, using %q", s.Timezone, def.Server.Scheduler.Timezone)
		s.Timezone = def.Server.Scheduler.Timezone
	}

	if s.CatchUpWindow < 0 {
		c.warnf("invalid scheduler.catch_up_window %v, using %v", s.CatchUpWindow, def.Server.Scheduler.CatchUpWindow)
		s.CatchUpWindow = def.Server.Scheduler.CatchUpWindow
	}

	if s.Tasks == nil {
		s.Tasks = defaultSchedulerTasks()
		return
	}
	defaults := defaultSchedulerTasks()
	for name, defTask := range defaults {
		task, ok := s.Tasks[name]
		if !ok {
			s.Tasks[name] = defTask
			continue
		}
		if task.Schedule == "" {
			task.Schedule = defTask.Schedule
		}
		c.validateBackupRetention(&task.Retention, defTask.Retention, name)
		s.Tasks[name] = task
	}
}

// validateBackupRetention checks the numeric retention limits from
// AI.md PART 22. Every threshold here is a warn-not-error ceiling:
// an operator may exceed it deliberately, but redxt flags the choice.
func (c *Config) validateBackupRetention(r *BackupRetention, def BackupRetention, task string) {
	if r.MaxBackups <= 0 {
		r.MaxBackups = def.MaxBackups
	} else if r.MaxBackups > 7 {
		c.warnf("scheduler.tasks.%s.retention.max_backups %d exceeds the recommended ceiling of 7", task, r.MaxBackups)
	}
	if r.KeepWeekly < 0 {
		r.KeepWeekly = def.KeepWeekly
	} else if r.KeepWeekly > 8 {
		c.warnf("scheduler.tasks.%s.retention.keep_weekly %d exceeds the recommended ceiling of 8", task, r.KeepWeekly)
	}
	if r.KeepMonthly < 0 {
		r.KeepMonthly = def.KeepMonthly
	} else if r.KeepMonthly > 12 {
		c.warnf("scheduler.tasks.%s.retention.keep_monthly %d exceeds the recommended ceiling of 12", task, r.KeepMonthly)
	}
	if r.KeepYearly < 0 {
		r.KeepYearly = def.KeepYearly
	} else if r.KeepYearly > 2 {
		c.warnf("scheduler.tasks.%s.retention.keep_yearly %d exceeds the recommended ceiling of 2", task, r.KeepYearly)
	}
	if r.MaxTotalSize == "" {
		r.MaxTotalSize = def.MaxTotalSize
	}
}

// validateGeoIP checks server.geoip.* per AI.md PART 20. GeoIP stays
// disabled unless explicitly turned on, per IDEA.md's override of the
// generic spec default.
func (c *Config) validateGeoIP(def *Config) {
	_ = def
	g := &c.Server.GeoIP

	g.DenyCountries = c.normalizeCountryList(g.DenyCountries, "geoip.deny_countries")
	g.AllowCountries = c.normalizeCountryList(g.AllowCountries, "geoip.allow_countries")

	// PART 20: allow_countries wins when both are set. The losing list
	// is kept in the document so the operator can see what they wrote,
	// but the warning records which one actually applies.
	if len(g.AllowCountries) > 0 && len(g.DenyCountries) > 0 {
		c.warnf("geoip.allow_countries and geoip.deny_countries are both set; allow_countries takes precedence")
	}

	// Country blocking cannot work without the country database, and
	// PART 20 requires the fail-open path be a logged warning rather
	// than a silent no-op.
	if !g.Databases.Country && (len(g.AllowCountries) > 0 || len(g.DenyCountries) > 0) {
		c.warnf("geoip country blocking is configured but geoip.databases.country is disabled; country blocking will be skipped")
	}
}

// normalizeCountryList upper-cases and de-duplicates a country list,
// dropping anything that is not an ISO 3166-1 alpha-2 code.
func (c *Config) normalizeCountryList(list []string, name string) []string {
	if len(list) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, raw := range list {
		code := strings.ToUpper(strings.TrimSpace(raw))
		if len(code) != 2 || !isAlphaOnly(code) {
			c.warnf("invalid %s entry %q, expected an ISO 3166-1 alpha-2 code", name, raw)
			continue
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

// isAlphaOnly reports whether s is made up only of ASCII letters.
func isAlphaOnly(s string) bool {
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return s != ""
}

// validateMetrics checks server.metrics.* per AI.md PART 21. Tokens
// are free-form secrets and are never checked for shape; an empty
// token simply disables that service per PART 21's "empty token =
// service disabled" rule.
func (c *Config) validateMetrics(def *Config) {
	m := &c.Server.Metrics

	if m.Loki.MaxEntries <= 0 {
		m.Loki.MaxEntries = def.Server.Metrics.Loki.MaxEntries
	}
	if m.Loki.MaxAge <= 0 {
		m.Loki.MaxAge = def.Server.Metrics.Loki.MaxAge
	}
	if len(m.DurationBuckets) == 0 {
		m.DurationBuckets = append([]float64(nil), def.Server.Metrics.DurationBuckets...)
	}
	if len(m.SizeBuckets) == 0 {
		m.SizeBuckets = append([]float64(nil), def.Server.Metrics.SizeBuckets...)
	}
}

// validateBackup checks server.backup.* per AI.md PART 22. Compliance
// mode forces encryption on: an operator cannot both require
// compliance and disable encryption.
func (c *Config) validateBackup(def *Config) {
	b := &c.Server.Backup
	if b.Compliance.Enabled && !b.Encryption.Enabled {
		c.warnf("backup.compliance.enabled requires backup.encryption.enabled; enabling encryption")
		b.Encryption.Enabled = true
	}
	if b.DiskThreshold < 1 || b.DiskThreshold > 99 {
		if b.DiskThreshold != 0 {
			c.warnf("invalid backup.disk_threshold %d, using %d", b.DiskThreshold, def.Server.Backup.DiskThreshold)
		}
		b.DiskThreshold = def.Server.Backup.DiskThreshold
	}
}

// bandwidthRatePattern matches the "{n} KB"/"{n} MB" form PART 32.1
// accepts for tor's BandwidthRate and BandwidthBurst.
var bandwidthRatePattern = regexp.MustCompile(`^\d+\s*(KB|MB)$`)

// monthlyBandwidthPattern matches the "{n} GB"/"{n} TB"/"unlimited"
// form PART 32.1 accepts for max_monthly_bandwidth.
var monthlyBandwidthPattern = regexp.MustCompile(`^(\d+\s*(GB|TB)|unlimited)$`)

// validateTor checks tor.* per AI.md PART 32.1. Nothing here can
// disable the hidden service: PART 32.1 gives it no toggle, so an
// unusable value falls back to its default instead of switching Tor
// off.
func (c *Config) validateTor(def *Config) {
	t := &c.Tor
	d := def.Tor

	c.clampInt(&t.MaxCircuits, d.MaxCircuits, 1, 128, "tor.max_circuits")
	c.clampInt(&t.CircuitTimeout, d.CircuitTimeout, 10, 300, "tor.circuit_timeout")
	c.clampInt(&t.BootstrapTimeout, d.BootstrapTimeout, 30, 600, "tor.bootstrap_timeout")
	c.clampInt(&t.MaxStreamsPerCircuit, d.MaxStreamsPerCircuit, 10, 500, "tor.max_streams_per_circuit")
	c.clampInt(&t.NumIntroPoints, d.NumIntroPoints, 3, 10, "tor.num_intro_points")
	c.clampInt(&t.VirtualPort, d.VirtualPort, 1, 65535, "tor.virtual_port")

	t.BandwidthRate = c.normalizeBandwidth(t.BandwidthRate, d.BandwidthRate, bandwidthRatePattern, "tor.bandwidth_rate")
	t.BandwidthBurst = c.normalizeBandwidth(t.BandwidthBurst, d.BandwidthBurst, bandwidthRatePattern, "tor.bandwidth_burst")
	t.MaxMonthlyBandwidth = c.normalizeBandwidth(t.MaxMonthlyBandwidth, d.MaxMonthlyBandwidth, monthlyBandwidthPattern, "tor.max_monthly_bandwidth")

	// A burst below the sustained rate is not a rate limit tor can
	// honor, so the pair is reset together rather than half-corrected.
	rate, rateErr := ParseByteSize(t.BandwidthRate)
	burst, burstErr := ParseByteSize(t.BandwidthBurst)
	if rateErr == nil && burstErr == nil && burst < rate {
		c.warnf("tor.bandwidth_burst %q is below tor.bandwidth_rate %q, using %q", t.BandwidthBurst, t.BandwidthRate, d.BandwidthBurst)
		t.BandwidthRate = d.BandwidthRate
		t.BandwidthBurst = d.BandwidthBurst
	}
}

// validateI2P checks server.i2p.* per AI.md PART 32.2. Enabled is
// never touched here in either direction: the eepsite is opt-in only
// and validation must not become a path that turns it on.
func (c *Config) validateI2P(def *Config) {
	i := &c.Server.I2P
	d := def.Server.I2P

	if strings.TrimSpace(i.SAMAddress) == "" {
		i.SAMAddress = d.SAMAddress
	} else if _, _, err := net.SplitHostPort(i.SAMAddress); err != nil {
		c.warnf("invalid i2p.sam_address %q, using %q", i.SAMAddress, d.SAMAddress)
		i.SAMAddress = d.SAMAddress
	}

	c.clampInt(&i.VirtualPort, d.VirtualPort, 1, 65535, "i2p.virtual_port")
	c.clampInt(&i.InboundLength, d.InboundLength, 0, 7, "i2p.inbound_length")
	c.clampInt(&i.OutboundLength, d.OutboundLength, 0, 7, "i2p.outbound_length")
	c.clampInt(&i.InboundQuantity, d.InboundQuantity, 1, 16, "i2p.inbound_quantity")
	c.clampInt(&i.OutboundQuantity, d.OutboundQuantity, 1, 16, "i2p.outbound_quantity")
	c.clampInt(&i.SignatureType, d.SignatureType, 0, 11, "i2p.signature_type")
	c.clampInt(&i.BootstrapTimeout, d.BootstrapTimeout, 30, 600, "i2p.bootstrap_timeout")
}

// clampInt replaces an out-of-range value with fallback and records a
// warning. A zero value is treated as "unset" and takes the fallback
// silently, which is what an omitted YAML key unmarshals to.
func (c *Config) clampInt(v *int, fallback, min, max int, name string) {
	if *v == 0 && min > 0 {
		*v = fallback
		return
	}
	if *v < min || *v > max {
		c.warnf("invalid %s %d, expected %d-%d, using %d", name, *v, min, max, fallback)
		*v = fallback
	}
}

// normalizeBandwidth trims and validates a bandwidth string against
// pattern, falling back to fallback when it does not match.
func (c *Config) normalizeBandwidth(value, fallback string, pattern *regexp.Regexp, name string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	if !pattern.MatchString(trimmed) {
		c.warnf("invalid %s %q, using %q", value, name, fallback)
		return fallback
	}
	return trimmed
}

// validateUpdate checks server.update.* per AI.md PART 23.
func (c *Config) validateUpdate(def *Config) {
	u := &c.Server.Update
	validBranches := []string{"stable", "beta", "daily"}
	if u.Branch == "" {
		u.Branch = def.Server.Update.Branch
	} else if !contains(validBranches, strings.ToLower(u.Branch)) {
		c.warnf("invalid update.branch %q, using %q", u.Branch, def.Server.Update.Branch)
		u.Branch = def.Server.Update.Branch
	}
	if u.DeferDays < 0 {
		c.warnf("invalid update.defer_days %d, using %d", u.DeferDays, def.Server.Update.DeferDays)
		u.DeferDays = def.Server.Update.DeferDays
	}
}

// validateNotifications checks the PART 18 toast position, toast
// duration, and SMTP TLS mode. It never rejects a Host, Username,
// Password, or From value: those are free-form and their
// applicability is decided by autodetection/connection testing at
// startup, not by shape here.
func (c *Config) validateNotifications(def *Config) {
	n := &c.Server.Notifications

	validPositions := []string{"top-right", "top-left", "bottom-right", "bottom-left"}
	if !contains(validPositions, strings.ToLower(n.WebUI.Position)) {
		c.warnf("invalid notifications.webui.position %q, using %q", n.WebUI.Position, def.Server.Notifications.WebUI.Position)
		n.WebUI.Position = def.Server.Notifications.WebUI.Position
	}
	if n.WebUI.Duration < 0 {
		c.warnf("invalid notifications.webui.duration %v, using %v", n.WebUI.Duration, def.Server.Notifications.WebUI.Duration)
		n.WebUI.Duration = def.Server.Notifications.WebUI.Duration
	}

	smtp := &n.Email.SMTP
	if smtp.Port == 0 {
		smtp.Port = def.Server.Notifications.Email.SMTP.Port
	} else if smtp.Port < 0 || smtp.Port > 65535 {
		c.warnf("invalid notifications.email.smtp.port %d, using %d", smtp.Port, def.Server.Notifications.Email.SMTP.Port)
		smtp.Port = def.Server.Notifications.Email.SMTP.Port
	}
	if smtp.TLS == "" {
		smtp.TLS = def.Server.Notifications.Email.SMTP.TLS
	} else if !contains(SMTPTLSModes, strings.ToLower(smtp.TLS)) {
		c.warnf("invalid notifications.email.smtp.tls %q, using %q", smtp.TLS, def.Server.Notifications.Email.SMTP.TLS)
		smtp.TLS = def.Server.Notifications.Email.SMTP.TLS
	}

	if n.Email.Events == nil {
		n.Email.Events = defaultNotificationEvents()
	}
}

// validateServer checks the core listen address, port, base URL, and
// identity strings.
func (c *Config) validateServer(def *Config) {
	s := &c.Server

	if !containsFold(ValidModes, s.Mode) {
		if strings.TrimSpace(s.Mode) != "" {
			c.warnf("invalid server.mode %q, using %q", s.Mode, def.Server.Mode)
		}
		s.Mode = def.Server.Mode
	}
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))

	if strings.TrimSpace(s.Listen) == "" {
		s.Listen = def.Server.Listen
	} else if net.ParseIP(s.Listen) == nil && s.Listen != "localhost" {
		c.warnf("invalid server.listen %q, using %q", s.Listen, def.Server.Listen)
		s.Listen = def.Server.Listen
	}

	if s.Port < 1 || s.Port > 65535 {
		port := RandomUnusedPort()
		if s.Port != 0 {
			c.warnf("invalid server.port %d, using random port %d", s.Port, port)
		} else {
			c.warnf("server.port not set, selected random port %d", port)
		}
		s.Port = port
	}

	s.BaseURL = NormalizeBaseURL(s.BaseURL)

	if strings.TrimSpace(s.ApplicationName) == "" {
		s.ApplicationName = def.Server.ApplicationName
	}
	if strings.TrimSpace(s.ApplicationTagline) == "" {
		s.ApplicationTagline = def.Server.ApplicationTagline
	}

	if s.Security.EncryptionKeyVersion < 1 {
		s.Security.EncryptionKeyVersion = 1
	}

	if !IsRouteSegment(s.AdminPath) {
		if strings.TrimSpace(s.AdminPath) != "" {
			c.warnf("invalid server.admin_path %q, using %q", s.AdminPath, def.Server.AdminPath)
		}
		s.AdminPath = def.Server.AdminPath
	}

	if !IsRouteSegment(s.APIVersion) {
		if strings.TrimSpace(s.APIVersion) != "" {
			c.warnf("invalid server.api_version %q, using %q", s.APIVersion, def.Server.APIVersion)
		}
		s.APIVersion = def.Server.APIVersion
	}
}

// IsRouteSegment reports whether s is usable as a single URL path
// segment under the AI.md PART 14 route rules: non-empty, lowercase,
// alphanumeric with interior hyphens, and no slashes or dots.
func IsRouteSegment(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// validateSSL checks the TLS listener settings from AI.md PART 15.
func (c *Config) validateSSL(def *Config) {
	s := &c.Server.SSL
	d := def.Server.SSL

	if s.Port < 0 || s.Port > 65535 {
		c.warnf("invalid server.ssl.port %d, disabling the HTTPS listener", s.Port)
		s.Port = 0
	}
	if s.Port != 0 && s.Port == c.Server.Port {
		c.warnf("server.ssl.port %d duplicates server.port, disabling the HTTPS listener", s.Port)
		s.Port = 0
	}

	if !containsFold(validTLSVersions, s.MinVersion) {
		if strings.TrimSpace(s.MinVersion) != "" {
			c.warnf("invalid server.ssl.min_version %q, using %q", s.MinVersion, d.MinVersion)
		}
		s.MinVersion = d.MinVersion
	} else {
		s.MinVersion = strings.ToUpper(strings.TrimSpace(s.MinVersion))
	}

	// A manual override needs both halves; one alone would silently
	// fall through to auto-detection and hide the misconfiguration.
	if (s.Cert == "") != (s.Key == "") {
		c.warnf("server.ssl.cert and server.ssl.key must be set together, falling back to certificate auto-detection")
		s.Cert = ""
		s.Key = ""
	}

	le := &s.LetsEncrypt
	if !containsFold(validACMEChallenges, le.Challenge) {
		if strings.TrimSpace(le.Challenge) != "" {
			c.warnf("invalid server.ssl.letsencrypt.challenge %q, using %q", le.Challenge, d.LetsEncrypt.Challenge)
		}
		le.Challenge = d.LetsEncrypt.Challenge
	} else {
		le.Challenge = strings.ToLower(strings.TrimSpace(le.Challenge))
	}
	if le.Enabled && strings.TrimSpace(le.Email) == "" {
		c.warnf("server.ssl.letsencrypt.enabled is set without an email address, disabling automatic certificates")
		le.Enabled = false
	}
}

// validateWeb checks the CORS, CSRF, and HSTS policy settings.
func (c *Config) validateWeb(def *Config) {
	w := &c.Server.Web
	d := def.Server.Web

	w.CORS = strings.TrimSpace(w.CORS)

	if w.CSRF.ExemptPaths == nil {
		w.CSRF.ExemptPaths = append([]string(nil), d.CSRF.ExemptPaths...)
	}
	kept := w.CSRF.ExemptPaths[:0]
	for _, p := range w.CSRF.ExemptPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			c.warnf("invalid server.web.csrf.exempt_paths entry %q, must start with /", p)
			continue
		}
		kept = append(kept, p)
	}
	w.CSRF.ExemptPaths = kept

	if w.HSTS.MaxAge < 0 {
		c.warnf("invalid server.web.hsts.max_age %d, using %d", w.HSTS.MaxAge, d.HSTS.MaxAge)
		w.HSTS.MaxAge = d.HSTS.MaxAge
	}
	// Preload is only honored by browsers with a long max-age and the
	// includeSubDomains directive, so an incomplete set is corrected.
	if w.HSTS.Preload && (!w.HSTS.IncludeSubdomains || w.HSTS.MaxAge < 31536000) {
		c.warnf("server.web.hsts.preload requires include_subdomains and a max_age of at least 31536000, dropping preload")
		w.HSTS.Preload = false
	}
}

// containsFold reports whether list holds s, ignoring case and
// surrounding whitespace.
func containsFold(list []string, s string) bool {
	s = strings.TrimSpace(s)
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// validateUsers checks the Regular User subsystem settings from AI.md
// PART 34.
func (c *Config) validateUsers(def *Config) {
	u := &c.Server.Users
	d := def.Server.Users

	if !containsFold(validAccountModes, u.Registration.Mode) {
		c.warnf("invalid server.users.registration.mode %q, using %q", u.Registration.Mode, d.Registration.Mode)
		u.Registration.Mode = d.Registration.Mode
	}
	u.Registration.Mode = strings.ToLower(strings.TrimSpace(u.Registration.Mode))

	if u.Registration.InviteExpirationDays <= 0 {
		c.warnf("invalid server.users.registration.invite_expiration_days %d, using %d", u.Registration.InviteExpirationDays, d.Registration.InviteExpirationDays)
		u.Registration.InviteExpirationDays = d.Registration.InviteExpirationDays
	}

	u.Registration.AllowedDomains = normalizeDomainList(u.Registration.AllowedDomains)
	u.Registration.BlockedDomains = normalizeDomainList(u.Registration.BlockedDomains)

	c.requirePositiveDuration(&u.Auth.SessionDuration, d.Auth.SessionDuration, "server.users.auth.session_duration")
	c.requirePositiveDuration(&u.Auth.LockoutDuration, d.Auth.LockoutDuration, "server.users.auth.lockout_duration")

	if u.Auth.PasswordMinLength < MinPasswordLength {
		c.warnf("invalid server.users.auth.password_min_length %d, using %d", u.Auth.PasswordMinLength, d.Auth.PasswordMinLength)
		u.Auth.PasswordMinLength = d.Auth.PasswordMinLength
	}

	if u.Auth.MaxFailedLogins <= 0 {
		c.warnf("invalid server.users.auth.max_failed_logins %d, using %d", u.Auth.MaxFailedLogins, d.Auth.MaxFailedLogins)
		u.Auth.MaxFailedLogins = d.Auth.MaxFailedLogins
	}

	if u.Auth.Require2FA && !u.Auth.Allow2FA {
		c.warnf("server.users.auth.require_2fa is set while allow_2fa is not, enabling allow_2fa")
		u.Auth.Allow2FA = true
	}

	if u.Tokens.MaxPerUser < 0 {
		c.warnf("invalid server.users.tokens.max_per_user %d, using %d", u.Tokens.MaxPerUser, d.Tokens.MaxPerUser)
		u.Tokens.MaxPerUser = d.Tokens.MaxPerUser
	}

	if u.Tokens.ExpirationDays < 0 {
		c.warnf("invalid server.users.tokens.expiration_days %d, using %d", u.Tokens.ExpirationDays, d.Tokens.ExpirationDays)
		u.Tokens.ExpirationDays = d.Tokens.ExpirationDays
	}

	if !containsFold(validVisibilities, u.Profile.DefaultVisibility) {
		c.warnf("invalid server.users.profile.default_visibility %q, using %q", u.Profile.DefaultVisibility, d.Profile.DefaultVisibility)
		u.Profile.DefaultVisibility = d.Profile.DefaultVisibility
	}
	u.Profile.DefaultVisibility = strings.ToLower(strings.TrimSpace(u.Profile.DefaultVisibility))
}

// validateOrgs checks the organization settings from AI.md PART 35.
// Organizations build on Regular Users, so the section is forced off
// when users are disabled.
func (c *Config) validateOrgs(def *Config) {
	o := &c.Server.Orgs
	d := def.Server.Orgs

	if o.Enabled && !c.Server.Users.Enabled {
		c.warnf("server.orgs.enabled requires server.users.enabled, disabling organizations")
		o.Enabled = false
	}

	if !containsFold(validAccountModes, o.Creation.Mode) {
		c.warnf("invalid server.orgs.creation.mode %q, using %q", o.Creation.Mode, d.Creation.Mode)
		o.Creation.Mode = d.Creation.Mode
	}
	o.Creation.Mode = strings.ToLower(strings.TrimSpace(o.Creation.Mode))

	if o.Creation.MaxPerUser < 0 {
		c.warnf("invalid server.orgs.creation.max_per_user %d, using %d", o.Creation.MaxPerUser, d.Creation.MaxPerUser)
		o.Creation.MaxPerUser = d.Creation.MaxPerUser
	}

	if !containsFold(validVisibilities, o.Profile.DefaultVisibility) {
		c.warnf("invalid server.orgs.profile.default_visibility %q, using %q", o.Profile.DefaultVisibility, d.Profile.DefaultVisibility)
		o.Profile.DefaultVisibility = d.Profile.DefaultVisibility
	}
	o.Profile.DefaultVisibility = strings.ToLower(strings.TrimSpace(o.Profile.DefaultVisibility))

	if !containsFold(validOrgRoles, o.Members.DefaultRole) {
		c.warnf("invalid server.orgs.members.default_role %q, using %q", o.Members.DefaultRole, d.Members.DefaultRole)
		o.Members.DefaultRole = d.Members.DefaultRole
	}
	o.Members.DefaultRole = strings.ToLower(strings.TrimSpace(o.Members.DefaultRole))

	// An owner is created with the organization, so it can never be the
	// role handed to somebody joining an existing organization.
	if o.Members.DefaultRole == "owner" {
		c.warnf("server.orgs.members.default_role cannot be owner, using %q", d.Members.DefaultRole)
		o.Members.DefaultRole = d.Members.DefaultRole
	}
}

// validateCustomDomains checks the custom domain settings from AI.md
// PART 36. Custom domains are org-scoped, so the section is forced off
// when organizations are disabled.
func (c *Config) validateCustomDomains(def *Config) {
	cd := &c.Server.Features.CustomDomains
	d := def.Server.Features.CustomDomains

	if cd.Enabled && !c.Server.Orgs.Enabled {
		c.warnf("server.features.custom_domains.enabled requires server.orgs.enabled, disabling custom domains")
		cd.Enabled = false
	}

	if cd.MaxDomainsPerUser < 0 {
		c.warnf("invalid server.features.custom_domains.max_domains_per_user %d, using %d", cd.MaxDomainsPerUser, d.MaxDomainsPerUser)
		cd.MaxDomainsPerUser = d.MaxDomainsPerUser
	}

	if cd.MaxDomainsPerOrg < 0 {
		c.warnf("invalid server.features.custom_domains.max_domains_per_org %d, using %d", cd.MaxDomainsPerOrg, d.MaxDomainsPerOrg)
		cd.MaxDomainsPerOrg = d.MaxDomainsPerOrg
	}

	if !cd.AllowApex && !cd.AllowSubdomain && !cd.AllowWildcard {
		c.warnf("server.features.custom_domains allows no domain shape, re-enabling apex and subdomain")
		cd.AllowApex = true
		cd.AllowSubdomain = true
	}

	c.requirePositiveDuration(&cd.VerificationTTL, d.VerificationTTL, "server.features.custom_domains.verification_ttl")

	if cd.SSLRenewalDays <= 0 {
		c.warnf("invalid server.features.custom_domains.ssl_renewal_days %d, using %d", cd.SSLRenewalDays, d.SSLRenewalDays)
		cd.SSLRenewalDays = d.SSLRenewalDays
	}

	cd.Reserved = normalizeDomainList(cd.Reserved)

	// A pattern that does not compile would silently accept every domain
	// it was meant to block, so it is dropped with a warning instead.
	patterns := make([]string, 0, len(cd.BlockedPatterns))
	for _, p := range cd.BlockedPatterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := regexp.Compile(p); err != nil {
			c.warnf("invalid server.features.custom_domains.blocked_patterns entry %q, dropping it", p)
			continue
		}
		patterns = append(patterns, p)
	}
	cd.BlockedPatterns = patterns
}

// normalizeDomainList lowercases, trims, and drops empty entries from a
// configured list of domains.
func normalizeDomainList(list []string) []string {
	out := make([]string, 0, len(list))
	for _, entry := range list {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// validateLimits checks the request size and timeout limits.
func (c *Config) validateLimits(def *Config) {
	l := &c.Server.Limits
	d := def.Server.Limits

	if l.MaxBodySize <= 0 {
		c.warnf("invalid server.limits.max_body_size %d, using %s", l.MaxBodySize, d.MaxBodySize)
		l.MaxBodySize = d.MaxBodySize
	}
	c.requirePositiveDuration(&l.ReadTimeout, d.ReadTimeout, "server.limits.read_timeout")
	c.requirePositiveDuration(&l.WriteTimeout, d.WriteTimeout, "server.limits.write_timeout")
	c.requirePositiveDuration(&l.IdleTimeout, d.IdleTimeout, "server.limits.idle_timeout")
}

// validateCompression checks the compression level and MIME list.
func (c *Config) validateCompression(def *Config) {
	comp := &c.Server.Compression
	if comp.Level < 1 || comp.Level > 9 {
		c.warnf("invalid server.compression.level %d, using %d", comp.Level, def.Server.Compression.Level)
		comp.Level = def.Server.Compression.Level
	}
	if len(comp.Types) == 0 {
		comp.Types = append([]string(nil), DefaultCompressionTypes...)
	}
}

// validateSession checks the cookie names, lifetimes, and attributes
// for both session audiences.
func (c *Config) validateSession(def *Config) {
	s := &c.Server.Session
	d := def.Server.Session

	c.validateSessionScope(&s.Admin, d.Admin, "admin")
	c.validateSessionScope(&s.User, d.User, "user")

	if !contains(validSecure, strings.ToLower(s.Secure)) {
		c.warnf("invalid server.session.secure %q, using %q", s.Secure, d.Secure)
		s.Secure = d.Secure
	} else {
		s.Secure = strings.ToLower(s.Secure)
	}

	if !contains(validSameSite, strings.ToLower(s.SameSite)) {
		c.warnf("invalid server.session.same_site %q, using %q", s.SameSite, d.SameSite)
		s.SameSite = d.SameSite
	} else {
		s.SameSite = strings.ToLower(s.SameSite)
	}
}

// validateSessionScope checks one session audience.
func (c *Config) validateSessionScope(s *SessionScope, d SessionScope, name string) {
	if strings.TrimSpace(s.CookieName) == "" {
		s.CookieName = d.CookieName
	}
	c.requirePositiveDuration(&s.MaxAge, d.MaxAge, "server.session."+name+".max_age")
	c.requirePositiveDuration(&s.IdleTimeout, d.IdleTimeout, "server.session."+name+".idle_timeout")

	if s.IdleTimeout > s.MaxAge {
		c.warnf("server.session.%s.idle_timeout exceeds max_age, clamping to %s", name, s.MaxAge)
		s.IdleTimeout = s.MaxAge
	}
}

// validateRateLimit checks every rate-limit bucket and the global
// burst ceiling.
func (c *Config) validateRateLimit(def *Config) {
	r := &c.Server.RateLimit
	d := def.Server.RateLimit

	c.validateBucket(&r.Read, d.Read, "server.rate_limit.read")
	c.validateBucket(&r.Write, d.Write, "server.rate_limit.write")
	c.validateBucket(&r.Health, d.Health, "server.rate_limit.health")
	c.validateBucket(&r.Auth.Login, d.Auth.Login, "server.rate_limit.auth.login")
	c.validateBucket(&r.Auth.PasswordReset, d.Auth.PasswordReset, "server.rate_limit.auth.password_reset")
	c.validateBucket(&r.Auth.Registration, d.Auth.Registration, "server.rate_limit.auth.registration")

	if r.GlobalBurst < 1 {
		c.warnf("invalid server.rate_limit.global_burst %d, using %d", r.GlobalBurst, d.GlobalBurst)
		r.GlobalBurst = d.GlobalBurst
	}
}

// validateBucket checks one sliding-window rule.
func (c *Config) validateBucket(b *RateBucket, d RateBucket, name string) {
	if b.Requests < 1 {
		c.warnf("invalid %s.requests %d, using %d", name, b.Requests, d.Requests)
		b.Requests = d.Requests
	}
	c.requirePositiveDuration(&b.Window, d.Window, name+".window")
}

// validateI18n checks the language settings, guaranteeing the default
// language is always present in the supported list.
func (c *Config) validateI18n(def *Config) {
	i := &c.Server.I18n
	if strings.TrimSpace(i.DefaultLanguage) == "" {
		i.DefaultLanguage = def.Server.I18n.DefaultLanguage
	}
	i.DefaultLanguage = strings.ToLower(i.DefaultLanguage)

	if len(i.Supported) == 0 {
		i.Supported = []string{i.DefaultLanguage}
	} else {
		for n, lang := range i.Supported {
			i.Supported[n] = strings.ToLower(strings.TrimSpace(lang))
		}
		if !contains(i.Supported, i.DefaultLanguage) {
			c.warnf("server.i18n.default_language %q missing from available_languages, adding it", i.DefaultLanguage)
			i.Supported = append([]string{i.DefaultLanguage}, i.Supported...)
		}
	}

	if strings.TrimSpace(i.FallbackLanguage) == "" {
		i.FallbackLanguage = def.Server.I18n.FallbackLanguage
	}
	i.FallbackLanguage = strings.ToLower(i.FallbackLanguage)
	if !contains(i.Supported, i.FallbackLanguage) {
		c.warnf("server.i18n.fallback_language %q missing from available_languages, adding it", i.FallbackLanguage)
		i.Supported = append(i.Supported, i.FallbackLanguage)
	}

	if strings.TrimSpace(i.CookieName) == "" {
		i.CookieName = def.Server.I18n.CookieName
	}
	if i.CookieMaxAge <= 0 {
		c.warnf("invalid server.i18n.cookie_max_age %v, using %v", i.CookieMaxAge, def.Server.I18n.CookieMaxAge)
		i.CookieMaxAge = def.Server.I18n.CookieMaxAge
	}
}

// validateCache checks the cache driver and connection settings.
func (c *Config) validateCache(def *Config) {
	ca := &c.Server.Cache
	d := def.Server.Cache

	ca.Type = strings.ToLower(strings.TrimSpace(ca.Type))
	if ca.Type == "" {
		ca.Type = d.Type
	} else if !contains(validCacheTypes, ca.Type) {
		c.warnf("invalid server.cache.type %q, using %q", ca.Type, d.Type)
		ca.Type = d.Type
	}

	if strings.TrimSpace(ca.Host) == "" {
		ca.Host = d.Host
	}
	if ca.Port < 1 || ca.Port > 65535 {
		c.warnf("invalid server.cache.port %d, using %d", ca.Port, d.Port)
		ca.Port = d.Port
	}
	if ca.DB < 0 {
		c.warnf("invalid server.cache.db %d, using %d", ca.DB, d.DB)
		ca.DB = d.DB
	}
	if ca.PoolSize < 1 {
		c.warnf("invalid server.cache.pool_size %d, using %d", ca.PoolSize, d.PoolSize)
		ca.PoolSize = d.PoolSize
	}
	if ca.MinIdle < 0 {
		c.warnf("invalid server.cache.min_idle %d, using %d", ca.MinIdle, d.MinIdle)
		ca.MinIdle = d.MinIdle
	}
	if ca.MinIdle > ca.PoolSize {
		c.warnf("server.cache.min_idle exceeds pool_size, clamping to %d", ca.PoolSize)
		ca.MinIdle = ca.PoolSize
	}
	c.requirePositiveDuration(&ca.Timeout, d.Timeout, "server.cache.timeout")
	c.requirePositiveDuration(&ca.TTL, d.TTL, "server.cache.ttl")
	if strings.TrimSpace(ca.Prefix) == "" {
		ca.Prefix = d.Prefix
	}
}

// validateLogs checks the log level and every per-file format,
// rotation, and retention policy.
func (c *Config) validateLogs(def *Config) {
	l := &c.Server.Logs
	d := def.Server.Logs

	l.Level = strings.ToLower(strings.TrimSpace(l.Level))
	if !contains(validLogLevels, l.Level) {
		c.warnf("invalid server.logs.level %q, using %q", l.Level, d.Level)
		l.Level = d.Level
	}

	c.validateLogFile(&l.Access.LogFile, d.Access.LogFile, validAccessFormats, "server.logs.access")
	c.validateLogFile(&l.Server, d.Server, validTextFormats, "server.logs.server")
	c.validateLogFile(&l.Error, d.Error, validTextFormats, "server.logs.error")
	c.validateLogFile(&l.Security, d.Security, validSecurityFormats, "server.logs.security")
	c.validateLogFile(&l.Debug.LogFile, d.Debug.LogFile, validTextFormats, "server.logs.debug")

	// The audit log must stay machine parseable, so json is the only
	// accepted format regardless of what the file requests.
	c.validateLogFile(&l.Audit.LogFile, d.Audit.LogFile, []string{"json"}, "server.logs.audit")
}

// validateLogFile checks one log destination against its permitted
// format list.
func (c *Config) validateLogFile(f *LogFile, d LogFile, formats []string, name string) {
	if strings.TrimSpace(f.Filename) == "" {
		f.Filename = d.Filename
	}
	if strings.ContainsAny(f.Filename, `/\`) {
		c.warnf("invalid %s.filename %q (path separators are not allowed), using %q", name, f.Filename, d.Filename)
		f.Filename = d.Filename
	}

	f.Format = strings.ToLower(strings.TrimSpace(f.Format))
	if !contains(formats, f.Format) {
		c.warnf("invalid %s.format %q, using %q", name, f.Format, d.Format)
		f.Format = d.Format
	}
	if f.Format == "custom" && strings.TrimSpace(f.Custom) == "" {
		c.warnf("%s.format is custom but %s.custom is empty, using %q", name, name, d.Format)
		f.Format = d.Format
	}

	if !ValidRotate(f.Rotate) {
		c.warnf("invalid %s.rotate %q, using %q", name, f.Rotate, d.Rotate)
		f.Rotate = d.Rotate
	}
	if !ValidKeep(f.Keep) {
		c.warnf("invalid %s.keep %q, using %q", name, f.Keep, d.Keep)
		f.Keep = d.Keep
	}
}

// validateDatabase checks the driver, port, and pool settings.
func (c *Config) validateDatabase(def *Config) {
	db := &c.Server.Database
	d := def.Server.Database

	db.Type = NormalizeDatabaseDriver(db.Type)
	if db.Type == "" {
		db.Type = d.Type
	} else if !contains(validDatabaseTypes, db.Type) {
		c.warnf("invalid server.database.type %q, using %q", db.Type, d.Type)
		db.Type = d.Type
	}

	if db.Port < 0 || db.Port > 65535 {
		c.warnf("invalid server.database.port %d, ignoring it", db.Port)
		db.Port = 0
	}
	if db.MaxOpenConns < 1 {
		c.warnf("invalid server.database.max_open_conns %d, using %d", db.MaxOpenConns, d.MaxOpenConns)
		db.MaxOpenConns = d.MaxOpenConns
	}
	if db.MaxIdleConns < 1 {
		c.warnf("invalid server.database.max_idle_conns %d, using %d", db.MaxIdleConns, d.MaxIdleConns)
		db.MaxIdleConns = d.MaxIdleConns
	}
	if db.MaxIdleConns > db.MaxOpenConns {
		c.warnf("server.database.max_idle_conns exceeds max_open_conns, clamping to %d", db.MaxOpenConns)
		db.MaxIdleConns = db.MaxOpenConns
	}
	c.requirePositiveDuration(&db.ConnMaxLifetime, d.ConnMaxLifetime, "server.database.conn_max_lifetime")
	c.requirePositiveDuration(&db.ConnMaxIdleTime, d.ConnMaxIdleTime, "server.database.conn_max_idle_time")
}

// validateTrustedProxies drops entries that are neither an IP, a
// CIDR, nor a plausible DNS name. Private and loopback ranges are
// always trusted and never need to be listed.
func (c *Config) validateTrustedProxies() {
	in := c.Server.TrustedProxies.Additional
	out := make([]string, 0, len(in))
	for _, entry := range in {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if net.ParseIP(entry) != nil {
			out = append(out, entry)
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err == nil {
			out = append(out, entry)
			continue
		}
		if isPlausibleHostname(entry) {
			out = append(out, entry)
			continue
		}
		c.warnf("invalid server.trusted_proxies.additional entry %q, dropping it", entry)
	}
	c.Server.TrustedProxies.Additional = out
}

// requirePositiveDuration replaces a non-positive duration with its
// default and records a warning.
func (c *Config) requirePositiveDuration(d *Duration, fallback Duration, name string) {
	if *d > 0 {
		return
	}
	c.warnf("invalid %s %s, using %s", name, d, fallback)
	*d = fallback
}

// NormalizeBaseURL canonicalizes a base URL prefix: an empty value
// becomes "/", a missing leading slash is added, and a trailing slash
// is removed so both "/myproject" and "/myproject/" behave the same.
func NormalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return "/"
	}
	return raw
}

// ValidRotate reports whether a rotation policy is one of the
// documented forms: a named period, a size such as "50MB", or a
// combined "period,size" pair.
func ValidRotate(policy string) bool {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		return false
	}
	for _, part := range strings.Split(policy, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		if contains(validRotate, part) {
			continue
		}
		if n, err := ParseByteSize(part); err == nil && n > 0 {
			continue
		}
		return false
	}
	return true
}

// ValidKeep reports whether a retention policy is one of the
// documented forms: "none", "forever", a plain file count, or a
// count suffixed with d, w, or m.
func ValidKeep(policy string) bool {
	policy = strings.ToLower(strings.TrimSpace(policy))
	switch policy {
	case "":
		return false
	case "none", "forever":
		return true
	}
	head := policy
	switch policy[len(policy)-1] {
	case 'd', 'w', 'm':
		head = policy[:len(policy)-1]
	}
	if head == "" {
		return false
	}
	for _, r := range head {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isPlausibleHostname reports whether a string could be a DNS name
// that the trusted-proxy resolver will look up at startup.
func isPlausibleHostname(s string) bool {
	if s == "" || len(s) > 253 || strings.HasPrefix(s, "-") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			default:
				return false
			}
		}
	}
	return true
}

// contains reports whether the list holds the value.
func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// RandomUnusedPort selects a random free port in the 64000-64999
// range described in AI.md PART 5. If the whole range is occupied it
// falls back to a kernel-assigned ephemeral port so startup still
// succeeds.
func RandomUnusedPort() int {
	span := defaultPortRangeHigh - defaultPortRangeLow + 1
	for attempt := 0; attempt < span*2; attempt++ {
		port := defaultPortRangeLow + rand.Intn(span)
		if isPortFree(port) {
			return port
		}
	}
	if port, ok := ephemeralPort(); ok {
		return port
	}
	return defaultPortRangeLow
}

// isPortFree reports whether a TCP port can be bound right now.
func isPortFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// ephemeralPort asks the kernel for any free port.
func ephemeralPort() (int, bool) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, false
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, false
	}
	return addr.Port, true
}

// EnsureEncryptionKey generates the server encryption key when it is
// missing, using the supplied generator. Config deliberately does not
// import the security package, so the caller passes
// security.GenerateEncryptionKey. It reports whether a key was
// created, which tells the caller to persist the config.
func EnsureEncryptionKey(c *Config, generate func() (string, error)) (bool, error) {
	if strings.TrimSpace(c.Server.Security.EncryptionKey) != "" {
		return false, nil
	}
	key, err := generate()
	if err != nil {
		return false, fmt.Errorf("config: generate encryption key: %w", err)
	}
	c.Server.Security.EncryptionKey = key
	if c.Server.Security.EncryptionKeyVersion < 1 {
		c.Server.Security.EncryptionKeyVersion = 1
	}
	return true, nil
}

// SessionSecure resolves the "auto" setting of server.session.secure
// into a concrete decision. Auto means the cookie is marked Secure
// whenever the request was served over TLS.
func (c *Config) SessionSecure(overTLS bool) bool {
	switch c.Server.Session.Secure {
	case "true":
		return true
	case "false":
		return false
	default:
		return overTLS
	}
}

// LogRotationInterval converts the time component of a rotation
// policy into a duration. It returns false when the policy rotates on
// size only or never rotates on a schedule.
func LogRotationInterval(policy string) (time.Duration, bool) {
	for _, part := range strings.Split(strings.ToLower(policy), ",") {
		switch strings.TrimSpace(part) {
		case "daily":
			return 24 * time.Hour, true
		case "weekly":
			return 7 * 24 * time.Hour, true
		case "monthly":
			return 30 * 24 * time.Hour, true
		case "yearly":
			return 365 * 24 * time.Hour, true
		}
	}
	return 0, false
}

// LogRotationSize converts the size component of a rotation policy
// into a byte count. It returns false when the policy rotates on a
// schedule only.
func LogRotationSize(policy string) (int64, bool) {
	for _, part := range strings.Split(strings.ToLower(policy), ",") {
		part = strings.TrimSpace(part)
		if part == "" || contains(validRotate, part) {
			continue
		}
		if n, err := ParseByteSize(part); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}
