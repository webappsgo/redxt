package urlvars

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/mode"
)

// isolateEnv blanks the environment variables the resolver falls back
// to, so no test depends on the machine it runs on.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DOMAIN", "")
	t.Setenv("HOSTNAME", "")
}

// baseOptions returns fully injected options: nothing is read from the
// host machine's hostname, environment, or network interfaces.
func baseOptions() Options {
	return Options{
		Listen:                "127.0.0.1",
		Port:                  8080,
		BaseURL:               "/base",
		Mode:                  mode.Production,
		ProjectName:           "redxt",
		Domain:                "example.com",
		Hostname:              "host.example.net",
		ShellHostname:         "shell.example.net",
		SkipPublicIPDetection: true,
		Learning:              LearningOptions{Enabled: Bool(false)},
	}
}

// newRequest builds a request with an explicit Host and TCP peer.
func newRequest(host, remoteAddr string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	req.Host = host
	req.RemoteAddr = remoteAddr
	req.TLS = nil
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req
}

const (
	trustedPeer   = "10.1.2.3:51000"
	untrustedPeer = "203.0.113.9:51000"
)

func TestURLVars(t *testing.T) {
	isolateEnv(t)

	tests := []struct {
		name      string
		mutate    func(*Options)
		host      string
		remote    string
		headers   map[string]string
		tls       bool
		wantProto string
		wantFQDN  string
		wantPort  string
	}{
		{
			name:      "forwarded proto host and port from trusted peer",
			host:      "internal:8080",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "proxy.example.org", HeaderForwardedPort: "8443"},
			wantProto: ProtoHTTPS,
			wantFQDN:  "proxy.example.org",
			wantPort:  "8443",
		},
		{
			name:      "https default port is stripped",
			host:      "internal:8080",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "proxy.example.org", HeaderForwardedPort: "443"},
			wantProto: ProtoHTTPS,
			wantFQDN:  "proxy.example.org",
			wantPort:  "",
		},
		{
			name:      "http default port is stripped",
			host:      "internal:8080",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedHost: "proxy.example.org", HeaderForwardedPort: "80"},
			wantProto: ProtoHTTP,
			wantFQDN:  "proxy.example.org",
			wantPort:  "",
		},
		{
			name:      "forwarded proto wins over forwarded ssl and url scheme",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedProto: "http", HeaderForwardedSsl: "on", HeaderURLScheme: "https"},
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name:      "forwarded ssl on means https",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedSsl: "on"},
			wantProto: ProtoHTTPS,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name:      "url scheme wins over front end https",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderURLScheme: "http", HeaderFrontEndHTTPS: "on"},
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name:      "front end https on means https",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderFrontEndHTTPS: "on"},
			wantProto: ProtoHTTPS,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name:      "tls connection means https",
			host:      "internal",
			remote:    untrustedPeer,
			tls:       true,
			wantProto: ProtoHTTPS,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name:      "untrusted peer headers are ignored",
			host:      "internal",
			remote:    untrustedPeer,
			headers:   map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "attacker.example.org", HeaderForwardedPort: "9999"},
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name:      "forwarded host chain uses leftmost entry",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedHost: "first.example.org, second.example.org"},
			wantProto: ProtoHTTP,
			wantFQDN:  "first.example.org",
			wantPort:  "8080",
		},
		{
			name:      "real host is used when forwarded host is absent",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderRealHost: "real.example.org", HeaderOriginalHost: "original.example.org"},
			wantProto: ProtoHTTP,
			wantFQDN:  "real.example.org",
			wantPort:  "8080",
		},
		{
			name:      "original host is the last forwarded host fallback",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderOriginalHost: "original.example.org"},
			wantProto: ProtoHTTP,
			wantFQDN:  "original.example.org",
			wantPort:  "8080",
		},
		{
			name:      "port embedded in forwarded host is used",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedHost: "proxy.example.org:8443"},
			wantProto: ProtoHTTP,
			wantFQDN:  "proxy.example.org",
			wantPort:  "8443",
		},
		{
			name:      "real port is the forwarded port fallback",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderRealPort: "9443"},
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "9443",
		},
		{
			name:      "host header port beats the listen port",
			host:      "client.example.org:9000",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "9000",
		},
		{
			name:      "ipv6 host header port is split correctly",
			host:      "[2001:db8::1]:8443",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "8443",
		},
		{
			name:      "invalid forwarded port falls through to the listen port",
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedPort: "not-a-port"},
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "8080",
		},
		{
			name: "onion host bypasses every proxy header",
			mutate: func(o *Options) {
				o.OnionAddress = "abcdefghij234567.onion"
			},
			host:      "abcdefghij234567.onion",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "clearnet.example.org", HeaderForwardedPort: "8443"},
			wantProto: ProtoHTTP,
			wantFQDN:  "abcdefghij234567.onion",
			wantPort:  "",
		},
		{
			name: "onion host arriving through a proxy header still forces http",
			mutate: func(o *Options) {
				o.OnionAddress = "abcdefghij234567.onion"
			},
			host:      "internal",
			remote:    trustedPeer,
			headers:   map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "abcdefghij234567.onion"},
			wantProto: ProtoHTTP,
			wantFQDN:  "abcdefghij234567.onion",
			wantPort:  "",
		},
		{
			name: "domain env first entry wins over hostname",
			mutate: func(o *Options) {
				o.Domain = "primary.example.com, secondary.example.com"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "primary.example.com",
			wantPort:  "8080",
		},
		{
			name: "invalid domain entries are skipped",
			mutate: func(o *Options) {
				o.Domain = "localhost,192.0.2.5,good.example.com"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "good.example.com",
			wantPort:  "8080",
		},
		{
			name: "hostname is used when domain is unset",
			mutate: func(o *Options) {
				o.Domain = ""
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "host.example.net",
			wantPort:  "8080",
		},
		{
			name: "shell hostname is used when hostname is invalid",
			mutate: func(o *Options) {
				o.Domain = ""
				o.Hostname = "singlelabel"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "shell.example.net",
			wantPort:  "8080",
		},
		{
			name: "public ipv6 is preferred over public ipv4",
			mutate: func(o *Options) {
				o.Domain = ""
				o.Hostname = "singlelabel"
				o.ShellHostname = "alsosinglelabel"
				o.PublicIPv4 = "203.0.113.7"
				o.PublicIPv6 = "2001:db8::1"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "2001:db8::1",
			wantPort:  "8080",
		},
		{
			name: "public ipv4 is used when no public ipv6 exists",
			mutate: func(o *Options) {
				o.Domain = ""
				o.Hostname = "singlelabel"
				o.ShellHostname = "alsosinglelabel"
				o.PublicIPv4 = "203.0.113.7"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "203.0.113.7",
			wantPort:  "8080",
		},
		{
			name: "localhost is the last resort",
			mutate: func(o *Options) {
				o.Domain = ""
				o.Hostname = "singlelabel"
				o.ShellHostname = "alsosinglelabel"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  FallbackFQDN,
			wantPort:  "8080",
		},
		{
			name: "development mode accepts a dev tld hostname",
			mutate: func(o *Options) {
				o.Mode = mode.Development
				o.Domain = "box.local"
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "box.local",
			wantPort:  "8080",
		},
		{
			name: "no listen port falls back to the stripped proto default",
			mutate: func(o *Options) {
				o.Port = 0
			},
			host:      "internal",
			remote:    untrustedPeer,
			wantProto: ProtoHTTP,
			wantFQDN:  "example.com",
			wantPort:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := baseOptions()
			if tc.mutate != nil {
				tc.mutate(&opts)
			}
			r := New(opts)

			req := newRequest(tc.host, tc.remote, tc.headers)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}

			proto, fqdn, port := r.URLVars(req)
			if proto != tc.wantProto || fqdn != tc.wantFQDN || port != tc.wantPort {
				t.Fatalf("URLVars = (%q, %q, %q), want (%q, %q, %q)",
					proto, fqdn, port, tc.wantProto, tc.wantFQDN, tc.wantPort)
			}
		})
	}
}

func TestURLVarsNilRequest(t *testing.T) {
	isolateEnv(t)

	r := New(baseOptions())
	proto, fqdn, port := r.URLVars(nil)
	if proto != ProtoHTTP || fqdn != "example.com" || port != "" {
		t.Fatalf("URLVars(nil) = (%q, %q, %q), want (http, example.com, )", proto, fqdn, port)
	}
}

func TestBuildURL(t *testing.T) {
	isolateEnv(t)

	tests := []struct {
		name    string
		mutate  func(*Options)
		host    string
		remote  string
		headers map[string]string
		path    string
		want    string
	}{
		{
			name:    "non standard port is included",
			host:    "internal",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedHost: "api.example.com", HeaderForwardedPort: "8080"},
			path:    "/api/v1/status",
			want:    "http://api.example.com:8080/api/v1/status",
		},
		{
			name:    "https on 443 omits the port",
			host:    "internal",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "api.example.com", HeaderForwardedPort: "443"},
			path:    "/api/v1/status",
			want:    "https://api.example.com/api/v1/status",
		},
		{
			name:    "empty path yields the bare origin",
			host:    "internal",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "api.example.com", HeaderForwardedPort: "443"},
			path:    "",
			want:    "https://api.example.com",
		},
		{
			name:    "missing leading slash is added",
			host:    "internal",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedProto: "https", HeaderForwardedHost: "api.example.com", HeaderForwardedPort: "443"},
			path:    "healthz",
			want:    "https://api.example.com/healthz",
		},
		{
			name: "ipv6 fqdn is bracketed",
			mutate: func(o *Options) {
				o.Domain = ""
				o.Hostname = "singlelabel"
				o.ShellHostname = "alsosinglelabel"
				o.PublicIPv6 = "2001:db8::1"
			},
			host:   "internal",
			remote: untrustedPeer,
			path:   "/healthz",
			want:   "http://[2001:db8::1]:8080/healthz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := baseOptions()
			if tc.mutate != nil {
				tc.mutate(&opts)
			}
			r := New(opts)

			got := r.BuildURL(newRequest(tc.host, tc.remote, tc.headers), tc.path)
			if got != tc.want {
				t.Fatalf("BuildURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBaseURLPrefix(t *testing.T) {
	isolateEnv(t)

	tests := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{
			name:    "forwarded prefix wins",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedPrefix: "/app/", HeaderForwardedPath: "/other", HeaderScriptName: "/cgi"},
			want:    "/app",
		},
		{
			name:    "forwarded path is the first fallback",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedPath: "other", HeaderScriptName: "/cgi"},
			want:    "/other",
		},
		{
			name:    "script name is the last header fallback",
			remote:  trustedPeer,
			headers: map[string]string{HeaderScriptName: "/cgi"},
			want:    "/cgi",
		},
		{
			name:    "untrusted peer falls back to the configured baseurl",
			remote:  untrustedPeer,
			headers: map[string]string{HeaderForwardedPrefix: "/app"},
			want:    "/base",
		},
		{
			name:   "no headers falls back to the configured baseurl",
			remote: trustedPeer,
			want:   "/base",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := New(baseOptions())
			if got := r.BaseURLPrefix(newRequest("internal", tc.remote, tc.headers)); got != tc.want {
				t.Fatalf("BaseURLPrefix = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBaseURLPrefixNilRequest(t *testing.T) {
	isolateEnv(t)

	opts := baseOptions()
	opts.BaseURL = ""
	if got := New(opts).BaseURLPrefix(nil); got != "/" {
		t.Fatalf("BaseURLPrefix(nil) = %q, want /", got)
	}
}

func TestClientIP(t *testing.T) {
	isolateEnv(t)

	tests := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{
			name:    "cloudflare header wins",
			remote:  trustedPeer,
			headers: map[string]string{HeaderCFConnectingIP: "198.51.100.5", HeaderTrueClientIP: "198.51.100.6", HeaderRealIP: "198.51.100.7"},
			want:    "198.51.100.5",
		},
		{
			name:    "true client ip is the second source",
			remote:  trustedPeer,
			headers: map[string]string{HeaderTrueClientIP: "198.51.100.6", HeaderRealIP: "198.51.100.7"},
			want:    "198.51.100.6",
		},
		{
			name:    "real ip beats the forwarded for chain",
			remote:  trustedPeer,
			headers: map[string]string{HeaderRealIP: "198.51.100.7", HeaderForwardedFor: "198.51.100.8, 10.0.0.1"},
			want:    "198.51.100.7",
		},
		{
			name:    "forwarded for uses the leftmost entry",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedFor: "198.51.100.8, 10.0.0.1, 10.0.0.2"},
			want:    "198.51.100.8",
		},
		{
			name:    "client ip is the last header source",
			remote:  trustedPeer,
			headers: map[string]string{HeaderClientIP: "198.51.100.9"},
			want:    "198.51.100.9",
		},
		{
			name:    "malformed header values are skipped",
			remote:  trustedPeer,
			headers: map[string]string{HeaderRealIP: "not-an-ip", HeaderForwardedFor: "also-not-an-ip", HeaderClientIP: "198.51.100.9"},
			want:    "198.51.100.9",
		},
		{
			name:    "untrusted peer ignores every header",
			remote:  untrustedPeer,
			headers: map[string]string{HeaderCFConnectingIP: "198.51.100.5", HeaderForwardedFor: "198.51.100.8"},
			want:    "203.0.113.9",
		},
		{
			name:   "ipv6 peer is split correctly",
			remote: "[2001:db8::99]:51000",
			want:   "2001:db8::99",
		},
		{
			name:    "forwarded for entry with a port is accepted",
			remote:  trustedPeer,
			headers: map[string]string{HeaderForwardedFor: "198.51.100.8:1234"},
			want:    "198.51.100.8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := New(baseOptions())
			if got := r.ClientIP(newRequest("internal", tc.remote, tc.headers)); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIPNilRequest(t *testing.T) {
	isolateEnv(t)

	if got := New(baseOptions()).ClientIP(nil); got != "" {
		t.Fatalf("ClientIP(nil) = %q, want empty", got)
	}
}

func TestIsTrustedProxy(t *testing.T) {
	isolateEnv(t)

	opts := baseOptions()
	opts.Listen = "203.0.113.5"
	opts.TrustedProxies = config.TrustedProxies{Additional: []string{"198.51.100.10", "192.0.2.0/24", "  "}}
	r := New(opts)

	tests := []struct {
		name  string
		addr  string
		trust bool
	}{
		{name: "loopback", addr: "127.0.0.1:51000", trust: true},
		{name: "rfc1918 ten", addr: "10.1.2.3:51000", trust: true},
		{name: "rfc1918 172.16", addr: "172.16.0.1:51000", trust: true},
		{name: "outside 172.16/12", addr: "172.32.0.1:51000", trust: false},
		{name: "rfc1918 192.168", addr: "192.168.1.1", trust: true},
		{name: "ipv4 link local", addr: "169.254.1.1:51000", trust: true},
		{name: "ipv6 loopback", addr: "[::1]:51000", trust: true},
		{name: "ipv6 unique local", addr: "[fd00::1]:51000", trust: true},
		{name: "ipv6 link local with zone", addr: "[fe80::1%eth0]:51000", trust: true},
		{name: "public peer", addr: "8.8.8.8:51000", trust: false},
		{name: "configured single ip", addr: "198.51.100.10:51000", trust: true},
		{name: "configured cidr", addr: "192.0.2.77:51000", trust: true},
		{name: "outside configured cidr", addr: "192.0.3.77:51000", trust: false},
		{name: "listen address slash 24", addr: "203.0.113.99:51000", trust: true},
		{name: "outside listen slash 24", addr: "203.0.114.99:51000", trust: false},
		{name: "empty", addr: "", trust: false},
		{name: "garbage", addr: "not-an-address", trust: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.IsTrustedProxy(tc.addr); got != tc.trust {
				t.Fatalf("IsTrustedProxy(%q) = %v, want %v", tc.addr, got, tc.trust)
			}
		})
	}
}

func TestIsTrustedProxyResolvesDNSNames(t *testing.T) {
	isolateEnv(t)

	opts := baseOptions()
	opts.TrustedProxies = config.TrustedProxies{Additional: []string{"lb.internal.example"}}
	opts.ResolveHost = func(name string) ([]net.IP, error) {
		if name != "lb.internal.example" {
			return nil, errors.New("unexpected lookup")
		}
		return []net.IP{net.ParseIP("198.51.100.42")}, nil
	}
	r := New(opts)

	if !r.IsTrustedProxy("198.51.100.42:51000") {
		t.Fatal("resolved proxy name should be trusted")
	}
	if r.IsTrustedProxy("198.51.100.43:51000") {
		t.Fatal("unrelated address should not be trusted")
	}

	r.RefreshTrustedProxies()
	if !r.IsTrustedProxy("198.51.100.42:51000") {
		t.Fatal("resolved proxy name should stay trusted after refresh")
	}
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		in       string
		wantHost string
		wantPort string
	}{
		{in: "example.com:8080", wantHost: "example.com", wantPort: "8080"},
		{in: "example.com", wantHost: "example.com", wantPort: ""},
		{in: "[2001:db8::1]:8443", wantHost: "2001:db8::1", wantPort: "8443"},
		{in: "[2001:db8::1]", wantHost: "2001:db8::1", wantPort: ""},
		{in: "2001:db8::1", wantHost: "2001:db8::1", wantPort: ""},
		{in: "", wantHost: "", wantPort: ""},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			host, port := splitHostPort(tc.in)
			if host != tc.wantHost || port != tc.wantPort {
				t.Fatalf("splitHostPort(%q) = (%q, %q), want (%q, %q)", tc.in, host, port, tc.wantHost, tc.wantPort)
			}
		})
	}
}
