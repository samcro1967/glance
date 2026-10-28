package glance

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestWidgetFreshnessRefreshesMonitorAndMarkets(t *testing.T) {
	t.Run("monitor", func(t *testing.T) {
		resetMonitorResourceCache(t)
		calls := 0
		wave3Transport(t, func(r *http.Request) (*http.Response, error) { calls++; return monitorResourceTestResponse(r), nil })
		w := &monitorWidget{Sites: []monitorSite{{SiteStatusRequest: &SiteStatusRequest{DefaultURL: "https://example.invalid"}}}}
		w.CustomCacheDuration = durationField(time.Minute)
		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}
		w.update(context.Background())
		key := monitorResourceRequestKey(w.Sites[0].SiteStatusRequest)
		monitorResourceCache.mu.Lock()
		e := monitorResourceCache.entries[key]
		monitorResourceCache.mu.Unlock()
		e.mu.Lock()
		e.cached.timestamp = time.Now().Add(-2 * time.Minute)
		e.mu.Unlock()
		w.update(context.Background())
		if calls != 2 {
			t.Fatalf("one-minute monitor reused old check; calls=%d", calls)
		}
	})
	t.Run("markets", func(t *testing.T) {
		resetYahooMarketResourceCache(t)
		calls := 0
		wave3Transport(t, func(r *http.Request) (*http.Response, error) {
			calls++
			return yahooMarketResourceTestResponse(r, "TEST"), nil
		})
		w := &marketsWidget{MarketRequests: []marketRequest{{Symbol: "TEST"}}}
		w.CustomCacheDuration = durationField(10 * time.Minute)
		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}
		w.update(context.Background())
		yahooMarketResourceCache.mu.Lock()
		e := yahooMarketResourceCache.entries[resourceTimeoutKey("TEST")]
		yahooMarketResourceCache.mu.Unlock()
		e.mu.Lock()
		e.cached.timestamp = time.Now().Add(-30 * time.Minute)
		e.mu.Unlock()
		w.update(context.Background())
		if calls != 2 {
			t.Fatalf("shorter market schedule reused old quote; calls=%d", calls)
		}
	})
}

func TestResourceFreshnessCronAndDefault(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	if !resourceFreshEnough(ctx, now.Add(-time.Minute), now, 5*time.Minute) {
		t.Fatal("default sharing changed")
	}
	base := &widgetBase{cacheType: cacheTypeCron}
	if resourceFreshEnough(withWidgetResourceFreshness(ctx, base), now, now, 5*time.Minute) {
		t.Fatal("cron check reused preexisting result")
	}
}
