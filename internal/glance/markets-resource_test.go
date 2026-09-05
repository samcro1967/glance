package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetYahooMarketResourceCache(t *testing.T) {
	t.Helper()

	yahooMarketResourceCache.Lock()
	oldEntries := yahooMarketResourceCache.entries
	yahooMarketResourceCache.entries = make(map[string]*yahooMarketResourceCacheEntry)
	yahooMarketResourceCache.Unlock()

	t.Cleanup(func() {
		yahooMarketResourceCache.Lock()
		yahooMarketResourceCache.entries = oldEntries
		yahooMarketResourceCache.Unlock()
	})
}

func yahooMarketResourceTestResponse(request *http.Request, symbol string) *http.Response {
	body := `{"chart":{"result":[{"meta":{"symbol":"` + symbol + `","currency":"USD","regularMarketPrice":123.45},"timestamp":[1],"indicators":{"quote":[{"close":[123.45]}]}}],"error":null}}`

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestYahooMarketResourceCachesSameSymbol(t *testing.T) {
	resetYahooMarketResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return yahooMarketResourceTestResponse(request, "NSIT"), nil
	})

	for i := 0; i < 2; i++ {
		if _, err := fetchYahooMarketResource(context.Background(), "NSIT"); err != nil {
			t.Fatalf("fetch %d: %v", i+1, err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestYahooMarketResourceDoesNotCacheFailure(t *testing.T) {
	resetYahooMarketResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("temporary failure")
		}
		return yahooMarketResourceTestResponse(request, "NSIT"), nil
	})

	if _, err := fetchYahooMarketResource(context.Background(), "NSIT"); err == nil {
		t.Fatal("first fetch unexpectedly succeeded")
	}
	if _, err := fetchYahooMarketResource(context.Background(), "NSIT"); err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}
}

func TestYahooMarketResourceCachesSymbolsIndependently(t *testing.T) {
	resetYahooMarketResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		symbol := strings.TrimPrefix(request.URL.Path, "/v8/finance/chart/")
		return yahooMarketResourceTestResponse(request, symbol), nil
	})

	for _, symbol := range []string{"NSIT", "^GSPC", "NSIT", "^GSPC"} {
		if _, err := fetchYahooMarketResource(context.Background(), symbol); err != nil {
			t.Fatalf("fetch %q: %v", symbol, err)
		}
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("HTTP calls = %d, want 2", got)
	}
}

func TestYahooMarketResourceCoalescesConcurrentFetches(t *testing.T) {
	resetYahooMarketResourceCache(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return yahooMarketResourceTestResponse(request, "NSIT"), nil
	})

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)

	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_, err := fetchYahooMarketResource(context.Background(), "NSIT")
			errs <- err
		}()
	}

	<-started
	time.Sleep(25 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("fetch error: %v", err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestYahooMarketResourceWaitingCallerCanCancel(t *testing.T) {
	resetYahooMarketResourceCache(t)

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		<-release
		return yahooMarketResourceTestResponse(request, "NSIT"), nil
	})

	go func() {
		_, err := fetchYahooMarketResource(context.Background(), "NSIT")
		firstDone <- err
	}()

	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := fetchYahooMarketResource(ctx, "NSIT"); err != context.Canceled {
		t.Fatalf("waiting fetch error = %v, want %v", err, context.Canceled)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("original fetch error: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1", got)
	}
}

func TestYahooMarketResourceEvictsIdleEntries(t *testing.T) {
	resetYahooMarketResourceCache(t)

	stale := &yahooMarketResourceCacheEntry{
		lastUsed: time.Now().Add(-yahooMarketResourceIdleRetention),
	}
	active := &yahooMarketResourceCacheEntry{
		lastUsed: time.Now().Add(-yahooMarketResourceIdleRetention),
		current: &yahooMarketResourceCall{
			done: make(chan struct{}),
		},
	}

	yahooMarketResourceCache.Lock()
	yahooMarketResourceCache.entries["STALE"] = stale
	yahooMarketResourceCache.entries["ACTIVE"] = active
	yahooMarketResourceCache.Unlock()

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return yahooMarketResourceTestResponse(request, "NSIT"), nil
	})

	if _, err := fetchYahooMarketResource(context.Background(), "NSIT"); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	yahooMarketResourceCache.Lock()
	_, staleExists := yahooMarketResourceCache.entries["STALE"]
	_, activeExists := yahooMarketResourceCache.entries["ACTIVE"]
	_, requestedExists := yahooMarketResourceCache.entries["NSIT"]
	yahooMarketResourceCache.Unlock()

	if staleExists {
		t.Fatal("idle stale entry was not evicted")
	}
	if !activeExists {
		t.Fatal("active stale entry was evicted")
	}
	if !requestedExists {
		t.Fatal("requested entry is missing")
	}
}

func TestYahooMarketResourceRefreshesLastUsed(t *testing.T) {
	resetYahooMarketResourceCache(t)

	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		return yahooMarketResourceTestResponse(request, "NSIT"), nil
	})

	before := time.Now()
	if _, err := fetchYahooMarketResource(context.Background(), "NSIT"); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	yahooMarketResourceCache.Lock()
	entry := yahooMarketResourceCache.entries["NSIT"]
	yahooMarketResourceCache.Unlock()

	entry.mu.Lock()
	lastUsed := entry.lastUsed
	entry.mu.Unlock()

	if lastUsed.Before(before) {
		t.Fatalf("last used = %v, want at or after %v", lastUsed, before)
	}
}

func TestMarketsShareResourceWithoutSharingWidgetConfiguration(t *testing.T) {
	resetYahooMarketResourceCache(t)

	var calls atomic.Int32
	wave3Transport(t, func(request *http.Request) (*http.Response, error) {
		calls.Add(1)

		body := `{"chart":{"result":[{"meta":{"currency":"USD","symbol":"NSIT","regularMarketPrice":152.28,"shortName":"Insight Enterprises","priceHint":2},"indicators":{"quote":[{"close":[150.00,152.28]}]}}],"error":null}}`

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})

	firstRequests := []marketRequest{
		{
			Symbol:     "NSIT",
			CustomName: "First Name",
			SymbolLink: "https://first.example/symbol",
			ChartLink:  "https://first.example/chart",
		},
	}

	secondRequests := []marketRequest{
		{
			Symbol:     "NSIT",
			CustomName: "Second Name",
			SymbolLink: "https://second.example/symbol",
			ChartLink:  "https://second.example/chart",
		},
	}

	first, err := fetchMarketsDataFromYahoo(context.Background(), firstRequests)
	if err != nil {
		t.Fatalf("first widget fetch: %v", err)
	}

	second, err := fetchMarketsDataFromYahoo(context.Background(), secondRequests)
	if err != nil {
		t.Fatalf("second widget fetch: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("Yahoo HTTP calls = %d, want 1", got)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("unexpected market counts: first=%d second=%d", len(first), len(second))
	}

	if first[0].Name != "First Name" {
		t.Fatalf("first name = %q, want %q", first[0].Name, "First Name")
	}
	if second[0].Name != "Second Name" {
		t.Fatalf("second name = %q, want %q", second[0].Name, "Second Name")
	}

	if first[0].SymbolLink != "https://first.example/symbol" {
		t.Fatalf("first symbol link = %q", first[0].SymbolLink)
	}
	if second[0].SymbolLink != "https://second.example/symbol" {
		t.Fatalf("second symbol link = %q", second[0].SymbolLink)
	}

	if first[0].ChartLink != "https://first.example/chart" {
		t.Fatalf("first chart link = %q", first[0].ChartLink)
	}
	if second[0].ChartLink != "https://second.example/chart" {
		t.Fatalf("second chart link = %q", second[0].ChartLink)
	}
}
