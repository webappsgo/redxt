// Package cache implements the driver-agnostic cache layer defined by AI.md
// PART 9 (ERROR HANDLING & CACHING) and configured by PART 12's "Cache
// Configuration" section.
//
// The in-process memory driver is the default and works standalone; the
// valkey and redis drivers are the production choices for clustered and mixed
// mode deployments and are selected through Config.Type.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Cache driver names accepted by Config.Type.
const (
	// TypeNone disables caching entirely.
	TypeNone = "none"
	// TypeMemory is the in-process driver used by single-instance deployments.
	TypeMemory = "memory"
	// TypeValkey is the Valkey driver used by clustered deployments.
	TypeValkey = "valkey"
	// TypeRedis is the Redis driver used by clustered deployments.
	TypeRedis = "redis"
)

// Cache is the driver-agnostic cache contract every driver implements.
type Cache interface {
	// Get returns the value for a key and whether it was present.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// Set stores a value under a key. A ttl of zero means no expiry.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes the supplied keys, ignoring keys that are absent.
	Delete(ctx context.Context, keys ...string) error
	// DeletePrefix removes every key sharing the supplied prefix.
	DeletePrefix(ctx context.Context, prefix string) error
	// SetNX stores a value only when the key is absent, reporting whether it stored.
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	// Ping verifies the cache is reachable.
	Ping(ctx context.Context) error
	// Close releases any resources held by the driver.
	Close() error
}

// Config holds the cache settings from PART 12's server.cache YAML block.
type Config struct {
	Type          string
	URL           string
	Host          string
	Port          int
	Username      string
	Password      string
	DB            int
	TLS           bool
	TLSSkipVerify bool
	PoolSize      int
	MinIdle       int
	Timeout       time.Duration
	Prefix        string
	TTL           time.Duration
	Cluster       bool
	ClusterNodes  []string
}

// DefaultConfig returns the PART 12 defaults: the in-process memory driver on
// a local Valkey/Redis endpoint should the operator switch drivers.
func DefaultConfig() Config {
	return Config{
		Type:     TypeMemory,
		Host:     "localhost",
		Port:     6379,
		DB:       0,
		PoolSize: 10,
		MinIdle:  2,
		Timeout:  5 * time.Second,
		Prefix:   "redxt:",
		TTL:      time.Hour,
	}
}

// New builds the cache driver named by cfg.Type. An empty type selects the
// memory driver. The valkey and redis drivers are not compiled into this
// build and report a clear error rather than silently degrading.
func New(cfg Config) (Cache, error) {
	switch cfg.Type {
	case TypeNone:
		return noopCache{}, nil
	case TypeMemory, "":
		return newMemoryCache(), nil
	case TypeValkey, TypeRedis:
		return nil, fmt.Errorf("cache: driver %q is not available in this build", cfg.Type)
	default:
		return nil, fmt.Errorf("cache: unknown driver %q", cfg.Type)
	}
}

// noopCache is the "none" driver: every read misses and every write is
// discarded, so callers need no branching when caching is disabled.
type noopCache struct{}

// Get always reports a miss.
func (noopCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, nil
}

// Set discards the value.
func (noopCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return nil
}

// Delete does nothing.
func (noopCache) Delete(ctx context.Context, keys ...string) error {
	return nil
}

// DeletePrefix does nothing.
func (noopCache) DeletePrefix(ctx context.Context, prefix string) error {
	return nil
}

// SetNX always reports success so lock-style callers keep working without a cache.
func (noopCache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return true, nil
}

// Ping always succeeds.
func (noopCache) Ping(ctx context.Context) error {
	return nil
}

// Close does nothing.
func (noopCache) Close() error {
	return nil
}

// GetJSON reads a key and decodes its JSON payload into T. The boolean
// reports whether the key was present.
func GetJSON[T any](ctx context.Context, c Cache, key string) (T, bool, error) {
	var zero T
	raw, ok, err := c.Get(ctx, key)
	if err != nil || !ok {
		return zero, ok, err
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, false, fmt.Errorf("cache: decode %q: %w", key, err)
	}
	return out, true, nil
}

// SetJSON encodes v as JSON and stores it under a key.
func SetJSON[T any](ctx context.Context, c Cache, key string, v T, ttl time.Duration) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("cache: encode %q: %w", key, err)
	}
	return c.Set(ctx, key, raw, ttl)
}

// GetOrSet returns the cached value for a key, calling load and caching its
// result on a miss. This is the read-through helper behind PART 9's
// version-based invalidation pattern, where a bumped version in the key makes
// the next read a miss and old keys expire naturally by TTL.
func GetOrSet[T any](ctx context.Context, c Cache, key string, ttl time.Duration, load func() (T, error)) (T, error) {
	if v, ok, err := GetJSON[T](ctx, c, key); err == nil && ok {
		return v, nil
	}
	var zero T
	v, err := load()
	if err != nil {
		return zero, err
	}
	if err := SetJSON(ctx, c, key, v, ttl); err != nil {
		return v, err
	}
	return v, nil
}
