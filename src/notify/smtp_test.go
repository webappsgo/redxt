package notify

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func withProbe(t *testing.T, p prober) {
	t.Helper()
	orig := probe
	probe = p
	t.Cleanup(func() { probe = orig })
}

func TestAutodetect(t *testing.T) {
	tests := []struct {
		name     string
		fqdn     string
		succeeds map[string]bool // "host:port" -> success
		wantOK   bool
		wantHost string
		wantPort int
	}{
		{
			name:     "loopback succeeds first",
			fqdn:     "example.com",
			succeeds: map[string]bool{"127.0.0.1:25": true},
			wantOK:   true,
			wantHost: "127.0.0.1",
			wantPort: 25,
		},
		{
			name:     "falls through to fqdn",
			fqdn:     "example.com",
			succeeds: map[string]bool{"example.com:587": true},
			wantOK:   true,
			wantHost: "example.com",
			wantPort: 587,
		},
		{
			name:     "nothing succeeds",
			fqdn:     "example.com",
			succeeds: map[string]bool{},
			wantOK:   false,
		},
		{
			name:     "587 tried before finding 465-only success",
			fqdn:     "",
			succeeds: map[string]bool{"127.0.0.1:465": true},
			wantOK:   true,
			wantHost: "127.0.0.1",
			wantPort: 465,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withProbe(t, func(host string, port int, _ time.Duration) error {
				key := host + ":" + itoa(port)
				if tt.succeeds[key] {
					return nil
				}
				return errors.New("connection refused")
			})

			got, ok := Autodetect(tt.fqdn, time.Second)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Host != tt.wantHost || got.Port != tt.wantPort {
				t.Errorf("got %+v, want host=%q port=%d", got, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestTestConnection(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		port    int
		probeOK bool
		wantErr bool
	}{
		{name: "empty host", host: "", port: 587, wantErr: true},
		{name: "invalid port", host: "localhost", port: 0, wantErr: true},
		{name: "port too large", host: "localhost", port: 70000, wantErr: true},
		{name: "probe fails", host: "localhost", port: 587, probeOK: false, wantErr: true},
		{name: "probe succeeds", host: "localhost", port: 587, probeOK: true, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withProbe(t, func(host string, port int, _ time.Duration) error {
				if tt.probeOK {
					return nil
				}
				return errors.New("refused")
			})

			err := TestConnection(tt.host, tt.port, time.Second)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseGatewayIP(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  string
	}{
		{
			name:  "empty table",
			table: "Iface\tDestination\tGateway\n",
			want:  "",
		},
		{
			name: "default route present",
			// Gateway 0100A8C0 little-endian = 192.168.0.1
			table: "Iface\tDestination\tGateway\tFlags\n" +
				"eth0\t00000000\t0100A8C0\t0003\n" +
				"eth0\t0000A8C0\t00000000\t0001\n",
			want: "192.168.0.1",
		},
		{
			name:  "no default route",
			table: "Iface\tDestination\tGateway\tFlags\n" + "eth0\t0000A8C0\t00000000\t0001\n",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGatewayIP(tt.table)
			if got != tt.want {
				t.Errorf("parseGatewayIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHexLEToIPv4(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0100A8C0", "192.168.0.1"},
		{"0101007F", "127.0.1.1"},
		{"bad", ""},
		{"ZZZZZZZZ", ""},
	}
	for _, tt := range tests {
		got := hexLEToIPv4(tt.in)
		if got != tt.want {
			t.Errorf("hexLEToIPv4(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsPrivateIPv4(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.5", true},
		{"172.16.0.5", true},
		{"192.168.1.5", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tt := range tests {
		got := isPrivateIPv4(net.ParseIP(tt.ip).To4())
		if got != tt.want {
			t.Errorf("isPrivateIPv4(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestBuildMessage(t *testing.T) {
	msg := buildMessage("from@example.com", "to@example.com", "Subject line", "Body\ntext")
	if !contains(msg, "From: from@example.com\r\n") {
		t.Errorf("missing From header: %q", msg)
	}
	if !contains(msg, "To: to@example.com\r\n") {
		t.Errorf("missing To header: %q", msg)
	}
	if !contains(msg, "Subject: Subject line\r\n") {
		t.Errorf("missing Subject header: %q", msg)
	}
	if !contains(msg, "Body\r\ntext") {
		t.Errorf("body not CRLF-normalized: %q", msg)
	}
}

func TestSendEmptyHost(t *testing.T) {
	err := Send(context.Background(), SMTPConfig{Host: ""}, "a@b.c", "d@e.f", "s", "b")
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func TestSendUnknownTLSMode(t *testing.T) {
	err := Send(context.Background(), SMTPConfig{Host: "localhost", Port: 587, TLS: "bogus-mode"}, "a@b.c", "d@e.f", "s", "b")
	if err == nil {
		t.Fatal("expected error for unknown tls mode")
	}
}

// itoa avoids importing strconv twice in tests just for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
