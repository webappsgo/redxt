package logging

import (
	"strings"
	"testing"
	"time"
)

func TestNewULIDShape(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := NewULID()
		if len(id) != ulidLen {
			t.Fatalf("ULID %q has length %d, want %d", id, len(id), ulidLen)
		}
		for j := 0; j < len(id); j++ {
			if !strings.ContainsRune(crockfordAlphabet, rune(id[j])) {
				t.Fatalf("ULID %q contains %q, which is outside Crockford base32", id, id[j])
			}
		}
	}
}

func TestNewULIDIsMonotonic(t *testing.T) {
	previous := ""
	for i := 0; i < 1000; i++ {
		id := NewULID()
		if id <= previous {
			t.Fatalf("ULID %q at index %d does not sort after %q", id, i, previous)
		}
		previous = id
	}
}

func TestNewULIDAtSameMillisecondIncrements(t *testing.T) {
	fixed := time.Date(2025, 1, 15, 10, 30, 0, 123000000, time.UTC)

	previous := NewULIDAt(fixed)
	for i := 0; i < 500; i++ {
		id := NewULIDAt(fixed)
		if id[:ulidTimeLen] != previous[:ulidTimeLen] {
			t.Fatalf("timestamp component changed within one millisecond: %q then %q", previous, id)
		}
		if id <= previous {
			t.Fatalf("ULID %q at index %d does not sort after %q", id, i, previous)
		}
		previous = id
	}
}

func TestParseULIDTimeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		when time.Time
	}{
		{
			name: "epoch",
			when: time.UnixMilli(0).UTC(),
		},
		{
			name: "spec example",
			when: time.Date(2025, 1, 15, 10, 30, 0, 123000000, time.UTC),
		},
		{
			name: "sub millisecond precision is truncated",
			when: time.Date(2025, 1, 15, 10, 30, 0, 123456789, time.UTC),
		},
		{
			name: "non UTC input is normalized",
			when: time.Date(2025, 6, 1, 12, 0, 0, 500000000, time.FixedZone("EST", -5*60*60)),
		},
		{
			name: "far future",
			when: time.Date(2199, 12, 31, 23, 59, 59, 999000000, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseULIDTime(NewULIDAt(tc.when))
			if err != nil {
				t.Fatalf("ParseULIDTime returned %v", err)
			}
			want := tc.when.UTC().Truncate(time.Millisecond)
			if !got.Equal(want) {
				t.Errorf("ParseULIDTime = %s, want %s", got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestParseULIDTimeRejectsInvalid(t *testing.T) {
	valid := NewULID()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name: "canonical identifier",
			id:   valid,
		},
		{
			name: "lower case decodes",
			id:   strings.ToLower(valid),
		},
		{
			name:    "empty",
			id:      "",
			wantErr: true,
		},
		{
			name:    "too short",
			id:      valid[:ulidLen-1],
			wantErr: true,
		},
		{
			name:    "too long",
			id:      valid + "0",
			wantErr: true,
		},
		{
			name:    "excluded letter I",
			id:      "0000000000IAAAAAAAAAAAAAAA",
			wantErr: true,
		},
		{
			name:    "excluded letter U in the timestamp",
			id:      "U000000000AAAAAAAAAAAAAAAA",
			wantErr: true,
		},
		{
			name:    "punctuation",
			id:      "0000000000AAAAAAAAAAAAAAA-",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseULIDTime(tc.id)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseULIDTime(%q) succeeded, want an error", tc.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseULIDTime(%q) returned %v", tc.id, err)
			}
		})
	}
}

func TestNewULIDAtClampsTimesBeforeEpoch(t *testing.T) {
	id := NewULIDAt(time.Date(1969, 1, 1, 0, 0, 0, 0, time.UTC))

	got, err := ParseULIDTime(id)
	if err != nil {
		t.Fatalf("ParseULIDTime returned %v", err)
	}
	if !got.Equal(time.UnixMilli(0).UTC()) {
		t.Errorf("pre-epoch time encoded as %s, want the epoch", got.Format(time.RFC3339Nano))
	}
}

func TestIncrementEntropy(t *testing.T) {
	tests := []struct {
		name   string
		in     [ulidEntropyBytes]byte
		want   [ulidEntropyBytes]byte
		wantOK bool
	}{
		{
			name:   "zero",
			in:     [ulidEntropyBytes]byte{},
			want:   [ulidEntropyBytes]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			wantOK: true,
		},
		{
			name:   "carry into the previous byte",
			in:     [ulidEntropyBytes]byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 0xff},
			want:   [ulidEntropyBytes]byte{0, 0, 0, 0, 0, 0, 0, 0, 2, 0},
			wantOK: true,
		},
		{
			name:   "wrap reports failure",
			in:     [ulidEntropyBytes]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			want:   [ulidEntropyBytes]byte{},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in
			ok := incrementEntropy(&got)
			if ok != tc.wantOK {
				t.Fatalf("incrementEntropy returned %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("entropy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEncodeULIDIsFixedWidth(t *testing.T) {
	tests := []struct {
		name string
		ms   uint64
	}{
		{name: "zero", ms: 0},
		{name: "one", ms: 1},
		{name: "spec example", ms: uint64(time.Date(2025, 1, 15, 10, 30, 0, 123000000, time.UTC).UnixMilli())},
		{name: "maximum", ms: ulidMaxTime},
	}

	var entropy [ulidEntropyBytes]byte
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := encodeULID(tc.ms, entropy)
			if len(id) != ulidLen {
				t.Fatalf("encodeULID produced %d characters, want %d", len(id), ulidLen)
			}
			got, err := ParseULIDTime(id)
			if err != nil {
				t.Fatalf("ParseULIDTime returned %v", err)
			}
			if uint64(got.UnixMilli()) != tc.ms {
				t.Errorf("round trip gave %d ms, want %d", got.UnixMilli(), tc.ms)
			}
		})
	}
}
