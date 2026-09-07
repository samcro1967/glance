package glance

import (
	"context"
	"sync"
	"time"
)

type keyedResourceCacheCall[V any] struct {
	done chan struct{}
	val  V
	err  error
}

type keyedResourceCacheEntry[V any] struct {
	mu       sync.Mutex
	cached   cachedEntry[V]
	hasValue bool
	current  *keyedResourceCacheCall[V]
	lastUsed time.Time
}

type keyedResourceCache[K comparable, V any] struct {
	mu            sync.Mutex
	entries       map[K]*keyedResourceCacheEntry[V]
	idleRetention time.Duration
}

func newKeyedResourceCache[K comparable, V any](idleRetention time.Duration) *keyedResourceCache[K, V] {
	return &keyedResourceCache[K, V]{
		entries:       make(map[K]*keyedResourceCacheEntry[V]),
		idleRetention: idleRetention,
	}
}

func (cache *keyedResourceCache[K, V]) Get(
	ctx context.Context,
	key K,
	valid func(cachedEntry[V], time.Time) bool,
	fetch func(context.Context) (V, error),
) (V, error) {
	now := time.Now()
	entry := cache.entry(key, now)

	entry.mu.Lock()

	if entry.hasValue && valid(entry.cached, now) {
		value := entry.cached.value
		entry.mu.Unlock()
		return value, nil
	}

	if entry.current != nil {
		call := entry.current
		entry.mu.Unlock()

		select {
		case <-call.done:
			return call.val, call.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}

	call := &keyedResourceCacheCall[V]{done: make(chan struct{})}
	entry.current = call
	entry.mu.Unlock()

	call.val, call.err = fetch(ctx)

	entry.mu.Lock()
	if call.err == nil {
		entry.cached = cachedEntry[V]{
			value:     call.val,
			timestamp: time.Now(),
		}
		entry.hasValue = true
	}
	entry.current = nil
	close(call.done)
	entry.mu.Unlock()

	return call.val, call.err
}

func (cache *keyedResourceCache[K, V]) entry(
	key K,
	now time.Time,
) *keyedResourceCacheEntry[V] {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.pruneIdleEntriesLocked(key, now)

	entry, ok := cache.entries[key]
	if !ok {
		entry = &keyedResourceCacheEntry[V]{}
		cache.entries[key] = entry
	}

	entry.mu.Lock()
	entry.lastUsed = now
	entry.mu.Unlock()

	return entry
}

func (cache *keyedResourceCache[K, V]) pruneIdleEntriesLocked(
	requestedKey K,
	now time.Time,
) {
	if cache.idleRetention <= 0 {
		return
	}

	for key, entry := range cache.entries {
		if key == requestedKey {
			continue
		}

		entry.mu.Lock()
		idle := entry.current == nil &&
			!entry.lastUsed.IsZero() &&
			now.Sub(entry.lastUsed) >= cache.idleRetention
		entry.mu.Unlock()

		if idle {
			delete(cache.entries, key)
		}
	}
}
