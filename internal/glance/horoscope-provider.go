package glance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const defaultHoroscopeProvider = "free-horoscope-api"

var freeHoroscopeAPIBaseURL = "https://freehoroscopeapi.com/api/v1/get-horoscope"

type horoscopeProvider interface {
	fetch(context.Context, string, string) (horoscopeReading, error)
}

type horoscopeReading struct {
	Date      string
	Period    string
	Sign      string
	Horoscope string
}

type freeHoroscopeAPIProvider struct{}

type freeHoroscopeAPIResponse struct {
	Data struct {
		Date      string `json:"date"`
		Period    string `json:"period"`
		Sign      string `json:"sign"`
		Horoscope string `json:"horoscope"`
	} `json:"data"`
}

func newHoroscopeProvider(name string) (horoscopeProvider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", defaultHoroscopeProvider:
		return freeHoroscopeAPIProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown horoscope provider %q; valid providers are %s", name, defaultHoroscopeProvider)
	}
}

func (freeHoroscopeAPIProvider) fetch(ctx context.Context, period, sign string) (horoscopeReading, error) {
	endpoint := fmt.Sprintf(
		"%s/%s?sign=%s",
		strings.TrimRight(freeHoroscopeAPIBaseURL, "/"),
		url.PathEscape(period),
		url.QueryEscape(sign),
	)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return horoscopeReading{}, fmt.Errorf("creating horoscope request: %w", err)
	}

	response, err := decodeJsonFromRequest[freeHoroscopeAPIResponse](defaultHTTPClient, request)
	if err != nil {
		return horoscopeReading{}, fmt.Errorf("fetching %s horoscope for %s: %w", period, sign, err)
	}

	if strings.TrimSpace(response.Data.Horoscope) == "" {
		return horoscopeReading{}, fmt.Errorf("fetching %s horoscope for %s: provider returned empty horoscope", period, sign)
	}

	return horoscopeReading{
		Date:      response.Data.Date,
		Period:    response.Data.Period,
		Sign:      response.Data.Sign,
		Horoscope: response.Data.Horoscope,
	}, nil
}
