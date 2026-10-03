package glance

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func resetEnvironmentResourceCaches(t *testing.T) {
	t.Helper()
	openMeteoEnvironmentResourceCache.mu.Lock()
	oldAir := openMeteoEnvironmentResourceCache.entries
	openMeteoEnvironmentResourceCache.entries = make(map[openMeteoEnvironmentResourceKey]*keyedResourceCacheEntry[*environmentAirQuality])
	openMeteoEnvironmentResourceCache.mu.Unlock()
	atmoSporePollenResourceCache.mu.Lock()
	oldPollen := atmoSporePollenResourceCache.entries
	atmoSporePollenResourceCache.entries = make(map[atmoSporeResourceKey]*keyedResourceCacheEntry[*environmentPollen])
	atmoSporePollenResourceCache.mu.Unlock()
	t.Cleanup(func() {
		openMeteoEnvironmentResourceCache.mu.Lock()
		openMeteoEnvironmentResourceCache.entries = oldAir
		openMeteoEnvironmentResourceCache.mu.Unlock()
		atmoSporePollenResourceCache.mu.Lock()
		atmoSporePollenResourceCache.entries = oldPollen
		atmoSporePollenResourceCache.mu.Unlock()
	})
}

func environmentTestPlace() *openMeteoPlaceResponseJson {
	return &openMeteoPlaceResponseJson{Name: "O Fallon", Area: "Missouri", Country: "United States", Latitude: 38.8106, Longitude: -90.6998, Timezone: "America/Chicago"}
}

func TestEnvironmentProviderParsing(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "air-quality-api.open-meteo.com":
			if got := r.URL.Query().Get("hourly"); got != "us_aqi,uv_index" {
				t.Fatalf("hourly=%q", got)
			}
			if got := r.URL.Query().Get("daily"); got != "" {
				t.Fatalf("unexpected daily=%q", got)
			}
			return wave3Response(http.StatusOK, `{"current":{"us_aqi":37,"european_aqi":25,"pm2_5":3.4,"pm10":3.8,"carbon_monoxide":144,"nitrogen_dioxide":6.3,"sulphur_dioxide":1.2,"ozone":69,"uv_index":0},"hourly":{"time":["2026-10-02T00:00","2026-10-02T12:00","2026-10-03T00:00","2026-10-03T12:00","2026-10-04T00:00","2026-10-04T12:00","2026-10-05T00:00"],"us_aqi":[38,42,null,null,null,null,0],"uv_index":[null,null,0,4.1,null,null,0]}}`, nil), nil
		case "pollenapi.com":
			if r.Header.Get("x-api-key") != "secret" {
				t.Fatal("missing pollen API key")
			}
			q := r.URL.Query()
			if q.Get("dt") == "" {
				t.Fatal("missing pollen dt")
			}
			if got := q.Get("forecast_days"); got != "7" {
				t.Fatalf("forecast_days=%q", got)
			}
			if got := q.Get("species"); got != "all" {
				t.Fatalf("species=%q", got)
			}
			return wave3Response(http.StatusOK, `{"data":[{"date":"2026-10-02","overall_risk":"moderate","species":{"bermuda_grass":{"value":12.035,"risk_level":"moderate","display_name":"Bermuda Grass","category":"grass"},"ragweed":{"value":0,"risk_level":"low","display_name":"Ragweed","category":"weed"}}},{"date":"2026-10-03","overall_risk":"low","species":{}},{"date":"2026-10-04","overall_risk":"low","species":{}},{"date":"2026-10-05","overall_risk":"low","species":{}},{"date":"2026-10-06","overall_risk":"low","species":{}},{"date":"2026-10-07","overall_risk":"low","species":{}},{"date":"2026-10-08","overall_risk":"low","species":{}},{"date":"2026-10-09","overall_risk":"low","species":{}}]}`, nil), nil
		default:
			t.Fatalf("unexpected host %q", r.URL.Host)
			return nil, nil
		}
	})
	air, err := fetchOpenMeteoEnvironment(context.Background(), environmentTestPlace())
	if err != nil ||
		air.USAQI != 37 ||
		air.PM25 != 3.4 ||
		len(air.Forecast) != 3 ||
		air.Forecast[0].USAQI != 42 ||
		!air.Forecast[0].HasUSAQI ||
		air.Forecast[0].HasUVIndex ||
		air.Forecast[1].UVIndex != 4.1 ||
		air.Forecast[1].HasUSAQI ||
		!air.Forecast[1].HasUVIndex ||
		air.Forecast[2].Date != "2026-10-05" ||
		air.Forecast[2].USAQI != 0 ||
		air.Forecast[2].UVIndex != 0 ||
		!air.Forecast[2].HasUSAQI ||
		!air.Forecast[2].HasUVIndex {
		t.Fatalf("air parse: air=%+v err=%v", air, err)
	}
	pollen, err := fetchAtmoSporePollen(context.Background(), environmentTestPlace(), "secret")
	if err != nil {
		t.Fatalf("pollen fetch: %v", err)
	}
	active := pollen.Current().ActiveSpecies()
	if len(pollen.Days) != 7 || pollen.Current().OverallRisk != "moderate" || len(active) != 1 || active[0].Name != "Bermuda Grass" {
		t.Fatalf("pollen parse: %+v", pollen.Current())
	}
}

func TestEnvironmentRejectsEmptyPollenForecast(t *testing.T) {
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		return wave3Response(http.StatusOK, `{"data":[]}`, nil), nil
	})
	if _, err := fetchAtmoSporePollen(context.Background(), environmentTestPlace(), "secret"); err == nil {
		t.Fatal("empty pollen forecast unexpectedly succeeded")
	}
}

func TestEnvironmentPartialRefreshPreservesStaleData(t *testing.T) {
	resetEnvironmentResourceCaches(t)
	wave3Transport(t, func(r *http.Request) (*http.Response, error) { return nil, errors.New("provider unavailable") })
	oldAir := &environmentAirQuality{USAQI: 37}
	oldPollen := &environmentPollen{Days: []environmentPollenDay{{Date: "2026-10-02", OverallRisk: "moderate"}}}
	w := &environmentWidget{Location: "O Fallon, Missouri, US", PollenAPIKey: "secret", Place: environmentTestPlace(), AirQuality: oldAir, Pollen: oldPollen}
	w.ContentAvailable = true
	w.update(context.Background())
	if w.AirQuality != oldAir || w.Pollen != oldPollen {
		t.Fatal("stale provider data was replaced")
	}
	if w.Error != nil || !errors.Is(w.Notice, errPartialContent) {
		t.Fatalf("error=%v notice=%v", w.Error, w.Notice)
	}
	if w.AirQualityError == nil || w.PollenError == nil {
		t.Fatal("provider failures were not retained")
	}
}

func TestEnvironmentAirOnlyAndRendering(t *testing.T) {
	resetEnvironmentResourceCaches(t)
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "pollenapi.com" {
			t.Fatal("pollen called without API key")
			return nil, nil
		}
		if r.URL.Host == "air-quality-api.open-meteo.com" {
			return wave3Response(http.StatusOK, `{"current":{"us_aqi":37,"european_aqi":25,"pm2_5":3.4,"pm10":3.8,"carbon_monoxide":144,"nitrogen_dioxide":6.3,"sulphur_dioxide":1.2,"ozone":69,"uv_index":0},"hourly":{"time":["2026-10-02T00:00","2026-10-02T12:00","2026-10-03T00:00","2026-10-03T12:00","2026-10-04T00:00","2026-10-04T12:00","2026-10-05T00:00"],"us_aqi":[38,42,31,35,null,null,0],"uv_index":[0,5.2,0,4.1,null,null,0]}}`, nil), nil
		}
		t.Fatalf("unexpected host %q", r.URL.Host)
		return nil, nil
	})
	w := &environmentWidget{Location: "O Fallon, Missouri, US", Place: environmentTestPlace()}
	w.update(context.Background())
	if w.Error != nil || w.Notice != nil || w.AirQuality == nil {
		t.Fatalf("air-only update error=%v notice=%v air=%+v", w.Error, w.Notice, w.AirQuality)
	}

	w.PollenAPIKey = "secret"
	w.Pollen = &environmentPollen{Days: []environmentPollenDay{{Date: "2026-10-02", OverallRisk: "moderate", Species: []environmentPollenSpecies{{Name: "Bermuda Grass", Value: 12, RiskLevel: "moderate"}}}}}
	w.PollenError = errors.New("temporary failure")
	w.ContentAvailable = true
	compact, expanded := string(w.Render()), string(w.RenderExpanded())
	for _, want := range []string{"Moderate", "showing last available data", "O Fallon, Missouri"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact missing %q", want)
		}
	}
	for _, want := range []string{"7-day air quality", "Fri 10/2", "AQI 42", "UV 5.2", "AtmoSpore"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded missing %q", want)
		}
	}
}

func TestEnvironmentFirstPollenFailureShowsUnavailable(t *testing.T) {
	resetEnvironmentResourceCaches(t)
	wave3Transport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "air-quality-api.open-meteo.com":
			return wave3Response(http.StatusOK, `{"current":{"us_aqi":37,"european_aqi":25,"pm2_5":3.4,"pm10":3.8,"carbon_monoxide":144,"nitrogen_dioxide":6.3,"sulphur_dioxide":1.2,"ozone":69,"uv_index":0},"hourly":{"time":["2026-10-02T00:00","2026-10-02T12:00","2026-10-03T00:00","2026-10-03T12:00","2026-10-04T00:00","2026-10-04T12:00","2026-10-05T00:00"],"us_aqi":[38,42,31,35,null,null,0],"uv_index":[0,5.2,0,4.1,null,null,0]}}`, nil), nil
		case "pollenapi.com":
			return nil, errors.New("pollen unavailable")
		default:
			t.Fatalf("unexpected host %q", r.URL.Host)
			return nil, nil
		}
	})

	w := &environmentWidget{
		Location:     "O Fallon, Missouri, US",
		PollenAPIKey: "secret",
		Place:        environmentTestPlace(),
	}
	w.ContentAvailable = true
	w.update(context.Background())

	if w.Error != nil || !errors.Is(w.Notice, errPartialContent) {
		t.Fatalf("error=%v notice=%v", w.Error, w.Notice)
	}
	if w.AirQuality == nil || w.Pollen != nil || w.PollenError == nil {
		t.Fatalf("air=%+v pollen=%+v pollenError=%v", w.AirQuality, w.Pollen, w.PollenError)
	}

	compact, expanded := string(w.Render()), string(w.RenderExpanded())
	if !strings.Contains(compact, "Pollen data unavailable") {
		t.Fatal("compact did not report unavailable pollen")
	}
	if strings.Contains(compact, "Pollen refresh failed; showing last available data") {
		t.Fatal("compact incorrectly reported stale pollen")
	}
	if !strings.Contains(expanded, "Pollen data unavailable.") {
		t.Fatal("expanded did not report unavailable pollen")
	}
	if strings.Contains(expanded, "Pollen refresh failed; showing last available data.") {
		t.Fatal("expanded incorrectly reported stale pollen")
	}
}

func TestEnvironmentRenderWithoutPlace(t *testing.T) {
	w := &environmentWidget{AirQuality: &environmentAirQuality{USAQI: 10}}
	w.ContentAvailable = true
	if w.Render() == "" || w.RenderExpanded() == "" || w.Error != nil {
		t.Fatalf("nil-place render failed: %v", w.Error)
	}
}

func TestEnvironmentForecastAvailabilityAndSpeciesOrdering(t *testing.T) {
	w := &environmentWidget{
		AirQuality: &environmentAirQuality{
			Forecast: []environmentAirQualityDay{
				{Date: "2026-10-02", USAQI: 42, HasUSAQI: true},
				{Date: "2026-10-03", UVIndex: 5.2, HasUVIndex: true},
				{Date: "2026-10-04", USAQI: 0, UVIndex: 0, HasUSAQI: true, HasUVIndex: true},
			},
		},
	}
	w.ContentAvailable = true

	expanded := string(w.RenderExpanded())

	if !strings.Contains(expanded, "AQI 42") {
		t.Fatal("available AQI missing")
	}
	if strings.Contains(expanded, "Fri 10/2</strong><span>AQI 42</span><span>UV 0.0") {
		t.Fatal("missing UV rendered as zero")
	}
	if !strings.Contains(expanded, "Sat 10/3</strong><span>UV 5.2") {
		t.Fatal("available UV missing")
	}
	if strings.Contains(expanded, "Sat 10/3</strong><span>AQI 0") {
		t.Fatal("missing AQI rendered as zero")
	}
	if !strings.Contains(expanded, "Sun 10/4</strong><span>AQI 0</span><span>UV 0.0") {
		t.Fatal("genuine zero measurements were not rendered")
	}

	day := &environmentPollenDay{
		Species: []environmentPollenSpecies{
			{Name: "Zeta", Value: 10},
			{Name: "Alpha", Value: 10},
			{Name: "Higher", Value: 20},
			{Name: "Inactive", Value: 0},
		},
	}
	active := day.ActiveSpecies()
	if len(active) != 3 ||
		active[0].Name != "Higher" ||
		active[1].Name != "Alpha" ||
		active[2].Name != "Zeta" {
		t.Fatalf("unexpected active species order: %+v", active)
	}
}

func TestEnvironmentDisplayHelpers(t *testing.T) {
	if got := environmentDisplayDate("2026-10-02"); got != "Fri 10/2" {
		t.Fatalf("date=%q", got)
	}
	if got := environmentDisplayRisk("very high"); got != "Very high" {
		t.Fatalf("risk=%q", got)
	}
	if got := environmentDisplayDate("unknown"); got != "unknown" {
		t.Fatalf("fallback date=%q", got)
	}
}
