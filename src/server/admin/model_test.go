package admin

import (
	"testing"
	"time"
)

func TestSessionExpired(t *testing.T) {
	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "no expiry", expiresAt: time.Time{}, want: false},
		{name: "one second before expiry", expiresAt: now.Add(time.Second), want: false},
		{name: "exactly at expiry", expiresAt: now, want: true},
		{name: "one second past expiry", expiresAt: now.Add(-time.Second), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{ExpiresAt: tt.expiresAt}
			if got := s.Expired(now); got != tt.want {
				t.Fatalf("Expired() = %v, want %v", got, tt.want)
			}
		})
	}
}
