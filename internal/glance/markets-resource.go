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

func fetchYahooMarketResource(ctx context.Context, symbol string, timeouts ...durationField) (marketResponseJson, error) {
	return yahooMarketResourceCache.Get(
		ctx,
		resourceTimeoutKey(symbol, timeouts...),
		func(cached cachedEntry[marketResponseJson], now time.Time) bool {
			return resourceFreshEnough(ctx, cached.timestamp, now, yahooMarketResourceCacheDuration)
		},
		func(ctx context.Context) (marketResponseJson, error) {
			return fetchYahooMarketResourceUncached(ctx, symbol, timeouts...)
		},
	)
}

func fetchYahooMarketResourceUncached(ctx context.Context, symbol string, timeouts ...durationField) (marketResponseJson, error) {
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

	return decodeJsonFromRequest[marketResponseJson](newProviderHTTPClient(timeouts...), request)
}
