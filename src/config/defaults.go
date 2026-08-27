package config

import (
	"time"
)

// Default application identity. These are the first-run values for
// server.application_name and server.application_tagline.
const (
	DefaultApplicationName    = "redxt"
	DefaultApplicationTagline = "The Complete DNS Server"
)

// Default routing segments. AI.md PART 14 fixes the API version prefix
// and PART 17 fixes the admin panel path; both stay configurable.
const (
	DefaultAPIVersion = "v1"
	DefaultAdminPath  = "administration"
)

// DefaultMode is the persisted application mode on a fresh install.
// PART 6 makes production the default and forbids implying debug.
const DefaultMode = "production"

// ValidModes are the values `--maintenance mode` accepts and the only
// values server.mode may hold. Debug mode is deliberately absent: PART 6
// makes it explicit opt-in via --mode/MODE only, and a persisted value
// would re-enable it on every restart without an operator saying so.
var ValidModes = []string{"production", "development"}

// DefaultHSTSMaxAge is the two-year max-age AI.md PART 15 requires on
// the Strict-Transport-Security header.
const DefaultHSTSMaxAge = 63072000

// Registration and organization creation modes. AI.md PART 34 lists open
// as the spec-wide default; IDEA.md sets redxt to invite, and a project
// decision in IDEA.md outranks the generic spec default.
const (
	DefaultRegistrationMode = "invite"
	DefaultOrgCreationMode  = "open"
)

// Regular User account defaults per AI.md PART 34.
const (
	DefaultInviteExpirationDays = 7
	DefaultPasswordMinLength    = 8
	DefaultMaxFailedLogins      = 5
	DefaultMaxTokensPerUser     = 10
	DefaultTokenExpirationDays  = 90
)

// DefaultOrgMemberRole is the role a newly added organization member
// receives. IDEA.md makes viewer the least-privilege starting point.
const DefaultOrgMemberRole = "viewer"

// Profile visibility values shared by user and organization profiles.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// DefaultReservedDomains lists the custom domains AI.md PART 36 refuses
// to let anyone claim.
var DefaultReservedDomains = []string{
	"localhost",
	"*.local",
	"*.test",
	"*.example",
	"*.invalid",
}

// DefaultBlockedDomainPatterns lists the regular expressions a custom
// domain must not match, per AI.md PART 36.
var DefaultBlockedDomainPatterns = []string{
	`.*\.(gov|mil|edu)$`,
}

// DefaultCSRFExemptPaths lists the route prefixes that never carry a
// browser session cookie and therefore never need a CSRF token.
var DefaultCSRFExemptPaths = []string{
	"/api/",
	"/.well-known/",
}

// defaultPortRangeLow and defaultPortRangeHigh bound the random
// first-run port selection described in AI.md PART 5, Port Rules.
const (
	defaultPortRangeLow  = 64000
	defaultPortRangeHigh = 64999
)

// DefaultCompressionTypes lists the MIME types compressed by default,
// per AI.md PART 12 "Response Compression".
var DefaultCompressionTypes = []string{
	"text/html",
	"text/css",
	"text/javascript",
	"application/json",
	"application/xml",
}

// DefaultConfig returns a Config populated entirely with the
// documented defaults from AI.md PART 11 and PART 12. Load starts
// from this value and overlays the file on top, so any key absent
// from server.yml keeps its default without needing a zero check.
func DefaultConfig() *Config {
	return &Config{
		Tor: DefaultTorConfig(),
		Server: Server{
			Mode:               DefaultMode,
			Listen:             "0.0.0.0",
			Port:               0,
			BaseURL:            "/",
			ApplicationName:    DefaultApplicationName,
			ApplicationTagline: DefaultApplicationTagline,
			AdminPath:          DefaultAdminPath,
			APIVersion:         DefaultAPIVersion,

			SSL: SSL{
				Enabled:    false,
				MinVersion: "TLS1.2",
				LetsEncrypt: LetsEncrypt{
					Enabled:   false,
					Challenge: "http-01",
					Staging:   false,
				},
			},

			Web: Web{
				CORS: "*",
				CSRF: CSRF{
					Enabled:     true,
					ExemptPaths: append([]string(nil), DefaultCSRFExemptPaths...),
				},
				HSTS: HSTS{
					Enabled:           true,
					MaxAge:            DefaultHSTSMaxAge,
					IncludeSubdomains: true,
					Preload:           true,
				},
			},

			Limits: Limits{
				MaxBodySize:  ByteSize(10 << 20),
				ReadTimeout:  Duration(30 * time.Second),
				WriteTimeout: Duration(30 * time.Second),
				IdleTimeout:  Duration(120 * time.Second),
			},

			Compression: Compression{
				Enabled: true,
				Level:   5,
				Types:   append([]string(nil), DefaultCompressionTypes...),
			},

			TrustedProxies: TrustedProxies{
				Additional: []string{},
			},

			Session: Session{
				Admin: SessionScope{
					CookieName:  "admin_session",
					MaxAge:      Duration(30 * 24 * time.Hour),
					IdleTimeout: Duration(24 * time.Hour),
				},
				User: SessionScope{
					CookieName:  "user_session",
					MaxAge:      Duration(7 * 24 * time.Hour),
					IdleTimeout: Duration(24 * time.Hour),
				},
				ExtendOnActivity: true,
				Secure:           "auto",
				HTTPOnly:         true,
				SameSite:         "strict",
			},

			RateLimit: RateLimit{
				Enabled:     true,
				Read:        RateBucket{Requests: 120, Window: Duration(60 * time.Second)},
				Write:       RateBucket{Requests: 10, Window: Duration(60 * time.Second)},
				Health:      RateBucket{Requests: 120, Window: Duration(60 * time.Second)},
				GlobalBurst: 240,
				Auth: AuthRateLimit{
					Login:         RateBucket{Requests: 5, Window: Duration(900 * time.Second)},
					PasswordReset: RateBucket{Requests: 3, Window: Duration(3600 * time.Second)},
					Registration:  RateBucket{Requests: 5, Window: Duration(3600 * time.Second)},
				},
			},

			I18n: I18n{
				Enabled:          true,
				DefaultLanguage:  "en",
				Supported:        []string{"en", "es", "zh", "fr", "ar", "de", "ja"},
				FallbackLanguage: "en",
				CookieName:       "lang",
				CookieMaxAge:     Duration(365 * 24 * time.Hour),
			},

			Cache: Cache{
				Type:     "memory",
				Host:     "localhost",
				Port:     6379,
				DB:       0,
				PoolSize: 10,
				MinIdle:  2,
				Timeout:  Duration(5 * time.Second),
				Prefix:   DefaultApplicationName + ":",
				TTL:      Duration(time.Hour),
				Cluster:  false,
			},

			Logs: Logs{
				Level: "warn",
				Access: AccessLog{
					LogFile: LogFile{
						Filename: "access.log",
						Format:   "apache",
						Rotate:   "monthly",
						Keep:     "none",
					},
					LogHealthChecks: false,
				},
				Server: LogFile{
					Filename: "server.log",
					Format:   "text",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Error: LogFile{
					Filename: "error.log",
					Format:   "text",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Audit: AuditLog{
					Enabled: true,
					LogFile: LogFile{
						Filename: "audit.log",
						Format:   "json",
						Rotate:   "daily",
						Keep:     "none",
					},
					Compress: false,
				},
				Security: LogFile{
					Filename: "security.log",
					Format:   "fail2ban",
					Rotate:   "weekly,50MB",
					Keep:     "none",
				},
				Debug: DebugLog{
					Enabled: false,
					LogFile: LogFile{
						Filename: "debug.log",
						Format:   "text",
						Rotate:   "weekly,50MB",
						Keep:     "none",
					},
				},
			},

			Database: Database{
				Type:            "sqlite",
				SSLMode:         "disable",
				MaxOpenConns:    25,
				MaxIdleConns:    5,
				ConnMaxLifetime: Duration(5 * time.Minute),
				ConnMaxIdleTime: Duration(time.Minute),
			},

			Security: Security{
				EncryptionKeyVersion: 1,
			},

			Healthz: Healthz{
				Root: HealthzRoot{
					Enabled: false,
				},
			},

			Users: Users{
				Enabled: true,
				Registration: Registration{
					Mode:                     DefaultRegistrationMode,
					RequireEmailVerification: true,
					InviteExpirationDays:     DefaultInviteExpirationDays,
				},
				Auth: UserAuth{
					SessionDuration:          Duration(7 * 24 * time.Hour),
					Allow2FA:                 true,
					PasswordMinLength:        DefaultPasswordMinLength,
					PasswordRequireUppercase: true,
					PasswordRequireLowercase: true,
					PasswordRequireNumber:    true,
					MaxFailedLogins:          DefaultMaxFailedLogins,
					LockoutDuration:          Duration(15 * time.Minute),
				},
				Tokens: UserTokens{
					Enabled:        true,
					MaxPerUser:     DefaultMaxTokensPerUser,
					ExpirationDays: DefaultTokenExpirationDays,
				},
				Profile: UserProfile{
					DefaultVisibility: VisibilityPublic,
					AllowBio:          true,
					AllowWebsite:      true,
					AllowLocation:     true,
					AllowAvatar:       true,
				},
			},

			Orgs: Orgs{
				Enabled: true,
				Creation: OrgCreation{
					Mode: DefaultOrgCreationMode,
				},
				Profile: OrgProfile{
					DefaultVisibility: VisibilityPublic,
				},
				Members: OrgMembers{
					DefaultRole:  DefaultOrgMemberRole,
					AllowInvites: true,
				},
			},

			Features: Features{
				CustomDomains: CustomDomains{
					Enabled:           false,
					MaxDomainsPerUser: 5,
					MaxDomainsPerOrg:  20,
					RequireSSL:        true,
					AllowApex:         true,
					AllowSubdomain:    true,
					AllowWildcard:     false,
					VerificationTTL:   Duration(24 * time.Hour),
					SSLRenewalDays:    7,
					Reserved:          append([]string(nil), DefaultReservedDomains...),
					BlockedPatterns:   append([]string(nil), DefaultBlockedDomainPatterns...),
				},
			},

			Notifications: Notifications{
				WebUI: NotificationsWebUI{
					Position: DefaultNotificationPosition,
					Duration: Duration(5 * time.Second),
				},
				Email: NotificationsEmail{
					SMTP: SMTPConfig{
						Port: DefaultSMTPPort,
						TLS:  "auto",
					},
					Events: defaultNotificationEvents(),
				},
			},

			Scheduler: Scheduler{
				Timezone:      DefaultSchedulerTimezone,
				CatchUpWindow: Duration(time.Hour),
				Tasks:         defaultSchedulerTasks(),
			},

			GeoIP: GeoIP{
				Enabled:        true,
				DenyCountries:  []string{},
				AllowCountries: []string{},
				Databases: GeoIPDatabases{
					ASN:     true,
					Country: true,
					City:    true,
				},
			},

			I2P: DefaultI2PConfig(),

			Metrics: Metrics{
				Enabled: true,
				Root: MetricsRoot{
					Enabled: true,
				},
				Auth: MetricsAuth{
					AllowUnauthenticated: false,
				},
				IncludeSystem:   true,
				IncludeRuntime:  true,
				Loki:            MetricsLoki{MaxEntries: 1000, MaxAge: Duration(time.Hour)},
				DurationBuckets: append([]float64(nil), DefaultMetricsDurationBuckets...),
				SizeBuckets:     append([]float64(nil), DefaultMetricsSizeBuckets...),
			},

			Backup: Backup{
				Encryption: BackupEncryption{
					Enabled: true,
				},
				Compliance: BackupCompliance{
					Enabled: false,
				},
				DiskThreshold: DefaultBackupDiskThreshold,
			},

			Update: Update{
				Branch:      DefaultUpdateBranch,
				AutoInstall: false,
				DeferDays:   0,
			},
		},
	}
}

// DefaultSchedulerTimezone is the first-run scheduler.timezone value,
// per AI.md PART 19.
const DefaultSchedulerTimezone = "America/New_York"

// DefaultBackupDiskThreshold is the percentage of the backup
// filesystem that may be in use before a scheduled backup aborts,
// per AI.md PART 22.
const DefaultBackupDiskThreshold = 90

// DefaultTorConfig returns the AI.md PART 32.1 first-run tor block.
// The hidden service itself has no on/off switch: it comes up
// whenever a tor binary is present, so these values only shape how it
// runs, never whether it does.
func DefaultTorConfig() Tor {
	return Tor{
		Binary:                    "",
		UseNetwork:                false,
		AllowUserPreference:       true,
		MaxCircuits:               32,
		CircuitTimeout:            60,
		BootstrapTimeout:          180,
		SafeLogging:               true,
		MaxStreamsPerCircuit:      100,
		CloseCircuitOnStreamLimit: true,
		BandwidthRate:             "1 MB",
		BandwidthBurst:            "2 MB",
		MaxMonthlyBandwidth:       "100 GB",
		NumIntroPoints:            3,
		VirtualPort:               80,
	}
}

// DefaultI2PConfig returns the AI.md PART 32.2 first-run i2p block.
// Enabled is false and stays false unless an operator sets it: the
// eepsite is the one overlay network redxt never auto-enables.
func DefaultI2PConfig() I2P {
	return I2P{
		Enabled:          false,
		Binary:           "",
		SAMAddress:       "127.0.0.1:7656",
		VirtualPort:      80,
		InboundLength:    3,
		OutboundLength:   3,
		InboundQuantity:  5,
		OutboundQuantity: 5,
		SignatureType:    7,
		BootstrapTimeout: 300,
	}
}

// DefaultUpdateBranch is the first-run release channel, per AI.md
// PART 23.
const DefaultUpdateBranch = "stable"

// DefaultMetricsDurationBuckets are the request-duration histogram
// buckets, in seconds, per AI.md PART 21.
var DefaultMetricsDurationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// DefaultMetricsSizeBuckets are the request/response-size histogram
// buckets, in bytes, per AI.md PART 21.
var DefaultMetricsSizeBuckets = []float64{100, 1000, 10000, 100000, 1000000, 10000000}

// defaultSchedulerTasks returns the built-in task table from AI.md
// PART 19 "Task Configuration", so every task has a documented
// schedule even before an operator touches server.yml.
func defaultSchedulerTasks() map[string]SchedulerTask {
	return map[string]SchedulerTask{
		"ssl_renewal":      {Schedule: "0 3 * * *", Enabled: true},
		"geoip_update":     {Schedule: "0 3 * * 0", Enabled: true},
		"blocklist_update": {Schedule: "0 4 * * *", Enabled: true, RetryOnFail: true, RetryDelay: Duration(time.Hour)},
		"cve_update":       {Schedule: "0 5 * * *", Enabled: true, RetryOnFail: true, RetryDelay: Duration(time.Hour)},
		"update_check":     {Schedule: "0 6 * * *", Enabled: true},
		"session_cleanup":  {Schedule: "@every 15m", Enabled: true},
		"token_cleanup":    {Schedule: "@every 15m", Enabled: true},
		"log_rotation":     {Schedule: "0 0 * * *", Enabled: true},
		"backup_daily": {
			Schedule: "0 2 * * *",
			Enabled:  true,
			Verify:   true,
			Retention: BackupRetention{
				MaxBackups:   1,
				MaxTotalSize: "10%",
			},
		},
		"backup_hourly":     {Schedule: "@hourly", Enabled: false},
		"healthcheck_self":  {Schedule: "@every 5m", Enabled: true},
		"tor_health":        {Schedule: "@every 10m", Enabled: true, RestartOnFail: true},
		"i2p_health":        {Schedule: "@every 10m", Enabled: true, RestartOnFail: true},
		"cluster_heartbeat": {Schedule: "@every 30s", Enabled: true},
	}
}

// DefaultNotificationPosition is the first-run toast anchor, per AI.md
// PART 18 "Sane Defaults".
const DefaultNotificationPosition = "top-right"

// DefaultSMTPPort is the first-run SMTP submission port, per AI.md
// PART 18 "SMTP Config".
const DefaultSMTPPort = 587

// defaultNotificationEvents returns the per-event email-send defaults
// from AI.md PART 18 "Configuration". Every entry here can be
// overridden by an admin or a recipient, except the Security-category
// events (login_alert, security_alert, password_changed,
// token_regenerated), which PART 18's "Notification Preferences"
// table marks as never user-disableable — that rule is enforced in
// the notify decision engine, not by omitting them here.
func defaultNotificationEvents() map[string]bool {
	return map[string]bool{
		"startup":            false,
		"shutdown":           false,
		"backup_complete":    false,
		"backup_failed":      true,
		"ssl_expiring":       true,
		"ssl_renewed":        false,
		"ssl_renewal_failed": true,
		"login_alert":        true,
		"security_alert":     true,
		"scheduler_error":    true,
		"password_changed":   true,
		"token_regenerated":  true,
		"update_available":   false,
		"update_installed":   true,
	}
}
