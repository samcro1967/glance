package glance

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	yahooMarketResourceCacheDuration = time.Hour
	yahooMarketResourceIdleRetention = 24 * time.Hour
)

var yahooMarketResourceCache = newKeyedResourceCache[string, marketResponseJson](
	yahooMarketResourceIdleRetention,
)

func fetchYahooMarketResource(ctx context.Context, symbol string) (marketResponseJson, error) {
	return yahooMarketResourceCache.Get(
		ctx,
		symbol,
		func(cached cachedEntry[marketResponseJson], _ time.Time) bool {
			return time.Since(cached.timestamp) < yahooMarketResourceCacheDuration
		},
		func(ctx context.Context) (marketResponseJson, error) {
			return fetchYahooMarketResourceUncached(ctx, symbol)
		},
	)
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
