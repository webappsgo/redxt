package cache

import (
	"context"
	"time"
)

// Locker implements the PART 9 distributed lock: operations that must run on
// exactly one node claim a lock key holding this node's identifier.
type Locker struct {
	cache  Cache
	nodeID string
}

// NewLocker builds a Locker that claims locks in c on behalf of nodeID.
func NewLocker(c Cache, nodeID string) *Locker {
	return &Locker{cache: c, nodeID: nodeID}
}

// Acquire claims the named lock for ttl, reporting whether this node won it.
// The ttl bounds how long a crashed node can hold the lock.
func (l *Locker) Acquire(ctx context.Context, name string, ttl time.Duration) (bool, error) {
	return l.cache.SetNX(ctx, Lock(name), []byte(l.nodeID), ttl)
}

// Release drops the named lock, but only when the stored value is this node's
// identifier.
//
// Releasing a lock another node owns would let two nodes run the same
// exclusive operation at once, so a value mismatch (the lock expired and was
// re-claimed elsewhere) is a no-op rather than a delete.
func (l *Locker) Release(ctx context.Context, name string) error {
	key := Lock(name)
	val, ok, err := l.cache.Get(ctx, key)
	if err != nil {
		return err
	}
	if !ok || string(val) != l.nodeID {
		return nil
	}
	return l.cache.Delete(ctx, key)
}

// WithLock runs fn while holding the named lock, releasing it afterwards.
//
// When the lock is already held elsewhere, fn is not run and nil is returned:
// another node is handling the work, which is the expected outcome, not a
// failure.
func (l *Locker) WithLock(ctx context.Context, name string, ttl time.Duration, fn func() error) error {
	acquired, err := l.Acquire(ctx, name, ttl)
	if err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer func() {
		_ = l.Release(ctx, name)
	}()
	return fn()
}
