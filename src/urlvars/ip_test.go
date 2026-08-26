package urlvars

import (
	"net"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{ip: "8.8.8.8", want: true},
		{ip: "203.0.113.7", want: true},
		{ip: "2001:db8::1", want: true},
		{ip: "10.1.2.3", want: false},
		{ip: "172.16.0.1", want: false},
		{ip: "172.32.0.1", want: true},
		{ip: "192.168.1.1", want: false},
		{ip: "127.0.0.1", want: false},
		{ip: "169.254.1.1", want: false},
		{ip: "0.0.0.0", want: false},
		{ip: "255.255.255.255", want: false},
		{ip: "224.0.0.1", want: false},
		{ip: "::1", want: false},
		{ip: "fe80::1", want: false},
		{ip: "fd00::1", want: false},
		{ip: "::", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			if got := IsPublicIP(net.ParseIP(tc.ip)); got != tc.want {
				t.Fatalf("IsPublicIP(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestIsPublicIPNil(t *testing.T) {
	if IsPublicIP(nil) {
		t.Fatal("a nil IP is never public")
	}
}

func TestSelectPublicIPs(t *testing.T) {
	tests := []struct {
		name     string
		ips      []string
		wantIPv4 string
		wantIPv6 string
	}{
		{
			name: "no addresses",
		},
		{
			name: "private only",
			ips:  []string{"127.0.0.1", "10.0.0.5", "fe80::1", "fd00::1"},
		},
		{
			name:     "first public of each family wins",
			ips:      []string{"10.0.0.5", "203.0.113.7", "203.0.113.8", "fd00::1", "2001:db8::1", "2001:db8::2"},
			wantIPv4: "203.0.113.7",
			wantIPv6: "2001:db8::1",
		},
		{
			name:     "ipv6 only",
			ips:      []string{"fe80::1", "2001:db8::9"},
			wantIPv6: "2001:db8::9",
		},
		{
			name:     "ipv4 only",
			ips:      []string{"192.168.0.9", "198.51.100.4"},
			wantIPv4: "198.51.100.4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ips := make([]net.IP, 0, len(tc.ips))
			for _, raw := range tc.ips {
				ips = append(ips, net.ParseIP(raw))
			}

			ipv4, ipv6 := SelectPublicIPs(ips)
			if ipv4 != tc.wantIPv4 || ipv6 != tc.wantIPv6 {
				t.Fatalf("SelectPublicIPs = (%q, %q), want (%q, %q)", ipv4, ipv6, tc.wantIPv4, tc.wantIPv6)
			}
		})
	}
}
