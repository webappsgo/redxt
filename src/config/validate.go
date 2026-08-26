package config

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

// Valid enumerations for the settings that accept a fixed set of
// values. A value outside its set is replaced with the default and a
// warning is recorded.
var (
	validLogLevels       = []string{"debug", "info", "warn", "error"}
	validAccessFormats   = []string{"apache", "nginx", "json", "custom"}
	validTextFormats     = []string{"text", "json", "custom"}
	validSecurityFormats = []string{"fail2ban", "syslog", "cef", "json", "text", "custom"}
	validRotate          = []string{"never", "daily", "weekly", "monthly", "yearly"}
	validCacheTypes      = []string{"none", "memory", "valkey", "redis"}
	validDatabaseTypes   = []string{"sqlite", "libsql", "postgres", "mysql", "mssql", "mongodb"}
	validSameSite        = []string{"strict", "lax", "none"}
	validSecure          = []string{"auto", "true", "false"}
)

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

	return len(c.warnings) != before
}

// validateServer checks the core listen address, port, base URL, and
// identity strings.
func (c *Config) validateServer(def *Config) {
	s := &c.Server

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
		return
	}
	for n, lang := range i.Supported {
		i.Supported[n] = strings.ToLower(strings.TrimSpace(lang))
	}
	if !contains(i.Supported, i.DefaultLanguage) {
		c.warnf("server.i18n.default_language %q missing from supported list, adding it", i.DefaultLanguage)
		i.Supported = append([]string{i.DefaultLanguage}, i.Supported...)
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
