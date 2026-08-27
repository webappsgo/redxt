package metrics

import (
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"
)

// logEntry is one line kept for the loki service.
type logEntry struct {
	labels map[string]string
	line   string
	at     time.Time
}

// LokiBuffer is a bounded, time-windowed ring buffer of recent log
// lines, served by the loki metrics service in Loki push-API stream
// format. It is deliberately separate from the file logger: it holds
// only what a monitoring stack needs to tail recently, not a durable
// record.
type LokiBuffer struct {
	mu      sync.Mutex
	entries []logEntry
	max     int
	maxAge  time.Duration
}

// NewLokiBuffer returns a buffer that keeps at most max entries, each
// no older than maxAge.
func NewLokiBuffer(max int, maxAge time.Duration) *LokiBuffer {
	if max <= 0 {
		max = 1000
	}
	if maxAge <= 0 {
		maxAge = time.Hour
	}
	return &LokiBuffer{max: max, maxAge: maxAge}
}

// Add appends one log line with its labels, redacting is the caller's
// responsibility (the same sanitization the file loggers already
// apply must run before Add per AI.md PART 21).
func (b *LokiBuffer) Add(labels map[string]string, line string, at time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = append(b.entries, logEntry{labels: labels, line: line, at: at})
	b.evict(at)
}

// evict drops entries older than maxAge or beyond the max count. It
// must be called with mu held.
func (b *LokiBuffer) evict(now time.Time) {
	cutoff := now.Add(-b.maxAge)
	kept := b.entries[:0]
	for _, e := range b.entries {
		if e.at.After(cutoff) {
			kept = append(kept, e)
		}
	}
	if len(kept) > b.max {
		kept = kept[len(kept)-b.max:]
	}
	b.entries = kept
}

// streamPayload is the Loki push-API stream document shape.
type streamPayload struct {
	Streams []stream `json:"streams"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// JSON renders the current buffer contents as of now in Loki
// push-API stream format, grouped by identical label sets.
func (b *LokiBuffer) JSON(now time.Time) ([]byte, error) {
	b.mu.Lock()
	b.evict(now)
	entries := append([]logEntry(nil), b.entries...)
	b.mu.Unlock()

	groups := map[string]*stream{}
	var order []string
	for _, e := range entries {
		key := seriesKey(e.labels)
		g, ok := groups[key]
		if !ok {
			g = &stream{Stream: e.labels}
			groups[key] = g
			order = append(order, key)
		}
		g.Values = append(g.Values, [2]string{formatNanoTimestamp(e.at), e.line})
	}
	sort.Strings(order)

	payload := streamPayload{}
	for _, key := range order {
		payload.Streams = append(payload.Streams, *groups[key])
	}
	return json.Marshal(payload)
}

func formatNanoTimestamp(t time.Time) string {
	return strconv.FormatInt(t.UnixNano(), 10)
}
