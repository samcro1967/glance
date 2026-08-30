package glance

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

func wave3Response(status int, body string, headers map[string]string) *http.Response {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func wave3Transport(t *testing.T, fn priorityRoundTripper) {
	t.Helper()
	old := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = fn
	t.Cleanup(func() { defaultHTTPClient.Transport = old })
}

func TestComprehensiveHackerNewsFetchSuccessPartialAndTemplate(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v0/topstories.json":
			return wave3Response(200, `[1,2,3]`, nil), nil
		case "/v0/item/1.json":
			return wave3Response(200, `{"id":1,"score":9,"title":"One","url":"https://example.invalid/a","descendants":4,"time":100}`, nil), nil
		case "/v0/item/2.json":
			return wave3Response(200, `{"id":2,"score":5,"title":"Two","descendants":2,"time":200}`, nil), nil
		case "/v0/item/3.json":
			return wave3Response(500, `bad`, nil), nil
		default:
			t.Fatalf("unexpected URL %s", r.URL.String())
			return nil, nil
		}
	})
	posts, err := fetchHackerNewsPosts(context.Background(), "top", 3, "https://comments.invalid/{POST-ID}")
	if len(posts) != 2 || !errors.Is(err, errPartialContent) {
		t.Fatalf("posts=%#v err=%v", posts, err)
	}
	if posts[0].DiscussionUrl != "https://comments.invalid/1" || posts[0].TargetUrlDomain != "example.invalid" {
		t.Fatalf("post=%#v", posts[0])
	}
	if posts[1].DiscussionUrl != "https://comments.invalid/2" {
		t.Fatalf("post=%#v", posts[1])
	}
}

func TestComprehensiveHackerNewsFetchNoContent(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) { return wave3Response(500, `bad`, nil), nil })
	_, err := fetchHackerNewsPostsFromIds(context.Background(), []int{1, 2}, "")
	if !errors.Is(err, errNoContent) {
		t.Fatalf("err=%v", err)
	}
}

func TestComprehensiveLobstersFeedAndURLSelection(t *testing.T) {
	var got string
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		got = r.URL.String()
		return wave3Response(200, `[{"created_at":"2026-08-30T12:00:00Z","title":"Post","url":"https://example.invalid/story","score":7,"comment_count":3,"comments_url":"https://lob.example/c/1","tags":["go"]}]`, nil), nil
	})
	posts, err := fetchLobstersPosts(context.Background(), "", "https://lob.example/", "new", []string{"go", "linux"})
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts=%#v err=%v", posts, err)
	}
	if got != "https://lob.example/t/go,linux.json" || posts[0].TargetUrlDomain != "example.invalid" {
		t.Fatalf("got=%q post=%#v", got, posts[0])
	}
}

func TestComprehensiveLobstersCustomEmptyAndBadTime(t *testing.T) {
	calls := 0
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return wave3Response(200, `[{"created_at":"bad","title":"Post","url":"https://example.invalid"}]`, nil), nil
		}
		return wave3Response(200, `[]`, nil), nil
	})
	posts, err := fetchLobstersPosts(context.Background(), "https://custom.invalid/feed", "", "hot", nil)
	if err != nil || len(posts) != 1 || !posts[0].TimePosted.IsZero() {
		t.Fatalf("posts=%#v err=%v", posts, err)
	}
	_, err = fetchLobstersPostsFromFeed(context.Background(), "https://custom.invalid/empty")
	if !errors.Is(err, errNoContent) {
		t.Fatalf("err=%v", err)
	}
}

func TestComprehensiveMarketsYahooSuccessPartialAndNoData(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "GOOD") {
			return wave3Response(200, `{"chart":{"result":[{"meta":{"currency":"USD","symbol":"GOOD","regularMarketPrice":110,"chartPreviousClose":90,"shortName":"Good Co","priceHint":2},"indicators":{"quote":[{"close":[90,100,110]}]}}]}}`, nil), nil
		}
		return wave3Response(200, `{"chart":{"result":[]}}`, nil), nil
	})
	markets, err := fetchMarketsDataFromYahoo(context.Background(), []marketRequest{{Symbol: "GOOD", CustomName: "Custom"}, {Symbol: "EMPTY"}})
	if len(markets) != 1 || !errors.Is(err, errPartialContent) {
		t.Fatalf("markets=%#v err=%v", markets, err)
	}
	if markets[0].Name != "Custom" || markets[0].CurrencySymbol != "$" || math.Abs(markets[0].PercentChange-10) > 1e-9 {
		t.Fatalf("market=%#v", markets[0])
	}
	_, err = fetchMarketsDataFromYahoo(context.Background(), []marketRequest{{Symbol: "EMPTY"}})
	if !errors.Is(err, errNoContent) {
		t.Fatalf("err=%v", err)
	}
}

func TestComprehensiveMarketsSorting(t *testing.T) {
	a := marketList{{Name: "a", PercentChange: -9}, {Name: "b", PercentChange: 5}, {Name: "c", PercentChange: 12}}
	a.sortByAbsChange()
	if a[0].Name != "c" || a[1].Name != "a" {
		t.Fatalf("abs=%#v", a)
	}
	a.sortByChange()
	if a[0].Name != "c" || a[2].Name != "a" {
		t.Fatalf("change=%#v", a)
	}
}

func TestComprehensiveWeatherGeocodeSuccessAreaAndErrors(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query().Get("name")
		switch q {
		case "City, United States":
			return wave3Response(200, `{"Results":[{"Name":"City","admin1":"Other","Latitude":1,"Longitude":2,"Timezone":"UTC","Country":"United States"},{"Name":"City","admin1":"Target","Latitude":3,"Longitude":4,"Timezone":"UTC","Country":"United States"}]}`, nil), nil
		case "None":
			return wave3Response(200, `{"Results":[]}`, nil), nil
		case "BadTZ":
			return wave3Response(200, `{"Results":[{"Name":"BadTZ","Timezone":"Not/AZone"}]}`, nil), nil
		default:
			return wave3Response(200, `{"Results":[{"Name":"City","admin1":"Other","Timezone":"UTC"}]}`, nil), nil
		}
	})
	p, err := fetchOpenMeteoPlaceFromName(context.Background(), "City, Target, US")
	if err != nil || p.Area != "Target" || p.location == nil {
		t.Fatalf("place=%#v err=%v", p, err)
	}
	if _, err := fetchOpenMeteoPlaceFromName(context.Background(), "None"); err == nil {
		t.Fatal("expected no places")
	}
	if _, err := fetchOpenMeteoPlaceFromName(context.Background(), "City, Missing, US"); err == nil {
		t.Fatal("expected area miss")
	}
	if _, err := fetchOpenMeteoPlaceFromName(context.Background(), "BadTZ"); err == nil {
		t.Fatal("expected timezone error")
	}
}

func TestComprehensiveWeatherForecastMetricImperialAndFlat(t *testing.T) {
	var units []string
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		units = append(units, r.URL.Query().Get("temperature_unit"))
		temps := make([]string, 24)
		precip := make([]string, 24)
		for i := range temps {
			temps[i] = "20"
			precip[i] = "80"
		}
		body := `{"daily":{"sunrise":[0],"sunset":[3600]},"hourly":{"temperature_2m":[` + strings.Join(temps, ",") + `],"precipitation_probability":[` + strings.Join(precip, ",") + `]},"current":{"temperature_2m":20,"apparent_temperature":19,"weather_code":1}}`
		return wave3Response(200, body, nil), nil
	})
	place := &openMeteoPlaceResponseJson{Latitude: 1, Longitude: 2, Timezone: "UTC", location: time.UTC}
	for _, u := range []string{"metric", "imperial"} {
		w, err := fetchWeatherForOpenMeteoPlace(context.Background(), place, u)
		if err != nil || len(w.Columns) != 12 || w.Columns[0].Scale != 1 || !w.Columns[0].HasPrecipitation || w.WeatherCodeAsString() != "Mainly Clear" {
			t.Fatalf("weather=%#v err=%v", w, err)
		}
	}
	if units[0] != "celsius" || units[1] != "fahrenheit" {
		t.Fatalf("units=%v", units)
	}
	if (&weather{WeatherCode: 999}).WeatherCodeAsString() != "" {
		t.Fatal("unknown weather code")
	}
}
