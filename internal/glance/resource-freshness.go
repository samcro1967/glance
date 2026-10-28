package glance

import (
	"context"
	"time"
)

type resourceFreshnessContextKey struct{}

// Caller freshness changes cache validity, not request identity. Equivalent
// requests can still share in-flight work even when their widgets use different
// schedules. A cron-triggered check requests fresh data at its scheduled time.
func withWidgetResourceFreshness(ctx context.Context, base *widgetBase) context.Context {
	if base.cacheType == cacheTypeCron {
		return context.WithValue(ctx, resourceFreshnessContextKey{}, time.Duration(0))
	}
	if base.cacheType == cacheTypeDuration && base.cacheDuration > 0 {
		return context.WithValue(ctx, resourceFreshnessContextKey{}, base.cacheDuration)
	}
	return ctx
}

func resourceFreshEnough(ctx context.Context, fetched, now time.Time, defaultAge time.Duration) bool {
	if requested, ok := ctx.Value(resourceFreshnessContextKey{}).(time.Duration); ok && requested < defaultAge {
		defaultAge = requested
	}
	return defaultAge > 0 && now.Sub(fetched) < defaultAge
}
