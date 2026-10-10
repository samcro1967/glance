package glance

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

const (
	rssSortRarity         = "rarity"
	rssSortTier           = "tier"
	defaultRarityBase     = 72.0
	defaultRarityExponent = 2.5
	defaultFeedGapHours   = 24.0
)

// validateRSSSort rejects invalid sorting options before widget updates begin.
func validateRSSSort(mode string, base, exponent float64) error {
	switch mode {
	case "", "chronological", rssSortRarity, rssSortTier:
	default:
		return fmt.Errorf("invalid RSS sort %q: expected chronological, rarity, or tier", mode)
	}
	if math.IsNaN(base) || math.IsInf(base, 0) || math.IsNaN(exponent) || math.IsInf(exponent, 0) || base < 0 || exponent < 0 {
		return fmt.Errorf("rarity-base and rarity-exponent must be finite and nonnegative")
	}
	return nil
}

// sortByRarity weights article age by the median posting gap of its source feed.
// An absent or zero tuning value selects the upstream default.
func (f rssFeedItemList) sortByRarity(now time.Time, base, exponent float64) rssFeedItemList {
	if base == 0 {
		base = defaultRarityBase
	}
	if exponent == 0 {
		exponent = defaultRarityExponent
	}
	times := make(map[string][]time.Time)
	for _, item := range f {
		times[item.feedURL] = append(times[item.feedURL], item.PublishedAt)
	}
	gaps := make(map[string]float64, len(times))
	for url, ts := range times {
		gaps[url] = defaultFeedGapHours
		if len(ts) < 2 {
			continue
		}
		slices.SortFunc(ts, func(a, b time.Time) int { return a.Compare(b) })
		diffs := make([]float64, 0, len(ts)-1)
		for i := 1; i < len(ts); i++ {
			diffs = append(diffs, ts[i].Sub(ts[i-1]).Hours())
		}
		slices.Sort(diffs)
		gaps[url] = diffs[len(diffs)/2]
	}
	effectiveAge := func(item rssFeedItem) float64 {
		gap := max(gaps[item.feedURL], 0.1)
		multiplier := math.Pow(base/gap, exponent)
		multiplier = min(max(multiplier, 0.0001), 100.0)
		return now.Sub(item.PublishedAt).Hours() * multiplier
	}
	slices.SortStableFunc(f, func(a, b rssFeedItem) int {
		return cmp.Compare(effectiveAge(a), effectiveAge(b))
	})
	return f
}

// sortByTier orders configured tiers first and sorts newest first within each tier.
// Missing tiers are compared explicitly to avoid integer overflow.
func (f rssFeedItemList) sortByTier(tiers map[string]int) rssFeedItemList {
	slices.SortStableFunc(f, func(a, b rssFeedItem) int {
		ta, oka := tiers[a.feedURL]
		tb, okb := tiers[b.feedURL]
		if oka != okb {
			if oka {
				return -1
			}
			return 1
		}
		if oka {
			if c := cmp.Compare(ta, tb); c != 0 {
				return c
			}
		}
		return b.PublishedAt.Compare(a.PublishedAt)
	})
	return f
}
