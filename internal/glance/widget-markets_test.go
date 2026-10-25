package glance

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestMarketsMissingQuoteSeries(t *testing.T) {
	for _, indicators := range []string{`{}`, `{"quote":null}`, `{"quote":[]}`} {
		t.Run(indicators, func(t *testing.T) {
			resetYahooMarketResourceCache(t)
			wave3Transport(t, func(request *http.Request) (*http.Response, error) {
				if strings.Contains(request.URL.Path, "GOOD") {
					return yahooMarketResourceTestResponse(request, "GOOD"), nil
				}
				return wave3Response(200, `{"chart":{"result":[{"meta":{"regularMarketPrice":123},"indicators":`+indicators+`}]}}`, nil), nil
			})
			markets, err := fetchMarketsDataFromYahoo(context.Background(), []marketRequest{{Symbol: "BAD"}, {Symbol: "GOOD"}})
			if len(markets) != 1 || markets[0].Symbol != "GOOD" || !errors.Is(err, errPartialContent) {
				t.Fatalf("mixed result = %#v, error = %v", markets, err)
			}
			markets, err = fetchMarketsDataFromYahoo(context.Background(), []marketRequest{{Symbol: "BAD"}})
			if markets != nil || !errors.Is(err, errNoContent) || !strings.Contains(err.Error(), "no quote series") {
				t.Fatalf("failed result = %#v, error = %v", markets, err)
			}
		})
	}
}

func TestMarketPreviousClose(t *testing.T) {
	tests := []struct {
		name          string
		current       float64
		previousClose float64
		prices        []float64
		want          float64
	}{
		{
			name:          "prefers explicit previous close",
			current:       105,
			previousClose: 101,
			prices:        []float64{95, 99, 105},
			want:          101,
		},
		{
			name:    "uses latest nonzero historical close",
			current: 105,
			prices:  []float64{95, 99, 0, 105},
			want:    99,
		},
		{
			name:    "falls back to current price",
			current: 105,
			prices:  []float64{0, 0, 105},
			want:    105,
		},
		{
			name:    "handles sparse price history",
			current: 105,
			prices:  []float64{105},
			want:    105,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := marketPreviousClose(test.current, test.previousClose, test.prices)
			if got != test.want {
				t.Fatalf("marketPreviousClose() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMarketsFetchCancellationPreservesClassificationAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	markets, err := fetchMarketsDataFromYahoo(
		ctx,
		[]marketRequest{
			{Symbol: "TEST"},
		},
	)
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error does not preserve context cancellation: %v", err)
	}

	if markets != nil {
		t.Fatalf("markets = %#v, want nil", markets)
	}

	expected := "failed to retrieve any content: fetching market data: context canceled"
	if err.Error() != expected {
		t.Fatalf(
			"unexpected cancellation error:\n got: %q\nwant: %q",
			err.Error(),
			expected,
		)
	}
}

func TestMarketsFetchEmptyRequestsReturnsNoContent(t *testing.T) {
	markets, err := fetchMarketsDataFromYahoo(
		context.Background(),
		nil,
	)
	if err == nil {
		t.Fatal("expected no-content error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if markets != nil {
		t.Fatalf("markets = %#v, want nil", markets)
	}

	expected := "failed to retrieve any content: failed 0 of 0 markets"
	if err.Error() != expected {
		t.Fatalf(
			"unexpected empty-market error:\n got: %q\nwant: %q",
			err.Error(),
			expected,
		)
	}
}

func TestMarketsFetchMalformedSymbolPreservesRequestFailure(t *testing.T) {
	const malformedSymbol = "invalid\x00symbol"

	markets, err := fetchMarketsDataFromYahoo(
		context.Background(),
		[]marketRequest{
			{Symbol: malformedSymbol},
		},
	)
	if err == nil {
		t.Fatal("expected request-construction error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if errors.Is(err, errPartialContent) {
		t.Fatalf("no-content error unexpectedly classified as partial-content: %v", err)
	}

	if markets != nil {
		t.Fatalf("markets = %#v, want nil", markets)
	}

	if !strings.Contains(err.Error(), "failed 1 of 1 markets") {
		t.Fatalf("error missing failure count: %v", err)
	}

	if !strings.Contains(err.Error(), "first failure: creating market request:") {
		t.Fatalf("error missing representative request failure: %v", err)
	}
}

func TestMarketsFetchMalformedSymbolStopsBeforeRemainingRequests(t *testing.T) {
	const malformedSymbol = "invalid\x00symbol"

	markets, err := fetchMarketsDataFromYahoo(
		context.Background(),
		[]marketRequest{
			{Symbol: malformedSymbol},
			{Symbol: "TEST"},
		},
	)
	if err == nil {
		t.Fatal("expected request-construction error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if markets != nil {
		t.Fatalf("markets = %#v, want nil", markets)
	}

	if !strings.Contains(err.Error(), "failed 1 of 2 markets") {
		t.Fatalf("error does not accurately describe attempted failures: %v", err)
	}

	if !strings.Contains(err.Error(), "first failure: creating market request:") {
		t.Fatalf("error missing representative request failure: %v", err)
	}
}
