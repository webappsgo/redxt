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
		Server: Server{
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
				DefaultLanguage: "en",
				Supported:       []string{"en"},
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
		},
	}
}
