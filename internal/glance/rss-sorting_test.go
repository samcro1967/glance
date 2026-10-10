package glance

import (
	"math"
	"testing"
	"time"
)

func TestRSSSortValidation(t *testing.T) {
	for _, tc := range []struct {
		mode           string
		base, exponent float64
		invalid        bool
	}{
		{"", 0, 0, false}, {"chronological", 0, 0, false}, {"rarity", 72, 2.5, false}, {"tier", 0, 0, false},
		{"unknown", 0, 0, true}, {"rarity", -1, 0, true}, {"rarity", 0, -1, true},
		{"rarity", math.NaN(), 0, true}, {"rarity", math.Inf(1), 0, true},
	} {
		if got := validateRSSSort(tc.mode, tc.base, tc.exponent); (got != nil) != tc.invalid {
			t.Errorf("%+v: %v", tc, got)
		}
	}
}

func TestRSSSortTierAndUntiered(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := rssFeedItemList{
		{feedURL: "none", PublishedAt: now}, {feedURL: "b", PublishedAt: now},
		{feedURL: "a", PublishedAt: now.Add(-time.Hour)}, {feedURL: "a", PublishedAt: now},
	}
	items.sortByTier(map[string]int{"a": -1, "b": math.MaxInt})
	for i, expected := range []string{"a", "a", "b", "none"} {
		if items[i].feedURL != expected {
			t.Fatalf("position %d: got %s, want %s", i, items[i].feedURL, expected)
		}
	}
	if items[0].PublishedAt.Before(items[1].PublishedAt) {
		t.Fatal("tier order not newest first")
	}
}

func TestRSSSortRarityAndSingleItemFallback(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	items := rssFeedItemList{}
	for i := 2; i < 7; i++ {
		items = append(items, rssFeedItem{feedURL: "busy", PublishedAt: now.Add(-time.Duration(i) * time.Hour)})
	}
	items = append(items, rssFeedItem{feedURL: "rare", PublishedAt: now.Add(-72 * time.Hour)}, rssFeedItem{feedURL: "rare", PublishedAt: now.Add(-312 * time.Hour)})
	items.sortByRarity(now, 0, 0)
	if items[0].feedURL != "rare" {
		t.Fatalf("rare feed not boosted: %+v", items)
	}
	single := rssFeedItemList{{feedURL: "a", PublishedAt: now.Add(-5 * time.Hour)}, {feedURL: "b", PublishedAt: now.Add(-time.Hour)}}
	single.sortByRarity(now, 0, 0)
	if single[0].feedURL != "b" {
		t.Fatal("single item fallback must preserve newest-first ordering")
	}
}

func TestRSSDuplicateFeedTierValidation(t *testing.T) {
	tier := func(value int) *int { return &value }

	tests := []struct {
		name     string
		first    *int
		second   *int
		wantErr  bool
		wantTier int
		hasTier  bool
	}{
		{name: "same explicit tier", first: tier(1), second: tier(1), wantTier: 1, hasTier: true},
		{name: "conflicting explicit tiers", first: tier(1), second: tier(2), wantErr: true},
		{name: "both omitted"},
		{name: "first explicit", first: tier(1), wantTier: 1, hasTier: true},
		{name: "second explicit", second: tier(1), wantTier: 1, hasTier: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			widget := &rssWidget{
				FeedRequests: []rssFeedRequest{
					{URL: "https://example.com/feed", Tier: tt.first},
					{URL: "https://example.com/feed", Tier: tt.second},
				},
			}

			err := widget.initialize()
			if (err != nil) != tt.wantErr {
				t.Fatalf("initialize() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			got, ok := widget.feedTiers["https://example.com/feed"]
			if ok != tt.hasTier || (ok && got != tt.wantTier) {
				t.Fatalf("feed tier = (%d, %t), want (%d, %t)", got, ok, tt.wantTier, tt.hasTier)
			}
		})
	}
}
