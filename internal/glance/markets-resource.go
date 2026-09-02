package glance

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const yahooMarketResourceCacheDuration = time.Hour

type yahooMarketResourceCall struct {
	done chan struct{}
	val  marketResponseJson
	err  error
}

type yahooMarketResourceCacheEntry struct {
	mu      sync.Mutex
	cached  cachedEntry[marketResponseJson]
	current *yahooMarketResourceCall
}

var yahooMarketResourceCache = struct {
	sync.Mutex
	entries map[string]*yahooMarketResourceCacheEntry
}{
	entries: make(map[string]*yahooMarketResourceCacheEntry),
}

func fetchYahooMarketResource(ctx context.Context, symbol string) (marketResponseJson, error) {
	yahooMarketResourceCache.Lock()
	entry, ok := yahooMarketResourceCache.entries[symbol]
	if !ok {
		entry = &yahooMarketResourceCacheEntry{}
		yahooMarketResourceCache.entries[symbol] = entry
	}
	yahooMarketResourceCache.Unlock()

	return entry.fetch(ctx, symbol)
}

func (entry *yahooMarketResourceCacheEntry) fetch(ctx context.Context, symbol string) (marketResponseJson, error) {
	entry.mu.Lock()

	if !entry.cached.timestamp.IsZero() && time.Since(entry.cached.timestamp) < yahooMarketResourceCacheDuration {
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
			return marketResponseJson{}, ctx.Err()
		}
	}

	call := &yahooMarketResourceCall{done: make(chan struct{})}
	entry.current = call
	entry.mu.Unlock()

	call.val, call.err = fetchYahooMarketResourceUncached(ctx, symbol)

	entry.mu.Lock()
	if call.err == nil {
		entry.cached = cachedEntry[marketResponseJson]{
			value:     call.val,
			timestamp: time.Now(),
		}
	}
	entry.current = nil
	close(call.done)
	entry.mu.Unlock()

	return call.val, call.err
}

func fetchYahooMarketResourceUncached(ctx context.Context, symbol string) (marketResponseJson, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(
			"https://query1.finance.yahoo.com/v8/finance/chart/%s?range=1mo&interval=1d",
			symbol,
		),
		nil,
	)
	if err != nil {
		return marketResponseJson{}, fmt.Errorf("creating market request: %w", err)
	}

	setBrowserUserAgentHeader(request)

	return decodeJsonFromRequest[marketResponseJson](defaultHTTPClient, request)
}
