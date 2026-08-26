package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/urlvars"
)

// Security header names from the PART 11 "Security Headers" table.
const (
	// HeaderContentTypeOptions blocks browser MIME sniffing.
	HeaderContentTypeOptions = "X-Content-Type-Options"
	// HeaderFrameOptions is the legacy clickjacking control, kept
	// alongside the CSP frame-ancestors directive for old browsers.
	HeaderFrameOptions = "X-Frame-Options"
	// HeaderXSSProtection is the deprecated reflected-XSS filter switch,
	// kept for older browser compatibility.
	HeaderXSSProtection = "X-XSS-Protection"
	// HeaderReferrerPolicy limits what the browser leaks in Referer.
	HeaderReferrerPolicy = "Referrer-Policy"
	// HeaderPermittedCrossDomain disables Adobe cross-domain policies.
	HeaderPermittedCrossDomain = "X-Permitted-Cross-Domain-Policies"
	// HeaderOriginAgentCluster requests origin-keyed agent clustering.
	HeaderOriginAgentCluster = "Origin-Agent-Cluster"
	// HeaderCOOP is Cross-Origin-Opener-Policy.
	HeaderCOOP = "Cross-Origin-Opener-Policy"
	// HeaderCOEP is Cross-Origin-Embedder-Policy.
	HeaderCOEP = "Cross-Origin-Embedder-Policy"
	// HeaderCORP is Cross-Origin-Resource-Policy.
	HeaderCORP = "Cross-Origin-Resource-Policy"
	// HeaderCSP is the enforcing Content-Security-Policy header.
	HeaderCSP = "Content-Security-Policy"
	// HeaderCSPReportOnly is the non-enforcing CSP header used in
	// development so violations are reported without breaking the page.
	HeaderCSPReportOnly = "Content-Security-Policy-Report-Only"
	// HeaderPermissionsPolicy gates browser feature access.
	HeaderPermissionsPolicy = "Permissions-Policy"
	// HeaderReportingEndpoints is the modern Reporting API endpoint map.
	HeaderReportingEndpoints = "Reporting-Endpoints"
	// HeaderReportTo is the legacy Reporting API group definition.
	HeaderReportTo = "Report-To"
	// HeaderNEL is the Network Error Logging policy.
	HeaderNEL = "NEL"
	// HeaderHSTS is Strict-Transport-Security, emitted over TLS only.
	HeaderHSTS = "Strict-Transport-Security"
	// HeaderClearSiteData instructs the browser to drop local state.
	HeaderClearSiteData = "Clear-Site-Data"
)

// Fixed security header values from the PART 11 table. These are not
// configurable: every one of them is a hard requirement of the spec.
const (
	// ValueNoSniff is the only accepted X-Content-Type-Options value.
	ValueNoSniff = "nosniff"
	// ValueSameOrigin is the X-Frame-Options value redxt serves.
	ValueSameOrigin = "SAMEORIGIN"
	// ValueXSSBlock is the X-XSS-Protection value redxt serves.
	ValueXSSBlock = "1; mode=block"
	// ValueReferrerPolicy is the Referrer-Policy value redxt serves.
	ValueReferrerPolicy = "strict-origin-when-cross-origin"
	// ValueCrossDomainNone is the X-Permitted-Cross-Domain-Policies value.
	ValueCrossDomainNone = "none"
	// ValueOriginAgentCluster opts the origin into agent clustering.
	ValueOriginAgentCluster = "?1"
	// ValueClearSiteData is the PART 11 Clear-Site-Data set. It omits
	// "executionContexts" on purpose: that directive breaks back
	// navigation, and operators who want hard isolation opt in.
	ValueClearSiteData = `"cache", "cookies", "storage"`
)

// Cross-origin isolation defaults from the PART 11 "Security Header
// Config" block. Isolation is off by default so ordinary embedding —
// CDN images, OG previews, third-party widgets — keeps working with no
// operator configuration.
const (
	// DefaultCOOP is the shipped Cross-Origin-Opener-Policy.
	DefaultCOOP = "unsafe-none"
	// DefaultCOEP is the shipped Cross-Origin-Embedder-Policy.
	DefaultCOEP = "unsafe-none"
	// DefaultCORP is the shipped Cross-Origin-Resource-Policy.
	DefaultCORP = "cross-origin"
)

// Reporting API lifetimes from the PART 11 header table, in seconds.
const (
	// ReportToMaxAge is the Report-To group lifetime, 126 days.
	ReportToMaxAge = 10886400
	// NELMaxAge is the Network Error Logging policy lifetime, 30 days.
	NELMaxAge = 2592000
)

// HSTS defaults from PART 11. Two years is the minimum the public
// preload list accepts; anything under a year makes the site
// preload-ineligible.
const (
	// DefaultHSTSMaxAge is the shipped max-age, two years in seconds.
	DefaultHSTSMaxAge = 63072000
)

// defaultRevokePaths lists the path suffixes whose responses revoke a
// session and therefore carry Clear-Site-Data. Handlers that revoke
// state on some other route call ClearSiteData directly.
var defaultRevokePaths = []string{"/logout", "/signout", "/revoke", "/account/delete"}

// HeadersOptions configures the PART 11 security header stage.
type HeadersOptions struct {
	// HSTS is the server.web.hsts.* block. Strict-Transport-Security is
	// emitted only when the request itself arrived over TLS: sending it
	// on a plaintext response is meaningless and RFC 6797 forbids it.
	HSTS config.HSTS

	// COOP, COEP and CORP override the cross-origin isolation defaults.
	// Empty values select DefaultCOOP, DefaultCOEP and DefaultCORP.
	COOP string
	COEP string
	CORP string

	// ContentSecurityPolicy is the full policy string. Empty selects the
	// PART 11 default policy with no learned origins.
	ContentSecurityPolicy string

	// CSPReportOnly emits the policy as Content-Security-Policy-Report-Only
	// instead of enforcing it, which is what development mode does so a
	// violation is logged rather than breaking the page.
	CSPReportOnly bool

	// PermissionsPolicy is the full Permissions-Policy string. Empty
	// selects the PART 11 default feature set.
	PermissionsPolicy string

	// ReportURL builds the absolute reporting endpoint for a request.
	// Nil selects a builder over the request's own scheme and host.
	ReportURL func(*http.Request) string

	// Revokes reports whether a response to this request revokes the
	// session and must therefore carry Clear-Site-Data. Nil selects the
	// default logout, signout, revoke and account-delete suffixes.
	Revokes func(*http.Request) bool
}

// DefaultHeadersOptions builds the header stage settings from
// server.yml, using the PART 11 default CSP and Permissions-Policy and
// the configured API base path for the reporting endpoints.
func DefaultHeadersOptions(cfg *config.Config) HeadersOptions {
	apiBase := cfg.APIBasePath()
	return HeadersOptions{
		HSTS:                  cfg.Server.Web.HSTS,
		ContentSecurityPolicy: DefaultCSP(apiBase, nil),
		PermissionsPolicy:     DefaultPermissionsPolicy(),
		ReportURL:             ReportURLFunc(apiBase),
	}
}

// SecurityHeaders returns the PART 11 security header middleware.
//
// Every header is written before the wrapped handler runs, because a
// handler that calls WriteHeader first would otherwise ship a response
// with none of them. The one header this stage does not own is
// X-Request-ID: urlvars.RequestIDMiddleware sets it at stage 2. This
// stage restores it from the request context if that stage was skipped,
// so the PART 11 guarantee that every response carries the header holds
// even in a partial chain.
func SecurityHeaders(opts HeadersOptions) Middleware {
	coop := firstNonEmpty(opts.COOP, DefaultCOOP)
	coep := firstNonEmpty(opts.COEP, DefaultCOEP)
	corp := firstNonEmpty(opts.CORP, DefaultCORP)
	csp := firstNonEmpty(opts.ContentSecurityPolicy, DefaultCSP("", nil))
	permissions := firstNonEmpty(opts.PermissionsPolicy, DefaultPermissionsPolicy())
	cspHeader := HeaderCSP
	if opts.CSPReportOnly {
		cspHeader = HeaderCSPReportOnly
	}
	reportURL := opts.ReportURL
	if reportURL == nil {
		reportURL = ReportURLFunc("")
	}
	revokes := opts.Revokes
	if revokes == nil {
		revokes = defaultRevokes
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			h := w.Header()
			h.Set(HeaderContentTypeOptions, ValueNoSniff)
			h.Set(HeaderFrameOptions, ValueSameOrigin)
			h.Set(HeaderXSSProtection, ValueXSSBlock)
			h.Set(HeaderReferrerPolicy, ValueReferrerPolicy)
			h.Set(HeaderPermittedCrossDomain, ValueCrossDomainNone)
			h.Set(HeaderOriginAgentCluster, ValueOriginAgentCluster)
			h.Set(HeaderCOOP, coop)
			h.Set(HeaderCOEP, coep)
			h.Set(HeaderCORP, corp)
			h.Set(cspHeader, csp)
			h.Set(HeaderPermissionsPolicy, permissions)

			endpoint := reportURL(req)
			h.Set(HeaderReportingEndpoints, `default="`+endpoint+`"`)
			h.Set(HeaderReportTo, `{"group":"default","max_age":`+strconv.Itoa(ReportToMaxAge)+`,"endpoints":[{"url":"`+endpoint+`"}]}`)
			h.Set(HeaderNEL, `{"report_to":"default","max_age":`+strconv.Itoa(NELMaxAge)+`,"include_subdomains":true}`)

			if h.Get(urlvars.HeaderRequestID) == "" {
				if id := urlvars.RequestIDFromContext(req.Context()); id != "" {
					h.Set(urlvars.HeaderRequestID, id)
				}
			}

			if value := hstsValue(opts.HSTS); value != "" && req.TLS != nil {
				h.Set(HeaderHSTS, value)
			}
			if revokes(req) {
				ClearSiteData(w)
			}

			next.ServeHTTP(w, req)
		})
	}
}

// ClearSiteData sets the PART 11 Clear-Site-Data header on a response
// that revokes a session: logout, account delete, consent withdrawal, or
// any other endpoint that invalidates the caller's local state.
func ClearSiteData(w http.ResponseWriter) {
	w.Header().Set(HeaderClearSiteData, ValueClearSiteData)
}

// defaultRevokes reports whether the request targets one of the
// well-known session-revoking endpoints.
func defaultRevokes(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	p := strings.ToLower(req.URL.Path)
	for _, suffix := range defaultRevokePaths {
		if p == suffix || strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

// hstsValue renders the Strict-Transport-Security value from config.
// A disabled block or a zero max-age produces no header: PART 11 makes
// max-age 0 the documented emergency disable, and emitting
// "max-age=0" would leave the directive present with no effect.
func hstsValue(cfg config.HSTS) string {
	if !cfg.Enabled {
		return ""
	}
	maxAge := cfg.MaxAge
	if maxAge < 0 {
		maxAge = DefaultHSTSMaxAge
	}
	if maxAge == 0 {
		return ""
	}
	value := "max-age=" + strconv.Itoa(maxAge)
	if cfg.IncludeSubdomains {
		value += "; includeSubDomains"
	}
	if cfg.Preload {
		value += "; preload"
	}
	return value
}

// ReportURLFunc returns a builder for the absolute Reporting API
// endpoint, "{scheme}://{host}{apiBase}/server/reports/default", derived
// from the request so a server reachable on several hostnames reports to
// the host the client actually used.
func ReportURLFunc(apiBase string) func(*http.Request) string {
	endpointPath := strings.TrimSuffix(apiBase, "/") + "/server/reports/default"
	return func(req *http.Request) string {
		if req == nil || req.Host == "" {
			return endpointPath
		}
		return requestScheme(req) + "://" + req.Host + endpointPath
	}
}

// cspDirectives holds the PART 11 default policy in its documented
// order. connect-src is the one directive that grows at runtime: the
// origins learned for CORS are appended to it, so a browser page served
// by redxt can call the same origins the API accepts.
var cspDirectives = []string{
	"default-src 'self'",
	"script-src 'self'",
	"style-src 'self' 'unsafe-inline'",
	"img-src 'self' data: blob: https:",
	"font-src 'self' https:",
	"media-src 'self' blob:",
	"worker-src 'self' blob:",
	"manifest-src 'self'",
	"frame-src 'self'",
	"frame-ancestors 'self'",
	"base-uri 'self'",
	"form-action 'self'",
	"object-src 'none'",
	"upgrade-insecure-requests",
	"report-to default",
}

// DefaultCSP renders the PART 11 default Content-Security-Policy.
//
// learnedOrigins are appended to connect-src; they come from the same
// CORS allow-list resolution the API uses, so the page and the API agree
// on which origins are ours. apiBase supplies the report-uri path for
// browsers that predate the Reporting API.
func DefaultCSP(apiBase string, learnedOrigins []string) string {
	connect := "connect-src 'self'"
	for _, origin := range learnedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" && origin != "*" {
			connect += " " + origin
		}
	}

	parts := make([]string, 0, len(cspDirectives)+2)
	for _, directive := range cspDirectives {
		parts = append(parts, directive)
		if directive == "font-src 'self' https:" {
			parts = append(parts, connect)
		}
	}
	if apiBase != "" {
		parts = append(parts, "report-uri "+strings.TrimSuffix(apiBase, "/")+"/server/reports/csp")
	}
	return strings.Join(parts, "; ")
}

// permissionsFeature is one Permissions-Policy entry. The defaults are
// kept as an ordered slice rather than a map so the rendered header is
// byte-identical on every start and across every cluster node.
type permissionsFeature struct {
	name  string
	value string
}

// defaultPermissionsFeatures is the PART 11 default feature set:
// hardware and sensor access locked, advertising and tracking proposals
// locked regardless of what a project asks for, and the features the
// spec itself uses scoped to the app's own origin.
var defaultPermissionsFeatures = []permissionsFeature{
	{name: "accelerometer", value: "()"},
	{name: "ambient-light-sensor", value: "()"},
	{name: "battery", value: "()"},
	{name: "camera", value: "()"},
	{name: "display-capture", value: "()"},
	{name: "geolocation", value: "()"},
	{name: "gyroscope", value: "()"},
	{name: "hid", value: "()"},
	{name: "idle-detection", value: "()"},
	{name: "magnetometer", value: "()"},
	{name: "microphone", value: "()"},
	{name: "midi", value: "()"},
	{name: "screen-wake-lock", value: "()"},
	{name: "serial", value: "()"},
	{name: "usb", value: "()"},
	{name: "xr-spatial-tracking", value: "()"},
	{name: "attribution-reporting", value: "()"},
	{name: "browsing-topics", value: "()"},
	{name: "interest-cohort", value: "()"},
	{name: "autoplay", value: "(self)"},
	{name: "encrypted-media", value: "(self)"},
	{name: "fullscreen", value: "(self)"},
	{name: "payment", value: "(self)"},
	{name: "picture-in-picture", value: "(self)"},
	{name: "publickey-credentials-get", value: "(self)"},
	{name: "storage-access", value: "(self)"},
	{name: "web-share", value: "(self)"},
}

// DefaultPermissionsPolicy renders the PART 11 default Permissions-Policy
// header by joining every configured feature into one
// "feature=value, feature=value" string.
func DefaultPermissionsPolicy() string {
	parts := make([]string, 0, len(defaultPermissionsFeatures))
	for _, feature := range defaultPermissionsFeatures {
		if feature.value == "" {
			continue
		}
		parts = append(parts, feature.name+"="+feature.value)
	}
	return strings.Join(parts, ", ")
}

// firstNonEmpty returns value when it holds a non-empty string and
// fallback otherwise.
func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
