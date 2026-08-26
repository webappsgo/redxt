package notify

import (
	"testing"
	"time"
)

func TestVars(t *testing.T) {
	fixed := time.Date(2026, time.March, 4, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		in   GlobalVarInput
		key  string
		want string
	}{
		{
			name: "app_name passthrough",
			in:   GlobalVarInput{AppName: "redxt", Now: fixed},
			key:  "app_name",
			want: "redxt",
		},
		{
			name: "year derived from fixed clock",
			in:   GlobalVarInput{Now: fixed},
			key:  "year",
			want: "2026",
		},
		{
			name: "onion_url built from onion_address",
			in:   GlobalVarInput{OnionAddress: "abc123.onion", Now: fixed},
			key:  "onion_url",
			want: "http://abc123.onion",
		},
		{
			name: "onion_url empty when no onion address",
			in:   GlobalVarInput{Now: fixed},
			key:  "onion_url",
			want: "",
		},
		{
			name: "i2p_url always empty",
			in:   GlobalVarInput{Now: fixed},
			key:  "i2p_url",
			want: "",
		},
		{
			name: "i2p_address always empty",
			in:   GlobalVarInput{Now: fixed},
			key:  "i2p_address",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Vars(tt.in)[tt.key]
			if got != tt.want {
				t.Errorf("Vars()[%q] = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestVarsZeroNowFallsBackToWallClock(t *testing.T) {
	vars := Vars(GlobalVarInput{})
	if vars["year"] == "" {
		t.Error("expected a non-empty year when Now is zero")
	}
}

func TestMerge(t *testing.T) {
	base := map[string]string{"a": "1", "b": "2"}
	extra := map[string]string{"b": "override", "c": "3"}

	got := Merge(base, extra)

	if got["a"] != "1" {
		t.Errorf("a = %q, want 1", got["a"])
	}
	if got["b"] != "override" {
		t.Errorf("b = %q, want override", got["b"])
	}
	if got["c"] != "3" {
		t.Errorf("c = %q, want 3", got["c"])
	}
	// base must be unmodified.
	if base["b"] != "2" {
		t.Errorf("base was mutated: b = %q", base["b"])
	}
}
