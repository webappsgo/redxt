// Package urlvars implements AI.md PART 8 ("URL & FQDN Detection"): the
// per-request resolution of the {proto}, {fqdn}, {port} and {baseurl}
// template variables, the client-IP and request-ID extraction chains, the
// auth-token header priority order, and the FQDN validation rules used by
// both host checks and Let's Encrypt eligibility.
//
// Nothing in this package hardcodes a host, IP, or port: every value is
// derived from the incoming request, the process environment, the resolved
// configuration, or the machine's own interfaces. Reverse-proxy headers are
// only honored when the immediate TCP peer passes the trusted_proxies gate
// documented in AI.md PART 12 ("Trusted Proxies").
package urlvars

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/mode"
)

// Reverse-proxy headers honored for {proto}, {fqdn}, {port} and {baseurl}
// resolution. Every one of these is gated on the trusted-proxy check.
const (
	// HeaderForwardedProto is the standard scheme header ("https"/"http").
	HeaderForwardedProto = "X-Forwarded-Proto"
	// HeaderForwardedSsl is the "on"/"off" scheme header.
	HeaderForwardedSsl = "X-Forwarded-Ssl"
	// HeaderURLScheme is the alternative scheme header.
	HeaderURLScheme = "X-Url-Scheme"
	// HeaderFrontEndHTTPS is the Microsoft "on" scheme header.
	HeaderFrontEndHTTPS = "Front-End-Https"
	// HeaderForwardedHost is the standard forwarded host header.
	HeaderForwardedHost = "X-Forwarded-Host"
	// HeaderRealHost is the nginx forwarded host header.
	HeaderRealHost = "X-Real-Host"
	// HeaderOriginalHost is the alternative forwarded host header.
	HeaderOriginalHost = "X-Original-Host"
	// HeaderForwardedPort is the standard forwarded port header.
	HeaderForwardedPort = "X-Forwarded-Port"
	// HeaderRealPort is the nginx forwarded port header.
	HeaderRealPort = "X-Real-Port"
	// HeaderForwardedPrefix is the standard base-path header.
	HeaderForwardedPrefix = "X-Forwarded-Prefix"
	// HeaderForwardedPath is the fallback base-path header.
	HeaderForwardedPath = "X-Forwarded-Path"
	// HeaderScriptName is the CGI-style base-path header.
	HeaderScriptName = "X-Script-Name"
)

// Client-IP headers, in the PART 8 priority order. All are gated on the
// trusted-proxy check; r.RemoteAddr is the ungated fallback.
const (
	// HeaderCFConnectingIP is the Cloudflare client IP header.
	HeaderCFConnectingIP = "CF-Connecting-IP"
	// HeaderTrueClientIP is the Akamai / Cloudflare Enterprise header.
	HeaderTrueClientIP = "True-Client-IP"
	// HeaderRealIP is the nginx client IP header.
	HeaderRealIP = "X-Real-IP"
	// HeaderForwardedFor is the standard "client, proxy1, proxy2" chain.
	HeaderForwardedFor = "X-Forwarded-For"
	// HeaderClientIP is the alternative single-IP header.
	HeaderClientIP = "X-Client-IP"
)

const (
	// ProtoHTTP is the default scheme when nothing else is detected.
	ProtoHTTP = "http"
	// ProtoHTTPS is the scheme used whenever TLS is detected.
	ProtoHTTPS = "https"
	// FallbackFQDN is the last-resort host when no other source resolves.
	FallbackFQDN = "localhost"
)

// LearningOptions mirrors server.url_detection.* from AI.md PART 8.
//
// The three switches are pointers because their documented defaults are
// true: a nil pointer selects the default, a non-nil pointer selects the
// caller's explicit value. Use Bool to build one.
type LearningOptions struct {
	// Enabled turns smart domain learning on. Defaults to true.
	Enabled *bool
	// MinSamples is the number of distinct hosts sharing a base domain
	// required before a wildcard is inferred. Defaults to 3.
	MinSamples int
	// SampleWindow is the sliding window pattern analysis runs over.
	// Defaults to 5m.
	SampleWindow time.Duration
	// LogChanges emits a Logf line on every domain change. Defaults to
	// true.
	LogChanges *bool
	// LiveReload allows the resolved values to change without a restart.
	// Defaults to true. When false, the first resolved FQDN is pinned.
	LiveReload *bool
}

// Bool returns a pointer to v, for setting the LearningOptions switches
// explicitly instead of accepting their defaults.
func Bool(v bool) *bool {
	return &v
}

// Options configures a Resolver. Every environment- or network-derived
// input can be injected so the resolver is fully deterministic in tests;
// a zero field falls back to the live source named in its comment.
type Options struct {
	// Listen is the server's own bind address (config.Server.Listen).
	Listen string
	// Port is the server's own listen port (config.Server.Port).
	Port int
	// BaseURL is the configured path prefix (config.Server.BaseURL).
	BaseURL string
	// TrustedProxies is the configured proxy allow-list; the private and
	// loopback ranges are always trusted on top of it.
	TrustedProxies config.TrustedProxies
	// OnionAddress is tor.onion_address; empty disables Tor detection.
	OnionAddress string
	// Mode is the resolved application mode. Anything other than
	// production is treated as development for host validation.
	Mode mode.Mode
	// ProjectName enables the dynamic project TLD (e.g. "app.redxt").
	ProjectName string
	// Domain is the DOMAIN value, a comma-separated list whose first
	// valid entry is primary. Falls back to config.Domain().
	Domain string
	// Hostname is the value os.Hostname() would return. Falls back to
	// os.Hostname().
	Hostname string
	// ShellHostname is the $HOSTNAME fallback. Falls back to
	// os.Getenv("HOSTNAME").
	ShellHostname string
	// PublicIPv4 is the machine's first public IPv4. Falls back to
	// interface enumeration.
	PublicIPv4 string
	// PublicIPv6 is the machine's first public IPv6. Falls back to
	// interface enumeration.
	PublicIPv6 string
	// SkipPublicIPDetection disables interface enumeration for the public
	// IP fallbacks, keeping the resolver off the network and relying
	// solely on PublicIPv4/PublicIPv6.
	SkipPublicIPDetection bool
	// Learning configures smart FQDN detection.
	Learning LearningOptions
	// OnChange is called whenever the resolved FQDN changes, with the
	// previous and the new value. The previous value is empty on the
	// first resolution.
	OnChange func(oldFQDN, newFQDN string)
	// Logf receives human-readable domain-learning notices. It exists so
	// this package never imports the logging package.
	Logf func(format string, args ...any)
	// ResolveHost resolves a DNS name listed in TrustedProxies.Additional.
	// Falls back to net.LookupIP.
	ResolveHost func(name string) ([]net.IP, error)
	// Now supplies the clock used by the sliding sample window. Falls
	// back to time.Now.
	Now func() time.Time
}

// Resolver resolves URL variables from requests. It is safe for
// concurrent use: domain learning mutates state on every request.
type Resolver struct {
	mu sync.RWMutex

	listen      string
	listenPort  string
	baseURL     string
	onion       string
	appMode     mode.Mode
	projectName string

	domains       []string
	hostname      string
	shellHostname string
	publicIPv4    string
	publicIPv6    string

	additional []string
	trusted    []*net.IPNet
	resolveIP  func(name string) ([]net.IP, error)

	learning     bool
	minSamples   int
	sampleWindow time.Duration
	logChanges   bool
	liveReload   bool

	samples  []observation
	lastFQDN string
	pinned   string

	onChange func(oldFQDN, newFQDN string)
	logf     func(format string, args ...any)
	now      func() time.Time
}

// New builds a Resolver from opts, applying every documented default.
func New(opts Options) *Resolver {
	r := &Resolver{
		listen:        strings.TrimSpace(opts.Listen),
		baseURL:       config.NormalizeBaseURL(opts.BaseURL),
		onion:         strings.ToLower(strings.TrimSpace(opts.OnionAddress)),
		appMode:       opts.Mode,
		projectName:   strings.TrimSpace(opts.ProjectName),
		hostname:      strings.TrimSpace(opts.Hostname),
		shellHostname: strings.TrimSpace(opts.ShellHostname),
		publicIPv4:    strings.TrimSpace(opts.PublicIPv4),
		publicIPv6:    strings.TrimSpace(opts.PublicIPv6),
		additional:    opts.TrustedProxies.Additional,
		resolveIP:     opts.ResolveHost,
		minSamples:    opts.Learning.MinSamples,
		sampleWindow:  opts.Learning.SampleWindow,
		onChange:      opts.OnChange,
		logf:          opts.Logf,
		now:           opts.Now,
	}

	if r.appMode == "" {
		r.appMode = mode.Production
	}
	if opts.Port > 0 && opts.Port <= 65535 {
		r.listenPort = strconv.Itoa(opts.Port)
	}
	if r.resolveIP == nil {
		r.resolveIP = net.LookupIP
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.minSamples <= 0 {
		r.minSamples = 3
	}
	if r.sampleWindow <= 0 {
		r.sampleWindow = 5 * time.Minute
	}
	r.learning = boolOr(opts.Learning.Enabled, true)
	r.logChanges = boolOr(opts.Learning.LogChanges, true)
	r.liveReload = boolOr(opts.Learning.LiveReload, true)

	domain := opts.Domain
	if strings.TrimSpace(domain) == "" {
		domain = config.Domain()
	}
	r.domains = splitDomains(domain)

	if r.hostname == "" {
		if h, err := os.Hostname(); err == nil {
			r.hostname = strings.TrimSpace(h)
		}
	}
	if r.shellHostname == "" {
		r.shellHostname = strings.TrimSpace(os.Getenv("HOSTNAME"))
	}
	if !opts.SkipPublicIPDetection && r.publicIPv4 == "" && r.publicIPv6 == "" {
		r.publicIPv4, r.publicIPv6 = PublicIPs()
	}

	r.trusted = r.buildTrusted()
	return r
}

// boolOr returns *p when p is set and def otherwise.
func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// splitDomains splits a comma-separated DOMAIN value into trimmed,
// lowercased entries, dropping empties.
func splitDomains(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// devMode reports whether host validation runs with development rules.
// Every mode other than production relaxes validation, so debug mode
// behaves like development here.
func (r *Resolver) devMode() bool {
	return r.appMode != mode.Production
}

// RefreshTrustedProxies re-resolves the DNS names in the configured
// trusted-proxy allow-list. AI.md PART 12 refreshes those names every
// five minutes; the caller owns the schedule.
func (r *Resolver) RefreshTrustedProxies() {
	nets := r.buildTrusted()
	r.mu.Lock()
	r.trusted = nets
	r.mu.Unlock()
}

// URLVars returns the resolved {proto}, {fqdn} and {port} for req,
// following the PART 8 priority tables. Port is always empty for the
// scheme's default port (80 for http, 443 for https).
func (r *Resolver) URLVars(req *http.Request) (proto, fqdn, port string) {
	if req == nil {
		return ProtoHTTP, r.staticFQDN(), ""
	}

	reqHost, reqPort := splitHostPort(req.Host)

	// Priority 0: a Tor request bypasses the proxy-header gate entirely.
	if r.isOnion(reqHost) {
		return ProtoHTTP, r.onion, ""
	}

	trusted := r.IsTrustedProxy(req.RemoteAddr)

	proto = r.resolveProto(req, trusted)

	fromProxy := false
	hostPort := reqPort
	if trusted {
		if h, p, ok := r.proxyHost(req); ok {
			fqdn = h
			fromProxy = true
			if p != "" {
				hostPort = p
			}
		}
	}
	if fqdn == "" {
		fqdn = r.staticFQDN()
	}

	if r.isOnion(fqdn) {
		return ProtoHTTP, r.onion, ""
	}

	fqdn = r.observe(fqdn, fromProxy)
	port = normalizePort(proto, r.resolvePort(req, trusted, hostPort))
	return proto, fqdn, port
}

// resolveProto implements the {proto} priority table: forwarded scheme
// headers (trusted peers only), then the connection's own TLS state,
// then the http default.
func (r *Resolver) resolveProto(req *http.Request, trusted bool) string {
	if trusted {
		if v := firstValue(req.Header.Get(HeaderForwardedProto)); v != "" {
			if s := normalizeScheme(v); s != "" {
				return s
			}
		}
		if isOn(req.Header.Get(HeaderForwardedSsl)) {
			return ProtoHTTPS
		}
		if v := firstValue(req.Header.Get(HeaderURLScheme)); v != "" {
			if s := normalizeScheme(v); s != "" {
				return s
			}
		}
		if isOn(req.Header.Get(HeaderFrontEndHTTPS)) {
			return ProtoHTTPS
		}
	}
	if req.TLS != nil {
		return ProtoHTTPS
	}
	return ProtoHTTP
}

// proxyHost returns the forwarded host and any port embedded in it.
func (r *Resolver) proxyHost(req *http.Request) (host, port string, ok bool) {
	for _, name := range []string{HeaderForwardedHost, HeaderRealHost, HeaderOriginalHost} {
		raw := firstValue(req.Header.Get(name))
		if raw == "" {
			continue
		}
		h, p := splitHostPort(raw)
		if !plausibleHost(h) {
			continue
		}
		return strings.ToLower(h), p, true
	}
	return "", "", false
}

// staticFQDN resolves the non-header {fqdn} sources: the DOMAIN list,
// os.Hostname(), $HOSTNAME, the first public IPv6, the first public
// IPv4, then localhost.
func (r *Resolver) staticFQDN() string {
	dev := r.devMode()
	for _, d := range r.domains {
		if IsValidHost(d, dev, r.projectName) {
			return d
		}
	}
	if IsValidHost(r.hostname, dev, r.projectName) {
		return strings.ToLower(r.hostname)
	}
	if IsValidHost(r.shellHostname, dev, r.projectName) {
		return strings.ToLower(r.shellHostname)
	}
	if r.publicIPv6 != "" {
		return r.publicIPv6
	}
	if r.publicIPv4 != "" {
		return r.publicIPv4
	}
	return FallbackFQDN
}

// resolvePort implements the {port} priority table: forwarded port
// headers (trusted peers only), the port carried by the resolved host,
// then the server's own listen port.
func (r *Resolver) resolvePort(req *http.Request, trusted bool, hostPort string) string {
	if trusted {
		for _, name := range []string{HeaderForwardedPort, HeaderRealPort} {
			if p := validPort(firstValue(req.Header.Get(name))); p != "" {
				return p
			}
		}
	}
	if p := validPort(hostPort); p != "" {
		return p
	}
	return r.listenPort
}

// isOnion reports whether host is the configured Tor hidden service.
func (r *Resolver) isOnion(host string) bool {
	if r.onion == "" || host == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(host, "."), r.onion)
}

// BuildURL returns the absolute URL for path, using the request's
// resolved variables. The default ports 80 and 443 are never emitted.
func (r *Resolver) BuildURL(req *http.Request, path string) string {
	proto, fqdn, port := r.URLVars(req)
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	host := bracketIPv6(fqdn)
	if port == "" {
		return fmt.Sprintf("%s://%s%s", proto, host, path)
	}
	return fmt.Sprintf("%s://%s:%s%s", proto, host, port, path)
}

// BaseURLPrefix returns the normalized {baseurl} path prefix: the
// forwarded base-path headers from a trusted peer, else the configured
// server.baseurl.
func (r *Resolver) BaseURLPrefix(req *http.Request) string {
	if req != nil && r.IsTrustedProxy(req.RemoteAddr) {
		for _, name := range []string{HeaderForwardedPrefix, HeaderForwardedPath, HeaderScriptName} {
			if v := strings.TrimSpace(req.Header.Get(name)); v != "" {
				return config.NormalizeBaseURL(v)
			}
		}
	}
	return r.baseURL
}

// ClientIP returns the end client's IP address for access logs, rate
// limiting, blocklists and GeoIP. Proxy headers are honored only for a
// trusted immediate peer; otherwise the TCP peer's own address is used.
// The result is never used for trust decisions.
func (r *Resolver) ClientIP(req *http.Request) string {
	if req == nil {
		return ""
	}
	if r.IsTrustedProxy(req.RemoteAddr) {
		for _, name := range []string{HeaderCFConnectingIP, HeaderTrueClientIP, HeaderRealIP} {
			if ip := parseIPValue(req.Header.Get(name)); ip != "" {
				return ip
			}
		}
		if ip := parseIPValue(firstValue(req.Header.Get(HeaderForwardedFor))); ip != "" {
			return ip
		}
		if ip := parseIPValue(req.Header.Get(HeaderClientIP)); ip != "" {
			return ip
		}
	}
	host, _ := splitHostPort(req.RemoteAddr)
	return strings.TrimSuffix(host, ".")
}

// IsTrustedProxy reports whether remoteAddr — an "ip:port" pair or a
// bare IP — is an immediate peer whose forwarded headers may be
// honored, per AI.md PART 12 "Trusted Proxies".
func (r *Resolver) IsTrustedProxy(remoteAddr string) bool {
	host, _ := splitHostPort(remoteAddr)
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}

	r.mu.RLock()
	nets := r.trusted
	r.mu.RUnlock()

	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// alwaysTrusted lists the ranges trusted with no configuration at all:
// loopback, RFC 1918, RFC 4193 unique-local, and link-local.
var alwaysTrusted = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

// buildTrusted expands the always-trusted ranges, the listen address's
// own /24 (the reverse-proxy sidecar pattern), and every configured
// additional IP, CIDR, or DNS name into networks.
func (r *Resolver) buildTrusted() []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(alwaysTrusted)+len(r.additional)+1)
	for _, cidr := range alwaysTrusted {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			nets = append(nets, n)
		}
	}
	if n := listenSubnet(r.listen); n != nil {
		nets = append(nets, n)
	}
	for _, entry := range r.additional {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, n)
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			if n := hostNet(ip); n != nil {
				nets = append(nets, n)
			}
			continue
		}
		ips, err := r.resolveIP(entry)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if n := hostNet(ip); n != nil {
				nets = append(nets, n)
			}
		}
	}
	return nets
}

// listenSubnet returns the /24 surrounding an IPv4 listen address, so a
// reverse-proxy sidecar on the same container network is trusted.
func listenSubnet(listen string) *net.IPNet {
	host, _ := splitHostPort(listen)
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return nil
	}
	v4 := ip.To4()
	if v4 == nil || v4.IsUnspecified() {
		return nil
	}
	return &net.IPNet{IP: v4.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}
}

// hostNet wraps a single IP as a /32 or /128 network.
func hostNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	if v6 := ip.To16(); v6 != nil {
		return &net.IPNet{IP: v6, Mask: net.CIDRMask(128, 128)}
	}
	return nil
}

// splitHostPort splits "host:port", "[v6]:port", a bare host, or a bare
// bracketed IPv6 literal, returning the port as "" when absent.
func splitHostPort(hostport string) (host, port string) {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return "", ""
	}
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return strings.TrimSpace(h), strings.TrimSpace(p)
	}
	if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]"), ""
	}
	return hostport, ""
}

// bracketIPv6 wraps a bare IPv6 literal in brackets so it is usable in
// a URL authority.
func bracketIPv6(host string) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]"
	}
	return host
}

// firstValue returns the first entry of a comma-separated header value,
// which is the client-side entry for chained proxy headers.
func firstValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, ","); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}

// normalizeScheme returns "http" or "https" for a scheme header value,
// or "" when the value is neither.
func normalizeScheme(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ProtoHTTP:
		return ProtoHTTP
	case ProtoHTTPS:
		return ProtoHTTPS
	default:
		return ""
	}
}

// isOn reports whether an "on"/"off" style header is on.
func isOn(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "on")
}

// validPort returns raw when it is a usable TCP port, else "".
func validPort(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > 65535 {
		return ""
	}
	return strconv.Itoa(n)
}

// normalizePort strips the scheme's default port, which never appears
// in a URL built by this package.
func normalizePort(proto, port string) string {
	if (proto == ProtoHTTP && port == "80") || (proto == ProtoHTTPS && port == "443") {
		return ""
	}
	return port
}

// plausibleHost rejects header values that cannot be a hostname before
// they reach URL construction.
func plausibleHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 253 {
		return false
	}
	for _, c := range host {
		if c <= ' ' || c == 0x7f {
			return false
		}
		if strings.ContainsRune("/\\?#@\"'<>", c) {
			return false
		}
	}
	return true
}

// parseIPValue returns raw when it parses as an IP address, else "".
func parseIPValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		raw = h
	}
	raw = strings.Trim(raw, "[]")
	ip := net.ParseIP(raw)
	if ip == nil {
		return ""
	}
	return ip.String()
}
