package glance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type openMeteoEnvironmentResponse struct {
	Current struct {
		USAQI           float64 `json:"us_aqi"`
		EuropeanAQI     float64 `json:"european_aqi"`
		PM25            float64 `json:"pm2_5"`
		PM10            float64 `json:"pm10"`
		Ozone           float64 `json:"ozone"`
		NitrogenDioxide float64 `json:"nitrogen_dioxide"`
		SulphurDioxide  float64 `json:"sulphur_dioxide"`
		CarbonMonoxide  float64 `json:"carbon_monoxide"`
		UVIndex         float64 `json:"uv_index"`
	} `json:"current"`
	Hourly struct {
		Time    []string   `json:"time"`
		USAQI   []*float64 `json:"us_aqi"`
		UVIndex []*float64 `json:"uv_index"`
	} `json:"hourly"`
}

type openMeteoEnvironmentResourceKey struct {
	Latitude, Longitude float64
	Timezone            string
}

var openMeteoEnvironmentResourceCache = newKeyedResourceCache[openMeteoEnvironmentResourceKey, *environmentAirQuality](openMeteoResourceIdleRetention)

func fetchOpenMeteoEnvironmentResource(ctx context.Context, place *openMeteoPlaceResponseJson) (*environmentAirQuality, error) {
	key := openMeteoEnvironmentResourceKey{place.Latitude, place.Longitude, place.Timezone}
	return openMeteoEnvironmentResourceCache.Get(ctx, key, func(c cachedEntry[*environmentAirQuality], now time.Time) bool {
		return sameClockHour(c.timestamp, now)
	}, func(ctx context.Context) (*environmentAirQuality, error) {
		return fetchOpenMeteoEnvironment(ctx, place)
	})
}

func fetchOpenMeteoEnvironment(ctx context.Context, place *openMeteoPlaceResponseJson) (*environmentAirQuality, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%f", place.Latitude))
	q.Set("longitude", fmt.Sprintf("%f", place.Longitude))
	q.Set("timezone", place.Timezone)
	q.Set("forecast_days", "7")
	q.Set("current", "us_aqi,european_aqi,pm2_5,pm10,carbon_monoxide,nitrogen_dioxide,sulphur_dioxide,ozone,uv_index")
	q.Set("hourly", "us_aqi,uv_index")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://air-quality-api.open-meteo.com/v1/air-quality?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating air quality request: %w", err)
	}
	r, err := decodeJsonFromRequest[openMeteoEnvironmentResponse](defaultHTTPClient, req)
	if err != nil {
		return nil, fmt.Errorf("fetching air quality data: %w", err)
	}
	out := &environmentAirQuality{USAQI: r.Current.USAQI, EuropeanAQI: r.Current.EuropeanAQI, PM25: r.Current.PM25, PM10: r.Current.PM10, Ozone: r.Current.Ozone, NitrogenDioxide: r.Current.NitrogenDioxide, SulphurDioxide: r.Current.SulphurDioxide, CarbonMonoxide: r.Current.CarbonMonoxide, UVIndex: r.Current.UVIndex}
	for i, timestamp := range r.Hourly.Time {
		if len(timestamp) < len("2006-01-02") {
			continue
		}

		var aqi, uv *float64
		if i < len(r.Hourly.USAQI) {
			aqi = r.Hourly.USAQI[i]
		}
		if i < len(r.Hourly.UVIndex) {
			uv = r.Hourly.UVIndex[i]
		}
		if aqi == nil && uv == nil {
			continue
		}

		date := timestamp[:len("2006-01-02")]
		if len(out.Forecast) == 0 || out.Forecast[len(out.Forecast)-1].Date != date {
			if len(out.Forecast) == 7 {
				break
			}
			out.Forecast = append(out.Forecast, environmentAirQualityDay{Date: date})
		}

		day := &out.Forecast[len(out.Forecast)-1]
		if aqi != nil {
			if !day.HasUSAQI || *aqi > day.USAQI {
				day.USAQI = *aqi
			}
			day.HasUSAQI = true
		}
		if uv != nil {
			if !day.HasUVIndex || *uv > day.UVIndex {
				day.UVIndex = *uv
			}
			day.HasUVIndex = true
		}
	}
	return out, nil
}

type atmoSporeSpecies struct {
	Value       float64 `json:"value"`
	RiskLevel   string  `json:"risk_level"`
	DisplayName string  `json:"display_name"`
	Category    string  `json:"category"`
}

type atmoSporeResponse struct {
	Data []struct {
		Date        string                      `json:"date"`
		OverallRisk string                      `json:"overall_risk"`
		Species     map[string]atmoSporeSpecies `json:"species"`
	} `json:"data"`
}
type atmoSporeResourceKey struct {
	Latitude, Longitude float64
	APIKey              string
}

var atmoSporePollenResourceCache = newKeyedResourceCache[atmoSporeResourceKey, *environmentPollen](24 * time.Hour)

func fetchAtmoSporePollenResource(ctx context.Context, place *openMeteoPlaceResponseJson, apiKey string) (*environmentPollen, error) {
	key := atmoSporeResourceKey{place.Latitude, place.Longitude, apiKey}
	return atmoSporePollenResourceCache.Get(ctx, key, func(c cachedEntry[*environmentPollen], now time.Time) bool { return now.Sub(c.timestamp) < 6*time.Hour }, func(ctx context.Context) (*environmentPollen, error) { return fetchAtmoSporePollen(ctx, place, apiKey) })
}
func fetchAtmoSporePollen(ctx context.Context, place *openMeteoPlaceResponseJson, apiKey string) (*environmentPollen, error) {
	q := url.Values{}
	q.Set("lat", fmt.Sprintf("%f", place.Latitude))
	q.Set("lon", fmt.Sprintf("%f", place.Longitude))
	location, err := time.LoadLocation(place.Timezone)
	if err != nil {
		return nil, fmt.Errorf("loading pollen timezone: %w", err)
	}
	q.Set("dt", time.Now().In(location).Format("2006-01-02"))
	q.Set("forecast_days", "7")
	q.Set("species", "all")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://pollenapi.com/v1/pollen?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating pollen request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	r, err := decodeJsonFromRequest[atmoSporeResponse](defaultHTTPClient, req)
	if err != nil {
		return nil, fmt.Errorf("fetching pollen data: %w", err)
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("pollen provider returned no forecast data")
	}
	out := &environmentPollen{}
	for i, d := range r.Data {
		if i == 7 {
			break
		}
		day := environmentPollenDay{Date: d.Date, OverallRisk: d.OverallRisk}
		for speciesID, s := range d.Species {
			name := s.DisplayName
			if name == "" {
				name = speciesID
			}
			day.Species = append(day.Species, environmentPollenSpecies{Name: name, Category: s.Category, Value: s.Value, RiskLevel: s.RiskLevel})
		}
		out.Days = append(out.Days, day)
	}
	return out, nil
}
