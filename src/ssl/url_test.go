package ssl

import "testing"

func TestFormatURL(t *testing.T) {
	tests := []struct {
		host    string
		port    int
		isHTTPS bool
		want    string
		reason  string
	}{
		{host: "example.com", port: 80, isHTTPS: false, want: "http://example.com", reason: ":80 is always stripped"},
		{host: "example.com", port: 443, isHTTPS: true, want: "https://example.com", reason: ":443 is always stripped"},
		{host: "example.com", port: 443, isHTTPS: false, want: "https://example.com", reason: "port 443 forces https"},
		{host: "example.com", port: 80, isHTTPS: true, want: "https://example.com", reason: "explicit https keeps its scheme"},
		{host: "example.com", port: 8080, isHTTPS: false, want: "http://example.com:8080", reason: "other ports are written out"},
		{host: "example.com", port: 8443, isHTTPS: true, want: "https://example.com:8443", reason: "other https ports are written out"},
		{host: "example.com", port: 64123, isHTTPS: false, want: "http://example.com:64123", reason: "first-run random port"},
		{host: "[2001:db8::1]", port: 8080, isHTTPS: false, want: "http://[2001:db8::1]:8080", reason: "bracketed IPv6 literal"},
		{host: "[2001:db8::1]", port: 443, isHTTPS: true, want: "https://[2001:db8::1]", reason: "bracketed IPv6 literal on 443"},
		{host: "127.0.0.1", port: 80, isHTTPS: false, want: "http://127.0.0.1", reason: "IPv4 literal on 80"},
	}

	for _, tc := range tests {
		t.Run(tc.want+"/"+tc.reason, func(t *testing.T) {
			if got := FormatURL(tc.host, tc.port, tc.isHTTPS); got != tc.want {
				t.Errorf("FormatURL(%q, %d, %v) = %q, want %q (%s)", tc.host, tc.port, tc.isHTTPS, got, tc.want, tc.reason)
			}
		})
	}
}
