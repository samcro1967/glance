package glance

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyedResourceCacheCachesValidValue(t *testing.T) {
	cache := newKeyedResourceCache[string, string](time.Hour)
	var calls atomic.Int32

	fetch := func(context.Context) (string, error) {
		calls.Add(1)
		return "value", nil
	}
	valid := func(cachedEntry[string], time.Time) bool {
		return true
	}

	first, err := cache.Get(context.Background(), "key", valid, fetch)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	second, err := cache.Get(context.Background(), "key", valid, fetch)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}

	if first != "value" || second != "value" {
		t.Fatalf("values = %q / %q, want value / value", first, second)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1", got)
	}
}

func TestKeyedResourceCacheKeepsKeysIndependent(t *testing.T) {
	cache := newKeyedResourceCache[string, string](time.Hour)
	var calls atomic.Int32

	fetch := func(value string) func(context.Context) (string, error) {
		return func(context.Context) (string, error) {
			calls.Add(1)
			return value, nil
		}
	}
	valid := func(cachedEntry[string], time.Time) bool {
		return true
	}

	first, err := cache.Get(context.Background(), "first", valid, fetch("first"))
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	second, err := cache.Get(context.Background(), "second", valid, fetch("second"))
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}

	if first != "first" || second != "second" {
		t.Fatalf("values = %q / %q", first, second)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("fetch calls = %d, want 2", got)
	}
}

func TestKeyedResourceCacheDoesNotCacheFailure(t *testing.T) {
	cache := newKeyedResourceCache[string, string](time.Hour)
	var calls atomic.Int32
	wantErr := errors.New("failed")

	fetch := func(context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "", wantErr
		}
		return "recovered", nil
	}
	valid := func(cachedEntry[string], time.Time) bool {
		return true
	}

	if _, err := cache.Get(context.Background(), "key", valid, fetch); !errors.Is(err, wantErr) {
		t.Fatalf("first Get error = %v, want %v", err, wantErr)
	}

	value, err := cache.Get(context.Background(), "key", valid, fetch)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if value != "recovered" {
		t.Fatalf("value = %q, want recovered", value)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("fetch calls = %d, want 2", got)
	}
}

func TestKeyedResourceCacheRefreshesInvalidValue(t *testing.T) {
	cache := newKeyedResourceCache[string, int](time.Hour)
	var calls atomic.Int32

	fetch := func(context.Context) (int, error) {
		return int(calls.Add(1)), nil
	}

	first, err := cache.Get(
		context.Background(),
		"key",
		func(cachedEntry[int], time.Time) bool { return true },
		fetch,
	)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	second, err := cache.Get(
		context.Background(),
		"key",
		func(cachedEntry[int], time.Time) bool { return false },
		fetch,
	)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}

	if first != 1 || second != 2 {
		t.Fatalf("values = %d / %d, want 1 / 2", first, second)
	}
}

func TestKeyedResourceCacheCoalescesConcurrentFetches(t *testing.T) {
	cache := newKeyedResourceCache[string, string](time.Hour)

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32

	fetch := func(context.Context) (string, error) {
		calls.Add(1)
		once.Do(func() { close(started) })
		<-release
		return "value", nil
	}
	valid := func(cachedEntry[string], time.Time) bool {
		return true
	}

	const callers = 8
	errs := make(chan error, callers)

	for range callers {
		go func() {
			_, err := cache.Get(context.Background(), "key", valid, fetch)
			errs <- err
		}()
	}

	<-started
	time.Sleep(20 * time.Millisecond)
	close(release)

	for range callers {
		if err := <-errs; err != nil {
			t.Fatalf("Get: %v", err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("fetch calls = %d, want 1", got)
	}
}

func TestKeyedResourceCacheWaitingCallerCanCancel(t *testing.T) {
	cache := newKeyedResourceCache[string, string](time.Hour)

	started := make(chan struct{})
	release := make(chan struct{})

	fetch := func(context.Context) (string, error) {
		close(started)
		<-release
		return "value", nil
	}
	valid := func(cachedEntry[string], time.Time) bool {
		return true
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.Get(context.Background(), "key", valid, fetch)
		firstDone <- err
	}()

	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := cache.Get(ctx, "key", valid, fetch); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting Get error = %v, want context.Canceled", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Get: %v", err)
	}
}

func TestKeyedResourceCacheEvictsIdleEntries(t *testing.T) {
	retention := time.Hour
	cache := newKeyedResourceCache[string, string](retention)

	stale := &keyedResourceCacheEntry[string]{
		lastUsed: time.Now().Add(-retention),
	}
	active := &keyedResourceCacheEntry[string]{
		current: &keyedResourceCacheCall[string]{
			done: make(chan struct{}),
		},
		lastUsed: time.Now().Add(-retention),
	}

	cache.entries["stale"] = stale
	cache.entries["active"] = active

	_, err := cache.Get(
		context.Background(),
		"requested",
		func(cachedEntry[string], time.Time) bool { return true },
		func(context.Context) (string, error) { return "value", nil },
	)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	cache.mu.Lock()
	_, staleExists := cache.entries["stale"]
	_, activeExists := cache.entries["active"]
	_, requestedExists := cache.entries["requested"]
	cache.mu.Unlock()

	if staleExists {
		t.Fatal("stale entry was not evicted")
	}
	if !activeExists {
		t.Fatal("active entry was evicted")
	}
	if !requestedExists {
		t.Fatal("requested entry is missing")
	}
}

func TestKeyedResourceCacheFetchesDifferentKeysConcurrently(t *testing.T) {
	cache := newKeyedResourceCache[string, string](time.Hour)

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})

	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.Get(
			context.Background(),
			"first",
			func(cachedEntry[string], time.Time) bool { return true },
			func(context.Context) (string, error) {
				close(firstStarted)
				<-release
				return "first", nil
			},
		)
		firstDone <- err
	}()

	<-firstStarted

	secondDone := make(chan error, 1)
	go func() {
		_, err := cache.Get(
			context.Background(),
			"second",
			func(cachedEntry[string], time.Time) bool { return true },
			func(context.Context) (string, error) {
				close(secondStarted)
				<-release
				return "second", nil
			},
		)
		secondDone <- err
	}()

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("second key did not begin fetching while first key was in flight")
	}

	close(release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Get: %v", err)
	}
}
