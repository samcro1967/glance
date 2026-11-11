package glance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFetchTopGamesFromTwitchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchTopGamesFromTwitch(ctx, nil, 10)
	if err == nil {
		t.Fatal("expected canceled Twitch top games request to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestFetchTopGamesFromTwitchRequestLimit(t *testing.T) {
	for _, tc := range []struct {
		name           string
		limit          int
		excluded       int
		wantRequest    int
		wantCategories int
	}{
		{"below cap", 10, 5, 15, 10},
		{"at cap", 10, 90, 100, 10},
		{"above cap", 10, 95, 100, 5},
		{"large display limit", 150, 0, 100, 100},
		{"all returned categories excluded", 10, 100, 100, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requested int
			remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
				var body []struct {
					Variables struct {
						Limit int `json:"limit"`
					} `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 {
					t.Fatalf("invalid request: body=%#v err=%v", body, err)
				}
				requested = body[0].Variables.Limit
				if requested < 1 || requested > 100 {
					return remainingValueResponse(r, http.StatusOK, "application/json",
						`[{"errors":[{"message":"argument 'first' value must be between 1 and 100."}],"data":{"directoriesWithTags":null}}]`), nil
				}

				// Model Twitch's request ceiling rather than returning a fixed fixture.
				edges := make([]string, 0, requested)
				for i := range requested {
					edges = append(edges, fmt.Sprintf(`{"node":{"slug":"game-%d","name":"Game %d"}}`, i, i))
				}
				return remainingValueResponse(r, http.StatusOK, "application/json",
					`[{"data":{"directoriesWithTags":{"edges":[`+strings.Join(edges, ",")+`]}}}]`), nil
			})

			exclude := make([]string, tc.excluded)
			for i := range exclude {
				exclude[i] = fmt.Sprintf("game-%d", i)
			}
			categories, err := fetchTopGamesFromTwitch(context.Background(), exclude, tc.limit)
			if err != nil || requested != tc.wantRequest || len(categories) != tc.wantCategories {
				t.Fatalf("requested=%d categories=%d err=%v; want requested=%d categories=%d", requested, len(categories), err, tc.wantRequest, tc.wantCategories)
			}
			for i, category := range categories {
				if want := fmt.Sprintf("game-%d", tc.excluded+i); category.Slug != want {
					t.Fatalf("category[%d]=%q, want %q", i, category.Slug, want)
				}
			}
		})
	}
}

func TestTwitchTopGamesGraphQLErrorPreservesContentAndRecovers(t *testing.T) {
	failed := true
	remainingValueTransport(t, func(r *http.Request) (*http.Response, error) {
		body := `[{"data":{"directoriesWithTags":{"edges":[{"node":{"slug":"recovered","name":"Recovered"}}]}}}]`
		if failed {
			body = `[{"errors":[{"message":"PersistedQueryNotFound"}],"data":{"directoriesWithTags":null}}]`
		}
		return remainingValueResponse(r, http.StatusOK, "application/json", body), nil
	})

	w := &twitchGamesWidget{Categories: []twitchCategory{{Slug: "previous"}}}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.ContentAvailable = true
	w.update(context.Background())
	if w.Error == nil || !strings.Contains(w.Error.Error(), "PersistedQueryNotFound") {
		t.Fatalf("expected provider error, got %v", w.Error)
	}
	if len(w.Categories) != 1 || w.Categories[0].Slug != "previous" {
		t.Fatalf("failed refresh replaced previous content: %#v", w.Categories)
	}
	failed = false
	w.update(context.Background())
	if w.Error != nil || len(w.Categories) != 1 || w.Categories[0].Slug != "recovered" {
		t.Fatalf("recovery: categories=%#v err=%v", w.Categories, w.Error)
	}
}
