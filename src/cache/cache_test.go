package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// newTestCache returns a memory driver closed at the end of the test.
func newTestCache(t *testing.T) Cache {
	t.Helper()
	c, err := New(Config{Type: TypeMemory})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
	return c
}

func TestNewDriverSelection(t *testing.T) {
	tests := []struct {
		name       string
		cfgType    string
		wantErr    bool
		wantErrSub string
	}{
		{name: "none", cfgType: TypeNone},
		{name: "memory", cfgType: TypeMemory},
		{name: "empty defaults to memory", cfgType: ""},
		{name: "valkey unavailable", cfgType: TypeValkey, wantErr: true, wantErrSub: `driver "valkey" is not available`},
		{name: "redis unavailable", cfgType: TypeRedis, wantErr: true, wantErrSub: `driver "redis" is not available`},
		{name: "unknown driver", cfgType: "memcached", wantErr: true, wantErrSub: `unknown driver "memcached"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(Config{Type: tt.cfgType})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErrSub)
				}
				if c != nil {
					t.Fatal("expected a nil cache alongside the error")
				}
				return
			}
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			if c == nil {
				t.Fatal("expected a cache, got nil")
			}
			if err := c.Close(); err != nil {
				t.Fatalf("Close returned error: %v", err)
			}
		})
	}
}

func TestNoopDriver(t *testing.T) {
	ctx := context.Background()
	c, err := New(Config{Type: TypeNone})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := c.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if _, ok, err := c.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("Get = (_, %v, %v), want (nil, false, nil)", ok, err)
	}
	if ok, err := c.SetNX(ctx, "k", []byte("v"), time.Minute); err != nil || !ok {
		t.Fatalf("SetNX = (%v, %v), want (true, nil)", ok, err)
	}
	if err := c.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := c.DeletePrefix(ctx, "k"); err != nil {
		t.Fatalf("DeletePrefix returned error: %v", err)
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestMemoryRoundTrip(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value []byte
	}{
		{name: "short value", key: "user:1", value: []byte("alice")},
		{name: "empty value", key: "user:2", value: []byte{}},
		{name: "binary value", key: "user:3", value: []byte{0x00, 0xff, 0x10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			if err := c.Set(ctx, tt.key, tt.value, 0); err != nil {
				t.Fatalf("Set returned error: %v", err)
			}
			got, ok, err := c.Get(ctx, tt.key)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if !ok {
				t.Fatal("Get reported a miss for a key just set")
			}
			if string(got) != string(tt.value) {
				t.Fatalf("value = %q, want %q", got, tt.value)
			}
			if err := c.Delete(ctx, tt.key); err != nil {
				t.Fatalf("Delete returned error: %v", err)
			}
			if _, ok, _ := c.Get(ctx, tt.key); ok {
				t.Fatal("Get reported a hit after Delete")
			}
		})
	}
}

func TestMemoryMissAndDeletePrefix(t *testing.T) {
	ctx := context.Background()
	c := newTestCache(t)

	for _, k := range []string{"user:1", "user:2", "org:1"} {
		if err := c.Set(ctx, k, []byte(k), 0); err != nil {
			t.Fatalf("Set returned error: %v", err)
		}
	}
	if err := c.DeletePrefix(ctx, "user:"); err != nil {
		t.Fatalf("DeletePrefix returned error: %v", err)
	}

	tests := []struct {
		name    string
		key     string
		wantHit bool
	}{
		{name: "prefixed key removed", key: "user:1", wantHit: false},
		{name: "second prefixed key removed", key: "user:2", wantHit: false},
		{name: "other key kept", key: "org:1", wantHit: true},
		{name: "never set key misses", key: "nope", wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok, err := c.Get(ctx, tt.key)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if ok != tt.wantHit {
				t.Fatalf("hit = %v, want %v", ok, tt.wantHit)
			}
		})
	}
}

func TestMemorySetNX(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		prepare func(t *testing.T, c Cache)
		want    bool
	}{
		{
			name:    "absent key is set",
			prepare: func(t *testing.T, c Cache) {},
			want:    true,
		},
		{
			name: "present key is not set",
			prepare: func(t *testing.T, c Cache) {
				if err := c.Set(ctx, "k", []byte("first"), time.Minute); err != nil {
					t.Fatalf("Set returned error: %v", err)
				}
			},
			want: false,
		},
		{
			name: "expired key is set again",
			prepare: func(t *testing.T, c Cache) {
				if err := c.Set(ctx, "k", []byte("first"), 5*time.Millisecond); err != nil {
					t.Fatalf("Set returned error: %v", err)
				}
				time.Sleep(20 * time.Millisecond)
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			tt.prepare(t, c)
			ok, err := c.SetNX(ctx, "k", []byte("second"), time.Minute)
			if err != nil {
				t.Fatalf("SetNX returned error: %v", err)
			}
			if ok != tt.want {
				t.Fatalf("SetNX = %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestMemoryTTLExpiry(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		ttl     time.Duration
		sleep   time.Duration
		wantHit bool
	}{
		{name: "no expiry survives", ttl: 0, sleep: 20 * time.Millisecond, wantHit: true},
		{name: "long ttl survives", ttl: time.Minute, sleep: 20 * time.Millisecond, wantHit: true},
		{name: "short ttl expires", ttl: 5 * time.Millisecond, sleep: 20 * time.Millisecond, wantHit: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			if err := c.Set(ctx, "k", []byte("v"), tt.ttl); err != nil {
				t.Fatalf("Set returned error: %v", err)
			}
			time.Sleep(tt.sleep)
			_, ok, err := c.Get(ctx, "k")
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if ok != tt.wantHit {
				t.Fatalf("hit = %v, want %v", ok, tt.wantHit)
			}
		})
	}
}

func TestMemoryDefensiveCopy(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		mutate func(stored, returned []byte)
	}{
		{
			name:   "caller mutates the buffer it passed to Set",
			mutate: func(stored, returned []byte) { stored[0] = 'X' },
		},
		{
			name:   "caller mutates the slice returned by Get",
			mutate: func(stored, returned []byte) { returned[0] = 'X' },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			stored := []byte("value")
			if err := c.Set(ctx, "k", stored, 0); err != nil {
				t.Fatalf("Set returned error: %v", err)
			}
			returned, _, err := c.Get(ctx, "k")
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			tt.mutate(stored, returned)

			again, ok, err := c.Get(ctx, "k")
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if !ok {
				t.Fatal("Get reported a miss")
			}
			if string(again) != "value" {
				t.Fatalf("cached value = %q, want %q", again, "value")
			}
		})
	}
}

func TestMemoryCloseIsIdempotent(t *testing.T) {
	c, err := New(Config{Type: TypeMemory})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := c.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	}
}

func TestMemorySweep(t *testing.T) {
	ctx := context.Background()
	c := newMemoryCache()
	defer func() {
		_ = c.Close()
	}()

	if err := c.Set(ctx, "gone", []byte("v"), time.Millisecond); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if err := c.Set(ctx, "kept", []byte("v"), 0); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	c.sweep(time.Now().Add(time.Minute))

	c.mu.RLock()
	_, goneExists := c.entries["gone"]
	_, keptExists := c.entries["kept"]
	c.mu.RUnlock()

	if goneExists {
		t.Fatal("expired entry survived the sweep")
	}
	if !keptExists {
		t.Fatal("entry with no expiry was swept")
	}
}

// profile is a small payload used by the JSON helper tests.
type profile struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestJSONHelpers(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value profile
	}{
		{name: "populated", key: "user:1:profile", value: profile{Name: "alice", Age: 30}},
		{name: "zero value", key: "user:2:profile", value: profile{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			if _, ok, err := GetJSON[profile](ctx, c, tt.key); err != nil || ok {
				t.Fatalf("GetJSON before Set = (_, %v, %v), want (false, nil)", ok, err)
			}
			if err := SetJSON(ctx, c, tt.key, tt.value, TTLUserProfile); err != nil {
				t.Fatalf("SetJSON returned error: %v", err)
			}
			got, ok, err := GetJSON[profile](ctx, c, tt.key)
			if err != nil {
				t.Fatalf("GetJSON returned error: %v", err)
			}
			if !ok {
				t.Fatal("GetJSON reported a miss")
			}
			if got != tt.value {
				t.Fatalf("value = %+v, want %+v", got, tt.value)
			}
		})
	}
}

func TestGetJSONDecodeError(t *testing.T) {
	ctx := context.Background()
	c := newTestCache(t)
	if err := c.Set(ctx, "bad", []byte("{not json"), 0); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if _, ok, err := GetJSON[profile](ctx, c, "bad"); err == nil || ok {
		t.Fatalf("GetJSON = (_, %v, %v), want (false, error)", ok, err)
	}
}

func TestGetOrSet(t *testing.T) {
	ctx := context.Background()
	loadErr := errors.New("load failed")

	tests := []struct {
		name      string
		preload   bool
		loadErr   error
		wantCalls int
		wantValue profile
		wantErr   error
	}{
		{
			name:      "miss loads and caches",
			wantCalls: 1,
			wantValue: profile{Name: "alice", Age: 30},
		},
		{
			name:      "hit skips the loader",
			preload:   true,
			wantCalls: 0,
			wantValue: profile{Name: "cached", Age: 1},
		},
		{
			name:      "loader error propagates",
			loadErr:   loadErr,
			wantCalls: 1,
			wantErr:   loadErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			const key = "user:1"
			if tt.preload {
				if err := SetJSON(ctx, c, key, profile{Name: "cached", Age: 1}, 0); err != nil {
					t.Fatalf("SetJSON returned error: %v", err)
				}
			}
			calls := 0
			got, err := GetOrSet(ctx, c, key, TTLUserProfile, func() (profile, error) {
				calls++
				if tt.loadErr != nil {
					return profile{}, tt.loadErr
				}
				return profile{Name: "alice", Age: 30}, nil
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Fatalf("loader calls = %d, want %d", calls, tt.wantCalls)
			}
			if got != tt.wantValue {
				t.Fatalf("value = %+v, want %+v", got, tt.wantValue)
			}
			if tt.wantErr == nil {
				cached, ok, err := GetJSON[profile](ctx, c, key)
				if err != nil || !ok {
					t.Fatalf("cached lookup = (_, %v, %v), want (true, nil)", ok, err)
				}
				if cached != tt.wantValue {
					t.Fatalf("cached value = %+v, want %+v", cached, tt.wantValue)
				}
			}
		})
	}
}

func TestKeyBuilders(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "resource", got: Resource("user", "123"), want: "user:123"},
		{name: "field", got: Field("user", "123", "sessions"), want: "user:123:sessions"},
		{name: "list", got: List("jokes", "category=puns"), want: "jokes:list:category-puns"},
		{name: "scoped", got: Scoped("org", "42", "settings", "theme"), want: "org:42:settings:theme"},
		{name: "rate", got: Rate("api", "192.168.1.1"), want: "rate:api:192.168.1.1"},
		{name: "lock", got: Lock("backup"), want: "lock:backup"},
		{name: "versioned", got: Versioned(1, Resource("user", "123")), want: "v1:user:123"},
		{name: "uppercase lowered", got: Key("User", "ABC"), want: "user:abc"},
		{name: "spaces dashed", got: Key("user name", "  padded  "), want: "user-name:padded"},
		{name: "special characters collapsed", got: Key("a/b\\c", "d?e"), want: "a-b-c:d-e"},
		{name: "run collapses to one dash", got: Key("a$$$b"), want: "a-b"},
		{name: "allowed punctuation kept", got: Key("a.b_c-d"), want: "a.b_c-d"},
		{name: "empty parts dropped", got: Key("user", "", "123"), want: "user:123"},
		{name: "whitespace only part dropped", got: Key("user", "   ", "123"), want: "user:123"},
		{name: "no parts", got: Key(), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("key = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{name: "type", got: cfg.Type, want: TypeMemory},
		{name: "host", got: cfg.Host, want: "localhost"},
		{name: "port", got: cfg.Port, want: 6379},
		{name: "db", got: cfg.DB, want: 0},
		{name: "pool size", got: cfg.PoolSize, want: 10},
		{name: "min idle", got: cfg.MinIdle, want: 2},
		{name: "timeout", got: cfg.Timeout, want: 5 * time.Second},
		{name: "prefix", got: cfg.Prefix, want: "redxt:"},
		{name: "ttl", got: cfg.TTL, want: time.Hour},
		{name: "cluster", got: cfg.Cluster, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestLocker(t *testing.T) {
	ctx := context.Background()

	t.Run("acquire then release", func(t *testing.T) {
		c := newTestCache(t)
		l := NewLocker(c, "node-a")
		ok, err := l.Acquire(ctx, "backup", time.Minute)
		if err != nil {
			t.Fatalf("Acquire returned error: %v", err)
		}
		if !ok {
			t.Fatal("Acquire = false, want true for a free lock")
		}
		if err := l.Release(ctx, "backup"); err != nil {
			t.Fatalf("Release returned error: %v", err)
		}
		if _, held, _ := c.Get(ctx, Lock("backup")); held {
			t.Fatal("lock key survived Release")
		}
	})

	t.Run("second node cannot acquire", func(t *testing.T) {
		c := newTestCache(t)
		a := NewLocker(c, "node-a")
		b := NewLocker(c, "node-b")
		if ok, err := a.Acquire(ctx, "backup", time.Minute); err != nil || !ok {
			t.Fatalf("node-a Acquire = (%v, %v), want (true, nil)", ok, err)
		}
		ok, err := b.Acquire(ctx, "backup", time.Minute)
		if err != nil {
			t.Fatalf("Acquire returned error: %v", err)
		}
		if ok {
			t.Fatal("node-b acquired a lock node-a already holds")
		}
	})

	t.Run("release never drops another node's lock", func(t *testing.T) {
		c := newTestCache(t)
		a := NewLocker(c, "node-a")
		b := NewLocker(c, "node-b")
		if ok, err := a.Acquire(ctx, "backup", time.Minute); err != nil || !ok {
			t.Fatalf("node-a Acquire = (%v, %v), want (true, nil)", ok, err)
		}
		if err := b.Release(ctx, "backup"); err != nil {
			t.Fatalf("Release returned error: %v", err)
		}
		val, held, err := c.Get(ctx, Lock("backup"))
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if !held || string(val) != "node-a" {
			t.Fatalf("lock = (%q, %v), want (\"node-a\", true)", val, held)
		}
	})

	t.Run("release of a free lock is a no-op", func(t *testing.T) {
		c := newTestCache(t)
		l := NewLocker(c, "node-a")
		if err := l.Release(ctx, "never-held"); err != nil {
			t.Fatalf("Release returned error: %v", err)
		}
	})
}

func TestLockerWithLock(t *testing.T) {
	ctx := context.Background()
	fnErr := errors.New("work failed")

	tests := []struct {
		name      string
		heldBy    string
		fnErr     error
		wantRuns  int
		wantErr   error
		wantFreed bool
	}{
		{name: "runs when free", wantRuns: 1, wantFreed: true},
		{name: "propagates fn error", fnErr: fnErr, wantRuns: 1, wantErr: fnErr, wantFreed: true},
		{name: "skips when another node holds it", heldBy: "node-b", wantRuns: 0, wantFreed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			if tt.heldBy != "" {
				if ok, err := NewLocker(c, tt.heldBy).Acquire(ctx, "backup", time.Minute); err != nil || !ok {
					t.Fatalf("holder Acquire = (%v, %v), want (true, nil)", ok, err)
				}
			}
			l := NewLocker(c, "node-a")
			runs := 0
			err := l.WithLock(ctx, "backup", time.Minute, func() error {
				runs++
				return tt.fnErr
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if runs != tt.wantRuns {
				t.Fatalf("fn runs = %d, want %d", runs, tt.wantRuns)
			}
			_, held, _ := c.Get(ctx, Lock("backup"))
			if tt.wantFreed && held {
				t.Fatal("lock was not released")
			}
			if !tt.wantFreed && !held {
				t.Fatal("another node's lock was released")
			}
		})
	}
}
