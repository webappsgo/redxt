package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

// janitorInterval is how often the memory driver sweeps expired entries.
const janitorInterval = 60 * time.Second

// memoryEntry is one stored value. A zero expires means the entry never
// expires.
type memoryEntry struct {
	value   []byte
	expires time.Time
}

// expired reports whether the entry is past its expiry at the supplied time.
func (e memoryEntry) expired(now time.Time) bool {
	return !e.expires.IsZero() && now.After(e.expires)
}

// memoryCache is the in-process driver: a mutex-guarded map with lazy expiry
// on read plus a background janitor that reclaims memory held by entries that
// are never read again.
type memoryCache struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
	stop    chan struct{}
	once    sync.Once
}

// newMemoryCache builds a memory driver and starts its janitor goroutine.
func newMemoryCache() *memoryCache {
	c := &memoryCache{
		entries: make(map[string]memoryEntry),
		stop:    make(chan struct{}),
	}
	go c.janitor()
	return c
}

// janitor sweeps expired entries until Close stops it.
func (c *memoryCache) janitor() {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.sweep(time.Now())
		}
	}
}

// sweep removes every entry expired as of now.
func (c *memoryCache) sweep(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if e.expired(now) {
			delete(c.entries, k)
		}
	}
}

// Get returns a copy of the stored value and whether it was present and live.
//
// The copy is defensive: callers must not be able to mutate the bytes still
// held in the map, which would silently corrupt the cache for every other
// reader.
func (c *memoryCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || e.expired(time.Now()) {
		return nil, false, nil
	}
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, true, nil
}

// Set stores a copy of value under key. A ttl of zero stores the value
// without an expiry.
//
// The copy is defensive: a caller reusing or mutating its buffer after the
// call must not change what the cache returns later.
func (c *memoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = newEntry(value, ttl)
	return nil
}

// newEntry builds an entry holding a private copy of value.
func newEntry(value []byte, ttl time.Duration) memoryEntry {
	stored := make([]byte, len(value))
	copy(stored, value)
	e := memoryEntry{value: stored}
	if ttl > 0 {
		e.expires = time.Now().Add(ttl)
	}
	return e
}

// Delete removes the supplied keys.
func (c *memoryCache) Delete(ctx context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.entries, k)
	}
	return nil
}

// DeletePrefix removes every key sharing the supplied prefix.
func (c *memoryCache) DeletePrefix(ctx context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
	return nil
}

// SetNX stores a value only when the key is absent or already expired,
// reporting whether it stored.
func (c *memoryCache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok && !e.expired(time.Now()) {
		return false, nil
	}
	c.entries[key] = newEntry(value, ttl)
	return true, nil
}

// Ping always succeeds: the memory driver is always reachable.
func (c *memoryCache) Ping(ctx context.Context) error {
	return nil
}

// Close stops the janitor and drops every entry. It is safe to call more than
// once.
func (c *memoryCache) Close() error {
	c.once.Do(func() {
		close(c.stop)
		c.mu.Lock()
		c.entries = make(map[string]memoryEntry)
		c.mu.Unlock()
	})
	return nil
}
