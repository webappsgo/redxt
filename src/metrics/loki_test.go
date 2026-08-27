package metrics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLokiBufferGroupsByLabelSet(t *testing.T) {
	b := NewLokiBuffer(10, time.Hour)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	b.Add(map[string]string{"app": "redxt"}, "line1", now)
	b.Add(map[string]string{"app": "redxt"}, "line2", now.Add(time.Second))
	b.Add(map[string]string{"app": "other"}, "line3", now.Add(2*time.Second))

	body, err := b.JSON(now.Add(3 * time.Second))
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var payload streamPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Streams) != 2 {
		t.Fatalf("streams = %d, want 2", len(payload.Streams))
	}
	for _, s := range payload.Streams {
		if s.Stream["app"] == "redxt" && len(s.Values) != 2 {
			t.Fatalf("redxt stream values = %d, want 2", len(s.Values))
		}
		if s.Stream["app"] == "other" && len(s.Values) != 1 {
			t.Fatalf("other stream values = %d, want 1", len(s.Values))
		}
	}
}

func TestLokiBufferEvictsOldAndOverflow(t *testing.T) {
	b := NewLokiBuffer(2, time.Minute)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	b.Add(nil, "old", now.Add(-10*time.Minute)) // older than maxAge, evicted immediately
	b.Add(nil, "a", now)
	b.Add(nil, "b", now)
	b.Add(nil, "c", now) // pushes buffer past max of 2

	b.mu.Lock()
	n := len(b.entries)
	b.mu.Unlock()
	if n != 2 {
		t.Fatalf("buffer length = %d, want 2 (max enforced)", n)
	}
}

func TestNewLokiBufferDefaults(t *testing.T) {
	b := NewLokiBuffer(0, 0)
	if b.max != 1000 {
		t.Fatalf("max = %d, want 1000 default", b.max)
	}
	if b.maxAge != time.Hour {
		t.Fatalf("maxAge = %s, want 1h default", b.maxAge)
	}
}

func TestFormatNanoTimestamp(t *testing.T) {
	tm := time.Unix(1000, 500)
	if got := formatNanoTimestamp(tm); got != "1000000000500" {
		t.Fatalf("formatNanoTimestamp = %q, want 1000000000500", got)
	}
}
