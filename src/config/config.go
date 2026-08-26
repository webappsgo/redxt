// Package config loads, validates, and persists redxt's server.yml
// configuration.
//
// It implements AI.md PART 5 (Configuration) for the file location,
// legacy-name migration, and the random 64000-64999 first-run port
// range, and AI.md PART 12 (Server Configuration) for the full
// configuration tree: base URL, request limits, compression, trusted
// proxies, Tor, sessions, rate limiting, i18n, cache, and the PART 11
// logging tree.
//
// The governing rule from PART 12 is that an invalid value is never
// fatal: it is replaced with the documented default and a warning is
// recorded. Load therefore never fails because of a bad value, only
// because of unreadable or unparseable files.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/webappsgo/redxt/src/paths"
)

// Config is the full in-memory server.yml document.
type Config struct {
	Server Server `yaml:"server"`
	Tor    Tor    `yaml:"tor"`

	// path is the resolved location server.yml was loaded from and
	// will be saved to.
	path string

	// warnings collects every value that failed validation and was
	// replaced with its default. Startup logs these; they never abort
	// the process.
	warnings []string
}

// Server holds the server.* section of server.yml.
type Server struct {
	// Listen is the bind address for the HTTP admin/API surface.
	Listen string `yaml:"listen"`
	// Port is the HTTP admin/API port, persisted after the first-run
	// random selection described in PART 5.
	Port int `yaml:"port"`
	// BaseURL is the URL path prefix for all routes. Reverse-proxy
	// headers take priority over this value at request time.
	BaseURL string `yaml:"baseurl"`
	// ApplicationName is the display title.
	ApplicationName string `yaml:"application_name"`
	// ApplicationTagline is the display description.
	ApplicationTagline string `yaml:"application_tagline"`
	// AdminPath is the URL segment the admin panel is isolated under,
	// mounted as /server/{admin_path}. PART 17 requires this to be
	// configurable rather than fixed.
	AdminPath string `yaml:"admin_path"`
	// APIVersion is the version segment of every versioned API route,
	// used to build /api/{api_version}/... per PART 14.
	APIVersion string `yaml:"api_version"`

	SSL SSL `yaml:"ssl"`
	Web Web `yaml:"web"`

	Limits         Limits         `yaml:"limits"`
	Compression    Compression    `yaml:"compression"`
	TrustedProxies TrustedProxies `yaml:"trusted_proxies"`
	Session        Session        `yaml:"session"`
	RateLimit      RateLimit      `yaml:"rate_limit"`
	I18n           I18n           `yaml:"i18n"`
	Cache          Cache          `yaml:"cache"`
	Logs           Logs           `yaml:"logs"`
	Database       Database       `yaml:"database"`
	Security       Security       `yaml:"security"`
	Healthz        Healthz        `yaml:"healthz"`
	Contact        Contact        `yaml:"contact"`
	Users          Users          `yaml:"users"`
	Orgs           Orgs           `yaml:"orgs"`
	Features       Features       `yaml:"features"`
	Notifications  Notifications  `yaml:"notifications"`
}

// Users holds server.users.* per AI.md PART 34 "Multi-User".
type Users struct {
	// Enabled turns the Regular User subsystem on. Regular Users are
	// end users; they are never Server Admins and live in their own
	// tables.
	Enabled bool `yaml:"enabled"`

	Registration Registration `yaml:"registration"`
	Auth         UserAuth     `yaml:"auth"`
	Tokens       UserTokens   `yaml:"tokens"`
	Profile      UserProfile  `yaml:"profile"`
}

// Registration holds server.users.registration.* per PART 34.
type Registration struct {
	// Mode is open, invite, admin_only, or disabled. redxt defaults to
	// invite per IDEA.md, which overrides the spec-wide default of open.
	Mode string `yaml:"mode"`
	// RequireEmailVerification withholds a usable account until the
	// address is confirmed.
	RequireEmailVerification bool `yaml:"require_email_verification"`
	// AllowedDomains restricts signup to these email domains. An empty
	// list allows every domain that is not blocked.
	AllowedDomains []string `yaml:"allowed_domains"`
	// BlockedDomains refuses signup from these email domains.
	BlockedDomains []string `yaml:"blocked_domains"`
	// InviteExpirationDays is how long an issued invite stays redeemable.
	InviteExpirationDays int `yaml:"invite_expiration_days"`
}

// UserAuth holds server.users.auth.* per PART 34.
type UserAuth struct {
	// SessionDuration is the absolute lifetime of a user session.
	SessionDuration Duration `yaml:"session_duration"`
	// Allow2FA lets a user enroll a second factor.
	Allow2FA bool `yaml:"allow_2fa"`
	// Require2FA refuses a session until a second factor is enrolled.
	Require2FA bool `yaml:"require_2fa"`
	// PasswordMinLength is the shortest accepted password.
	PasswordMinLength int `yaml:"password_min_length"`
	// PasswordRequireUppercase demands an uppercase letter.
	PasswordRequireUppercase bool `yaml:"password_require_uppercase"`
	// PasswordRequireLowercase demands a lowercase letter.
	PasswordRequireLowercase bool `yaml:"password_require_lowercase"`
	// PasswordRequireNumber demands a digit.
	PasswordRequireNumber bool `yaml:"password_require_number"`
	// PasswordRequireSpecial demands a non-alphanumeric character.
	PasswordRequireSpecial bool `yaml:"password_require_special"`
	// MaxFailedLogins is how many wrong passwords lock an account.
	MaxFailedLogins int `yaml:"max_failed_logins"`
	// LockoutDuration is how long a locked account stays locked.
	LockoutDuration Duration `yaml:"lockout_duration"`
}

// UserTokens holds server.users.tokens.* per PART 34.
type UserTokens struct {
	// Enabled lets a user mint personal API tokens.
	Enabled bool `yaml:"enabled"`
	// MaxPerUser caps how many live tokens one user may hold.
	MaxPerUser int `yaml:"max_per_user"`
	// ExpirationDays is the default token lifetime. Zero means the
	// token does not expire on its own.
	ExpirationDays int `yaml:"expiration_days"`
}

// UserProfile holds server.users.profile.* per PART 34.
type UserProfile struct {
	// DefaultVisibility is public or private for a new account.
	DefaultVisibility string `yaml:"default_visibility"`
	// AllowBio lets a user publish a biography.
	AllowBio bool `yaml:"allow_bio"`
	// AllowWebsite lets a user publish a website link.
	AllowWebsite bool `yaml:"allow_website"`
	// AllowLocation lets a user publish a location.
	AllowLocation bool `yaml:"allow_location"`
	// AllowAvatar lets a user publish an avatar URL.
	AllowAvatar bool `yaml:"allow_avatar"`
}

// Orgs holds server.orgs.* per AI.md PART 35 "Organizations".
type Orgs struct {
	// Enabled turns organizations on. PART 35 requires PART 34, so this
	// has no effect while users are disabled.
	Enabled bool `yaml:"enabled"`

	Creation OrgCreation `yaml:"creation"`
	Profile  OrgProfile  `yaml:"profile"`
	Members  OrgMembers  `yaml:"members"`
}

// OrgCreation holds server.orgs.creation.* per PART 35.
type OrgCreation struct {
	// Mode is open, invite, admin_only, or disabled.
	Mode string `yaml:"mode"`
	// MaxPerUser caps how many organizations one user may own. Zero
	// means unlimited. The personal organization does not count.
	MaxPerUser int `yaml:"max_per_user"`
}

// OrgProfile holds server.orgs.profile.* per PART 35.
type OrgProfile struct {
	// DefaultVisibility is public or private for a new organization.
	DefaultVisibility string `yaml:"default_visibility"`
}

// OrgMembers holds server.orgs.members.* per PART 35.
type OrgMembers struct {
	// DefaultRole is the role a new member receives.
	DefaultRole string `yaml:"default_role"`
	// AllowInvites lets a member below admin invite others.
	AllowInvites bool `yaml:"allow_invites"`
	// Require2FA refuses membership to an account without a second
	// factor enrolled.
	Require2FA bool `yaml:"require_2fa"`
}

// Features holds server.features.* — the optional subsystems that are
// switched on per deployment rather than per request.
type Features struct {
	CustomDomains CustomDomains `yaml:"custom_domains"`
}

// CustomDomains holds server.features.custom_domains.* per AI.md PART 36.
type CustomDomains struct {
	// Enabled turns custom domain support on.
	Enabled bool `yaml:"enabled"`
	// MaxDomainsPerUser caps how many domains one user may hold in their
	// personal organization. Zero means unlimited.
	MaxDomainsPerUser int `yaml:"max_domains_per_user"`
	// MaxDomainsPerOrg caps how many domains one shared organization may
	// hold. Zero means unlimited.
	MaxDomainsPerOrg int `yaml:"max_domains_per_org"`
	// RequireSSL refuses to activate a domain without a certificate.
	RequireSSL bool `yaml:"require_ssl"`
	// AllowApex permits a registrable apex such as example.com.
	AllowApex bool `yaml:"allow_apex"`
	// AllowSubdomain permits a subdomain such as api.example.com.
	AllowSubdomain bool `yaml:"allow_subdomain"`
	// AllowWildcard permits a wildcard such as *.example.com. A
	// wildcard needs a DNS-01 challenge, so it is off by default.
	AllowWildcard bool `yaml:"allow_wildcard"`
	// VerificationTTL is how long an issued verification token stays
	// valid before it must be reissued.
	VerificationTTL Duration `yaml:"verification_ttl"`
	// SSLRenewalDays renews a certificate this many days before expiry.
	SSLRenewalDays int `yaml:"ssl_renewal_days"`
	// Reserved lists domains that can never be claimed.
	Reserved []string `yaml:"reserved"`
	// BlockedPatterns lists regular expressions a domain must not match.
	BlockedPatterns []string `yaml:"blocked_patterns"`
}

// Notifications holds server.notifications.* per AI.md PART 18 "Email &
// Notifications" — the WebUI toast/banner/notification-center settings
// and the SMTP-backed email settings, including the per-event send
// toggles.
type Notifications struct {
	WebUI NotificationsWebUI `yaml:"webui"`
	Email NotificationsEmail `yaml:"email"`
}

// NotificationsWebUI holds server.notifications.webui.*.
type NotificationsWebUI struct {
	// Position is where a toast is anchored on screen.
	Position string `yaml:"position"`
	// Duration is how long a non-error toast stays visible before it
	// auto-dismisses. An error toast is never auto-dismissed.
	Duration Duration `yaml:"duration"`
}

// NotificationsEmail holds server.notifications.email.*.
type NotificationsEmail struct {
	SMTP SMTPConfig `yaml:"smtp"`
	From EmailFrom  `yaml:"from"`
	// Events maps each notification event name to whether it sends an
	// email in addition to its WebUI notification. Security-category
	// events are never user-disableable regardless of this map.
	Events map[string]bool `yaml:"events"`
}

// SMTPConfig holds server.notifications.email.smtp.*. An empty Host
// means autodetect at startup; a non-empty Host is tested at startup
// instead.
type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// TLS is one of config.SMTPTLSModes: auto, starttls, tls, none.
	TLS string `yaml:"tls"`
}

// EmailFrom holds server.notifications.email.from.*. An empty Name
// falls back to the application name; an empty Email falls back to
// no-reply@{fqdn}.
type EmailFrom struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
}

// SSL holds server.ssl.* per AI.md PART 15 "SSL/TLS & Let's Encrypt".
// Empty Cert and Key mean the certificate is auto-detected through the
// PART 15 lookup order rather than loaded from a fixed path.
type SSL struct {
	// Enabled forces HTTPS on the configured port. Port 443 implies
	// HTTPS on its own; this setting overrides for any other port.
	Enabled bool `yaml:"enabled"`
	// Port is the HTTPS listener port when the server runs a dual
	// HTTP/HTTPS pair, as written `port: "80,443"` on the command line
	// or in a legacy config. Zero means no separate HTTPS listener.
	Port int `yaml:"port"`
	// Cert is an optional manual certificate path override.
	Cert string `yaml:"cert"`
	// Key is an optional manual private key path override.
	Key string `yaml:"key"`
	// MinVersion is the lowest negotiated TLS version, TLS1.2 or TLS1.3.
	MinVersion string `yaml:"min_version"`

	LetsEncrypt LetsEncrypt `yaml:"letsencrypt"`
}

// LetsEncrypt holds server.ssl.letsencrypt.* per PART 15.
type LetsEncrypt struct {
	// Enabled turns on automatic ACME certificate issuance and renewal.
	Enabled bool `yaml:"enabled"`
	// Email is the ACME account contact address.
	Email string `yaml:"email"`
	// Challenge is http-01, tls-alpn-01, or dns-01.
	Challenge string `yaml:"challenge"`
	// Staging directs issuance at the Let's Encrypt staging endpoint.
	Staging bool `yaml:"staging"`
}

// Web holds server.web.* — the browser-facing policy knobs defined in
// AI.md PART 16 (CORS, CSRF) and PART 15 (HSTS).
type Web struct {
	// CORS is the allow-list for Access-Control-Allow-Origin. "*"
	// leaves the policy unset and lets the resolution order pick an
	// origin; "" disables CORS entirely.
	CORS string `yaml:"cors"`

	CSRF CSRF `yaml:"csrf"`
	HSTS HSTS `yaml:"hsts"`
}

// CSRF holds server.web.csrf.* per PART 16 "CSRF Protection".
type CSRF struct {
	// Enabled turns on same-site token validation for cookie-authed
	// state-changing requests.
	Enabled bool `yaml:"enabled"`
	// ExemptPaths lists route prefixes that skip CSRF validation.
	ExemptPaths []string `yaml:"exempt_paths"`
}

// HSTS holds server.web.hsts.* per PART 15. The header is emitted only
// when the request was served over TLS.
type HSTS struct {
	// Enabled turns on Strict-Transport-Security.
	Enabled bool `yaml:"enabled"`
	// MaxAge is the max-age directive in seconds.
	MaxAge int `yaml:"max_age"`
	// IncludeSubdomains adds the includeSubDomains directive.
	IncludeSubdomains bool `yaml:"include_subdomains"`
	// Preload adds the preload directive.
	Preload bool `yaml:"preload"`
}

// Limits holds server.limits.* per PART 12 "Request Limits".
type Limits struct {
	// MaxBodySize is the largest accepted request body.
	MaxBodySize ByteSize `yaml:"max_body_size"`
	// ReadTimeout bounds reading the request.
	ReadTimeout Duration `yaml:"read_timeout"`
	// WriteTimeout bounds writing the response.
	WriteTimeout Duration `yaml:"write_timeout"`
	// IdleTimeout bounds an idle keep-alive connection.
	IdleTimeout Duration `yaml:"idle_timeout"`
}

// Compression holds server.compression.* per PART 12 "Response
// Compression".
type Compression struct {
	// Enabled turns response compression on.
	Enabled bool `yaml:"enabled"`
	// Level is the gzip compression level, 1 through 9.
	Level int `yaml:"level"`
	// Types lists the MIME types eligible for compression.
	Types []string `yaml:"types"`
}

// TrustedProxies holds server.trusted_proxies.* per PART 12. Private
// and loopback ranges are always trusted and are never listed here.
type TrustedProxies struct {
	// Additional lists extra IPs, CIDRs, or DNS names to trust as
	// upstream proxies, for proxies on routable networks.
	Additional []string `yaml:"additional"`
}

// Session holds server.session.* per PART 12 "Session Configuration".
type Session struct {
	// Admin configures the admin session cookie.
	Admin SessionScope `yaml:"admin"`
	// User configures the end-user session cookie.
	User SessionScope `yaml:"user"`
	// ExtendOnActivity resets the idle timeout on every request.
	ExtendOnActivity bool `yaml:"extend_on_activity"`
	// Secure controls the cookie Secure attribute: auto, true, false.
	Secure string `yaml:"secure"`
	// HTTPOnly controls the cookie HttpOnly attribute.
	HTTPOnly bool `yaml:"http_only"`
	// SameSite controls the cookie SameSite attribute: strict, lax,
	// none. The default is strict, matching the PART 11 CSRF defense.
	SameSite string `yaml:"same_site"`
}

// SessionScope holds the per-audience session settings.
type SessionScope struct {
	// CookieName is the session cookie name.
	CookieName string `yaml:"cookie_name"`
	// MaxAge is the absolute session lifetime.
	MaxAge Duration `yaml:"max_age"`
	// IdleTimeout expires a session after inactivity.
	IdleTimeout Duration `yaml:"idle_timeout"`
}

// RateLimit holds server.rate_limit.* per PART 12 "Rate Limiting".
type RateLimit struct {
	// Enabled turns rate limiting on. Rate limiting is the primary
	// abuse defense and is on by default.
	Enabled bool `yaml:"enabled"`
	// Read limits GET and HEAD requests.
	Read RateBucket `yaml:"read"`
	// Write limits POST, PUT, PATCH, and DELETE requests.
	Write RateBucket `yaml:"write"`
	// Health limits the health and status endpoints.
	Health RateBucket `yaml:"health"`
	// GlobalBurst is the absolute per-IP ceiling across all endpoint
	// classes, in requests per minute.
	GlobalBurst int `yaml:"global_burst"`
	// Auth holds the stricter authentication limits, applied
	// independently of the general limits above.
	Auth AuthRateLimit `yaml:"auth"`
}

// AuthRateLimit holds server.rate_limit.auth.* per PART 12.
type AuthRateLimit struct {
	// Login limits login attempts per IP and per identifier.
	Login RateBucket `yaml:"login"`
	// PasswordReset limits password-reset requests per IP.
	PasswordReset RateBucket `yaml:"password_reset"`
	// Registration limits registration attempts per IP.
	Registration RateBucket `yaml:"registration"`
}

// RateBucket is one sliding-window rate-limit rule.
type RateBucket struct {
	// Requests is the number of requests allowed in the window.
	Requests int `yaml:"requests"`
	// Window is the sliding window length.
	Window Duration `yaml:"window"`
}

// I18n holds server.i18n.* per PART 12 "Internationalization".
type I18n struct {
	// DefaultLanguage is the fallback language tag.
	DefaultLanguage string `yaml:"default_language"`
	// Supported lists the language tags the server serves.
	Supported []string `yaml:"supported"`
}

// Cache holds server.cache.* per PART 12 "Cache Configuration".
// Clustered and mixed-mode deployments require valkey or redis; a
// single instance runs on the in-process memory driver.
type Cache struct {
	// Type selects the driver: none, memory, valkey, redis.
	Type string `yaml:"type"`
	// URL is the full connection URL. It takes precedence over the
	// individual host, port, and password settings.
	URL string `yaml:"url"`
	// Host is the cache server hostname.
	Host string `yaml:"host"`
	// Port is the cache server port.
	Port int `yaml:"port"`
	// Username is the ACL username (Redis 6 and later).
	Username string `yaml:"username"`
	// Password is the cache server password.
	Password string `yaml:"password"`
	// DB is the numbered logical database.
	DB int `yaml:"db"`
	// TLS enables a TLS connection to the cache server.
	TLS bool `yaml:"tls"`
	// TLSSkipVerify disables certificate verification.
	TLSSkipVerify bool `yaml:"tls_skip_verify"`
	// PoolSize is the maximum number of pooled connections.
	PoolSize int `yaml:"pool_size"`
	// MinIdle is the minimum number of idle connections kept open.
	MinIdle int `yaml:"min_idle"`
	// Timeout bounds a single cache operation.
	Timeout Duration `yaml:"timeout"`
	// Prefix namespaces every key to avoid cross-application
	// collisions.
	Prefix string `yaml:"prefix"`
	// TTL is the default entry lifetime.
	TTL Duration `yaml:"ttl"`
	// Cluster enables Valkey/Redis cluster mode.
	Cluster bool `yaml:"cluster"`
	// ClusterNodes lists the cluster seed nodes.
	ClusterNodes []string `yaml:"cluster_nodes"`
}

// Logs holds server.logs.* per PART 11 "Logging".
type Logs struct {
	// Level is the global log level: debug, info, warn, error.
	Level string `yaml:"level"`

	Access   AccessLog `yaml:"access"`
	Server   LogFile   `yaml:"server"`
	Error    LogFile   `yaml:"error"`
	Audit    AuditLog  `yaml:"audit"`
	Security LogFile   `yaml:"security"`
	Debug    DebugLog  `yaml:"debug"`
}

// LogFile is the common shape shared by every log destination.
type LogFile struct {
	// Filename is the log file name inside the resolved log
	// directory.
	Filename string `yaml:"filename"`
	// Format selects the output encoding. Valid values vary by log
	// type; see the PART 11 format tables.
	Format string `yaml:"format"`
	// Custom is the format template used when Format is "custom".
	Custom string `yaml:"custom"`
	// Rotate is the rotation policy: never, daily, weekly, monthly,
	// yearly, a size such as 50MB, or a combined "weekly,50MB".
	Rotate string `yaml:"rotate"`
	// Keep is the retention policy: none, a count, Nd, Nw, Nm, or
	// forever.
	Keep string `yaml:"keep"`
}

// AccessLog adds the health-check suppression switch to the common
// log settings.
type AccessLog struct {
	LogFile `yaml:",inline"`
	// LogHealthChecks records successful health-check requests.
	// Failures are always logged regardless of this setting.
	LogHealthChecks bool `yaml:"log_health_checks"`
}

// AuditLog adds the enable and compress switches to the common log
// settings. The audit log is always JSON so it stays machine
// parseable.
type AuditLog struct {
	// Enabled turns the audit log on.
	Enabled bool `yaml:"enabled"`
	LogFile `yaml:",inline"`
	// Compress gzips rotated audit logs, which is only useful when
	// Keep retains them.
	Compress bool `yaml:"compress"`
}

// DebugLog adds an enable switch to the common log settings, since
// the debug log exists only for troubleshooting.
type DebugLog struct {
	// Enabled turns the debug log on.
	Enabled bool `yaml:"enabled"`
	LogFile `yaml:",inline"`
}

// Database holds server.database.* per PART 10. SQLite is the default
// and needs no configuration; the remaining fields apply only to a
// remote driver.
type Database struct {
	// Type selects the driver: sqlite, libsql, postgres, mysql,
	// mssql, mongodb.
	Type string `yaml:"type"`
	// URL is a full connection string. When set it takes precedence
	// over the individual host, port, name, user, and password
	// settings.
	URL string `yaml:"url"`
	// Host is the database server hostname.
	Host string `yaml:"host"`
	// Port is the database server port.
	Port int `yaml:"port"`
	// Name is the database (or schema) name.
	Name string `yaml:"name"`
	// User is the database username.
	User string `yaml:"user"`
	// Password is the database password.
	Password string `yaml:"password"`
	// SSLMode is the driver-specific TLS mode.
	SSLMode string `yaml:"sslmode"`
	// MaxOpenConns caps the total open connections.
	MaxOpenConns int `yaml:"max_open_conns"`
	// MaxIdleConns caps the idle connections kept in the pool.
	MaxIdleConns int `yaml:"max_idle_conns"`
	// ConnMaxLifetime retires a connection after this age.
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
	// ConnMaxIdleTime retires an idle connection after this long.
	ConnMaxIdleTime Duration `yaml:"conn_max_idle_time"`
}

// Security holds server.security.* per PART 11 "Cryptographic Keys".
// The encryption key is the only secret that lives in server.yml; all
// other project secrets live in the server.db app_secrets table.
type Security struct {
	// EncryptionKey is the base64-encoded 32-byte AES-256-GCM key
	// used for every at-rest encryption in the product, including
	// DNSSEC private keys and TOTP secrets. It is generated on first
	// run and must never be logged.
	EncryptionKey string `yaml:"encryption_key"`
	// EncryptionKeyVersion identifies which key generation encrypted
	// a given ciphertext, so a rotation can decrypt old data during
	// the grace window.
	EncryptionKeyVersion int `yaml:"encryption_key_version"`
}

// Healthz holds server.healthz.* per PART 13.
type Healthz struct {
	// Root gates the optional root-level /healthz alias. The canonical
	// route stays /server/healthz whether or not the alias is mounted.
	Root HealthzRoot `yaml:"root"`
}

// HealthzRoot holds server.healthz.root.* per AI.md PART 13 and PART 14.
type HealthzRoot struct {
	// Enabled mounts /healthz on the SAME handler as /server/healthz.
	// It is never a redirect, and it defaults to false.
	Enabled bool `yaml:"enabled"`
}

// Contact holds server.contact.* per PART 12 "Contact Configuration".
// It is the single place that answers "where do server messages go".
type Contact struct {
	// Admin receives administrative notifications and is the
	// fallback for every other role.
	Admin ContactRole `yaml:"admin"`
	// Security receives security reports and vulnerability
	// disclosures.
	Security ContactRole `yaml:"security"`
	// Abuse receives abuse reports.
	Abuse ContactRole `yaml:"abuse"`
	// Support receives contact-form submissions.
	Support ContactRole `yaml:"support"`
}

// ContactRole is one notification recipient: an address plus any
// number of webhook transports.
type ContactRole struct {
	// Email is the recipient address. An empty value falls back to
	// the admin address.
	Email string `yaml:"email"`
	// Webhooks lists the webhook transports for this role.
	Webhooks []ContactWebhook `yaml:"webhooks"`
}

// ContactWebhook is a single webhook transport for a contact role.
type ContactWebhook struct {
	// Type selects the transport: telegram, discord, slack, generic.
	Type string `yaml:"type"`
	// URL is the webhook endpoint.
	URL string `yaml:"url"`
	// Enabled turns this transport on.
	Enabled bool `yaml:"enabled"`
}

// Tor holds the top-level tor.* section per PART 12 "Tor Hidden
// Service Configuration". Tor detection is disabled while
// OnionAddress is empty.
type Tor struct {
	// OnionAddress is this service's .onion hostname without a
	// scheme. A request whose resolved Host matches it is a Tor
	// request.
	OnionAddress string `yaml:"onion_address"`
	// ContactEmail is shown on Tor responses. When it is empty no
	// email is shown; the clearnet address is never used as a
	// fallback.
	ContactEmail string `yaml:"contact_email"`
}

// Load reads server.yml from the resolved config path, migrating a
// legacy server.yaml if one is present, filling every unset field
// with its default, and validating the result. Invalid values are
// replaced with defaults and recorded in Warnings rather than
// aborting startup. The file is written back when anything changed.
func Load(p paths.Paths) (*Config, error) {
	cfg := DefaultConfig()
	cfg.path = p.ConfigFile

	if err := os.MkdirAll(filepath.Dir(p.ConfigFile), 0o750); err != nil {
		return nil, fmt.Errorf("config: create config dir: %w", err)
	}

	migrateLegacyYAML(p.ConfigFile)

	existed := false
	data, err := os.ReadFile(p.ConfigFile)
	switch {
	case err == nil:
		existed = true
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", p.ConfigFile, err)
		}
	case os.IsNotExist(err):
		// First run: the defaults stand as loaded.
	default:
		return nil, fmt.Errorf("config: read %s: %w", p.ConfigFile, err)
	}

	cfg.applyInitEnv()
	changed := cfg.Validate()

	if !existed || changed {
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("config: save %s: %w", p.ConfigFile, err)
		}
	}

	return cfg, nil
}

// Warnings returns every validation message recorded during Load, in
// the order the values were checked. Each entry names the setting,
// the rejected value, and the default that replaced it.
func (c *Config) Warnings() []string {
	out := make([]string, len(c.warnings))
	copy(out, c.warnings)
	return out
}

// warnf records a validation warning.
func (c *Config) warnf(format string, args ...any) {
	c.warnings = append(c.warnings, fmt.Sprintf(format, args...))
}

// migrateLegacyYAML renames a legacy server.yaml to server.yml on
// startup, per the PART 5 configuration-file migration rule. An
// existing server.yml always wins.
func migrateLegacyYAML(ymlPath string) {
	if len(ymlPath) < len("yml") {
		return
	}
	legacy := ymlPath[:len(ymlPath)-len("yml")] + "yaml"
	if _, err := os.Stat(ymlPath); err == nil {
		return
	}
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	_ = os.Rename(legacy, ymlPath)
}

// APIBasePath returns the versioned API prefix, "/api/" plus the
// configured version segment. AI.md PART 14 forbids hardcoding "v1"
// anywhere in the code, so every route builder starts from here.
func (c *Config) APIBasePath() string {
	v := c.Server.APIVersion
	if v == "" {
		v = DefaultAPIVersion
	}
	return "/api/" + v
}

// AdminBasePath returns the admin panel prefix, "/server/" plus the
// configured admin path segment, per AI.md PART 17.
func (c *Config) AdminBasePath() string {
	p := c.Server.AdminPath
	if p == "" {
		p = DefaultAdminPath
	}
	return "/server/" + p
}

// HTTPSPort returns the port the TLS listener should bind, or zero when
// this instance serves plain HTTP only. AI.md PART 15 makes port 443
// HTTPS-only on its own, lets ssl.enabled force TLS on any single port,
// and treats a configured ssl.port as the HTTPS half of a dual pair.
func (c *Config) HTTPSPort() int {
	if c.Server.SSL.Port > 0 {
		return c.Server.SSL.Port
	}
	if c.Server.Port == 443 || c.Server.SSL.Enabled {
		return c.Server.Port
	}
	return 0
}

// HTTPPort returns the port the plaintext listener should bind, or zero
// when the instance is HTTPS-only.
func (c *Config) HTTPPort() int {
	if c.Server.SSL.Port > 0 {
		return c.Server.Port
	}
	if c.Server.Port == 443 || c.Server.SSL.Enabled {
		return 0
	}
	return c.Server.Port
}

// Save writes the current configuration back to its resolved path.
// The file holds the AES-256-GCM encryption key, so it is written
// owner-and-group readable only.
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

// SetPath overrides the location this Config saves to. It exists for
// tests and for the first-run bootstrap, which resolves the path
// before the directory layout is known.
func (c *Config) SetPath(path string) {
	c.path = path
}
