package glance

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkKeyedResourceCacheHit(b *testing.B) {
	for _, entryCount := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("entries_%d", entryCount), func(b *testing.B) {
			cache := newKeyedResourceCache[int, int](time.Hour)
			now := time.Now()

			for i := 0; i < entryCount; i++ {
				cache.entries[i] = &keyedResourceCacheEntry[int]{
					cached: cachedEntry[int]{
						value:     i,
						timestamp: now,
					},
					hasValue: true,
					lastUsed: now,
				}
			}

			ctx := context.Background()
			valid := func(cachedEntry[int], time.Time) bool { return true }
			fetch := func(context.Context) (int, error) {
				b.Fatal("fetch called during cache-hit benchmark")
				return 0, nil
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if _, err := cache.Get(ctx, 0, valid, fetch); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
