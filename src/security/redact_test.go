package security

import (
	"reflect"
	"testing"
)

func TestIsSensitiveField(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  bool
	}{
		{name: "secret", field: "installation_secret", want: true},
		{name: "key", field: "encryption_key", want: true},
		{name: "password", field: "password", want: true},
		{name: "passwd", field: "passwd_hash", want: true},
		{name: "token", field: "api_token", want: true},
		{name: "credential", field: "gitCredential", want: true},
		{name: "private", field: "private_key_pem", want: true},
		{name: "apikey", field: "apikey", want: true},
		{name: "api_key", field: "provider_api_key", want: true},
		{name: "authorization", field: "Authorization", want: true},
		{name: "cookie", field: "Set-Cookie", want: true},
		{name: "session", field: "session_id", want: true},
		{name: "tsig", field: "tsig_secret", want: true},
		{name: "dnssec", field: "dnssec_ksk", want: true},
		{name: "mixed case", field: "TSIG_Secret", want: true},
		{name: "camel case token", field: "apiToken", want: true},
		{name: "zone name", field: "zone", want: false},
		{name: "record type", field: "rrtype", want: false},
		{name: "ttl", field: "ttl", want: false},
		{name: "empty", field: "", want: false},
		{name: "keyword contains key", field: "keyword", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSensitiveField(tt.field); got != tt.want {
				t.Fatalf("IsSensitiveField(%q) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestRedactValue(t *testing.T) {
	if got := RedactValue(); got != "xxxxx" {
		t.Fatalf("RedactValue() = %q, want %q", got, "xxxxx")
	}
}

func TestRedactMap(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{name: "nil map", in: nil, want: nil},
		{name: "empty map", in: map[string]any{}, want: map[string]any{}},
		{
			name: "flat map",
			in:   map[string]any{"zone": "example.com", "api_token": "adm_secretvalue", "ttl": 300},
			want: map[string]any{"zone": "example.com", "api_token": "xxxxx", "ttl": 300},
		},
		{
			name: "nested map",
			in: map[string]any{
				"zone": "example.com",
				"config": map[string]any{
					"upstream":    "9.9.9.9",
					"tsig_secret": "base64secret",
				},
			},
			want: map[string]any{
				"zone": "example.com",
				"config": map[string]any{
					"upstream":    "9.9.9.9",
					"tsig_secret": "xxxxx",
				},
			},
		},
		{
			name: "sensitive key holding a nested map",
			in: map[string]any{
				"dnssec_keys": map[string]any{"ksk": "private material"},
			},
			want: map[string]any{"dnssec_keys": "xxxxx"},
		},
		{
			name: "slice of maps",
			in: map[string]any{
				"agents": []any{
					map[string]any{"name": "edge-1", "token": "adm_agt_value"},
					map[string]any{"name": "edge-2", "password": "hunter2"},
				},
			},
			want: map[string]any{
				"agents": []any{
					map[string]any{"name": "edge-1", "token": "xxxxx"},
					map[string]any{"name": "edge-2", "password": "xxxxx"},
				},
			},
		},
		{
			name: "nested slice of slices",
			in: map[string]any{
				"batches": []any{[]any{map[string]any{"secret": "s"}}},
			},
			want: map[string]any{
				"batches": []any{[]any{map[string]any{"secret": "xxxxx"}}},
			},
		},
		{
			name: "scalar slice untouched",
			in:   map[string]any{"nameservers": []any{"ns1.example.com", "ns2.example.com"}},
			want: map[string]any{"nameservers": []any{"ns1.example.com", "ns2.example.com"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactMap(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("RedactMap() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRedactMapDoesNotMutateInput(t *testing.T) {
	nested := map[string]any{"tsig_secret": "original-secret"}
	items := []any{map[string]any{"password": "original-password"}}
	in := map[string]any{
		"api_token": "original-token",
		"config":    nested,
		"agents":    items,
	}

	out := RedactMap(in)

	if in["api_token"] != "original-token" {
		t.Fatalf("input top-level value was mutated: %v", in["api_token"])
	}
	if nested["tsig_secret"] != "original-secret" {
		t.Fatalf("input nested map was mutated: %v", nested["tsig_secret"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("input slice element type changed")
	}
	if first["password"] != "original-password" {
		t.Fatalf("input slice element was mutated: %v", first["password"])
	}
	if out["api_token"] != "xxxxx" {
		t.Fatalf("output was not redacted: %v", out["api_token"])
	}
}

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{name: "typical address", email: "john@example.com", want: "j***n@e***.com"},
		{name: "two character local part", email: "jo@example.com", want: "***@e***.com"},
		{name: "one character local part", email: "j@example.com", want: "***@e***.com"},
		{name: "three character local part", email: "jon@example.com", want: "j***n@e***.com"},
		{name: "subdomain", email: "john@mail.example.com", want: "j***n@m***.e***.com"},
		{name: "deep subdomain", email: "admin@ns1.dns.example.co", want: "a***n@n***.d***.e***.co"},
		{name: "single label domain", email: "john@localhost", want: "j***n@localhost"},
		{name: "no at sign", email: "notanemail", want: "***"},
		{name: "empty", email: "", want: "***"},
		{name: "empty domain", email: "john@", want: "***"},
		{name: "empty local part", email: "@example.com", want: "***@e***.com"},
		{name: "plus addressing", email: "john+dns@example.com", want: "j***s@e***.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskEmail(tt.email); got != tt.want {
				t.Fatalf("MaskEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestMaskUsername(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "typical", in: "casjay", want: "c***"},
		{name: "single character", in: "c", want: "c***"},
		{name: "empty", in: "", want: "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskUsername(tt.in); got != tt.want {
				t.Fatalf("MaskUsername(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "admin token", token: PrefixAdmin + fixedRandom, want: "adm_a1b2..."},
		{name: "agent token", token: PrefixAdminAgent + fixedRandom, want: "adm_agt_..."},
		{name: "short value", token: "adm_", want: "adm_..."},
		{name: "empty", token: "", want: "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskToken(tt.token); got != tt.want {
				t.Fatalf("MaskToken(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestMaskIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want string
	}{
		{name: "ipv4", ip: "192.168.1.100", want: "192.168.1.0"},
		{name: "ipv4 already zeroed", ip: "10.0.0.0", want: "10.0.0.0"},
		{name: "ipv4 loopback", ip: "127.0.0.1", want: "127.0.0.0"},
		{name: "ipv4 with whitespace", ip: "  8.8.8.8  ", want: "8.8.8.0"},
		{name: "ipv6", ip: "2001:db8:1:2::1", want: "2001:db8:1::"},
		{name: "ipv6 full", ip: "2606:4700:4700::1111", want: "2606:4700:4700::"},
		{name: "ipv6 already masked", ip: "2001:db8:1::", want: "2001:db8:1::"},
		{name: "ipv4 mapped ipv6", ip: "::ffff:192.168.1.100", want: "192.168.1.0"},
		{name: "garbage", ip: "not-an-ip", want: ""},
		{name: "empty", ip: "", want: ""},
		{name: "hostname", ip: "ns1.example.com", want: ""},
		{name: "ipv4 with port", ip: "192.168.1.100:53", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskIP(tt.ip); got != tt.want {
				t.Fatalf("MaskIP(%q) = %q, want %q", tt.ip, got, tt.want)
			}
		})
	}
}
