package urlvars

import "testing"

func TestIsValidHost(t *testing.T) {
	const projectName = "jokes"

	tests := []struct {
		host    string
		project bool
		dev     bool
		reason  string
	}{
		{host: "my.server.domain.co.uk", project: true, dev: true, reason: "valid eTLD+1 domain.co.uk"},
		{host: "api.example.com", project: true, dev: true, reason: "valid eTLD+1 example.com"},
		{host: "app.company.com.au", project: true, dev: true, reason: "valid eTLD+1 company.com.au"},
		{host: "server.company.io", project: true, dev: true, reason: "valid eTLD+1 company.io"},
		{host: "dev.local", project: false, dev: true, reason: "dev TLD .local"},
		{host: "app.test", project: false, dev: true, reason: "dev TLD .test"},
		{host: "staging.internal", project: false, dev: true, reason: "dev TLD .internal"},
		{host: "app.jokes", project: false, dev: true, reason: "dynamic dev TLD jokes"},
		{host: "my.app.jokes", project: false, dev: true, reason: "dynamic dev TLD jokes"},
		{host: "localhost", project: false, dev: true, reason: "dev only"},
		{host: "co.uk", project: false, dev: false, reason: "no eTLD+1, suffix only"},
		{host: "192.168.1.1", project: false, dev: false, reason: "IP address"},
		{host: "2001:db8::1", project: false, dev: false, reason: "IP address"},
		{host: "myhost", project: false, dev: false, reason: "no dot, single label"},
		{host: "devbox", project: false, dev: false, reason: "no dot, single label"},
		{host: "", project: false, dev: false, reason: "empty"},
		{host: "   ", project: false, dev: false, reason: "blank"},
		{host: "API.EXAMPLE.COM", project: true, dev: true, reason: "case insensitive"},
		{host: "abcdefghij234567.onion", project: true, dev: true, reason: "overlay network TLD"},
		{host: "abcdefghij234567.b32.i2p", project: true, dev: true, reason: "overlay network TLD"},
		{host: "node.exit", project: true, dev: true, reason: "overlay network TLD"},
	}

	for _, tc := range tests {
		t.Run(tc.host+"/"+tc.reason, func(t *testing.T) {
			if got := IsValidHost(tc.host, false, projectName); got != tc.project {
				t.Errorf("IsValidHost(%q, production) = %v, want %v (%s)", tc.host, got, tc.project, tc.reason)
			}
			if got := IsValidHost(tc.host, true, projectName); got != tc.dev {
				t.Errorf("IsValidHost(%q, development) = %v, want %v (%s)", tc.host, got, tc.dev, tc.reason)
			}
		})
	}
}

func TestIsValidHostWithoutProjectName(t *testing.T) {
	// Development mode is deliberately lenient about unrecognized TLDs
	// (only production enforces the ICANN-suffix requirement), so an
	// arbitrary non-ICANN host like "app.invalidtldnotreal" is expected
	// to validate in dev mode even with no projectName configured.
	if !IsValidHost("app.invalidtldnotreal", true, "") {
		t.Error("app.invalidtldnotreal should be valid in development mode regardless of project name")
	}
	// Production requires a real ICANN TLD, so the same host must be
	// rejected there.
	if IsValidHost("app.invalidtldnotreal", false, "") {
		t.Error("app.invalidtldnotreal must be invalid in production without a project name")
	}
}

func TestIsValidSSLHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "api.example.com", want: true},
		{host: "my.server.domain.co.uk", want: true},
		{host: "abcdefghij234567.onion", want: false},
		{host: "abcdefghij234567.b32.i2p", want: false},
		{host: "node.exit", want: false},
		{host: "dev.local", want: false},
		{host: "app.test", want: false},
		{host: "localhost", want: false},
		{host: "192.0.2.1", want: false},
		{host: "myhost", want: false},
		{host: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			if got := IsValidSSLHost(tc.host); got != tc.want {
				t.Fatalf("IsValidSSLHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestBaseDomainOf(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{host: "myapp.com", want: "myapp.com"},
		{host: "www.myapp.com", want: "myapp.com"},
		{host: "api.staging.myapp.com", want: "myapp.com"},
		{host: "MYAPP.COM", want: "myapp.com"},
		{host: "www.myapp.com:8443", want: "myapp.com"},
		{host: "my.server.domain.co.uk", want: "domain.co.uk"},
		{host: "a.b.c.local", want: "c.local"},
		{host: "x.abcdefghij234567.onion", want: "abcdefghij234567.onion"},
		{host: "192.0.2.1", want: ""},
		{host: "[2001:db8::1]", want: ""},
		{host: "myhost", want: ""},
		{host: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			if got := BaseDomainOf(tc.host); got != tc.want {
				t.Fatalf("BaseDomainOf(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}
