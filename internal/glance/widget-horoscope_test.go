package glance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHoroscopeWidgetInitialize(t *testing.T) {
	widget := &horoscopeWidget{Signs: []string{"Cancer", "libra"}}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if widget.Title != "Horoscope" {
		t.Fatalf("unexpected title: %q", widget.Title)
	}
	if widget.Provider != defaultHoroscopeProvider {
		t.Fatalf("provider = %q, want %q", widget.Provider, defaultHoroscopeProvider)
	}
	if len(widget.Periods) != 1 || widget.Periods[0] != "daily" {
		t.Fatalf("periods = %#v, want daily", widget.Periods)
	}
	if widget.cacheType != cacheTypeCron {
		t.Fatalf("cache type = %v, want cron", widget.cacheType)
	}
}

func TestHoroscopeWidgetValidation(t *testing.T) {
	tests := []horoscopeWidget{
		{},
		{Signs: []string{"ophiuchus"}},
		{Signs: []string{"cancer"}, Periods: []string{"yearly"}},
		{Signs: []string{"cancer"}, Provider: "unknown"},
	}
	for i := range tests {
		if err := tests[i].initialize(); err == nil {
			t.Fatalf("test %d: expected validation error", i)
		}
	}
}

func TestHoroscopeWidgetNormalizesAndDeduplicates(t *testing.T) {
	widget := &horoscopeWidget{
		Signs:   []string{" Cancer ", "cancer", "LIBRA"},
		Periods: []string{"daily", "DAILY", "weekly"},
	}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}
	if fmt.Sprint(widget.Signs) != "[cancer libra]" {
		t.Fatalf("unexpected signs: %#v", widget.Signs)
	}
	if fmt.Sprint(widget.Periods) != "[daily weekly]" {
		t.Fatalf("unexpected periods: %#v", widget.Periods)
	}
}

func TestFreeHoroscopeAPIProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/daily" || r.URL.Query().Get("sign") != "libra" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"date":"2026-09-30","period":"daily","sign":"Libra","horoscope":"A test reading."}}`))
	}))
	defer server.Close()

	previous := freeHoroscopeAPIBaseURL
	freeHoroscopeAPIBaseURL = server.URL
	defer func() { freeHoroscopeAPIBaseURL = previous }()

	reading, err := (freeHoroscopeAPIProvider{}).fetch(context.Background(), "daily", "libra")
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	if reading.Sign != "Libra" || reading.Horoscope != "A test reading." {
		t.Fatalf("unexpected reading: %#v", reading)
	}
}

func TestHoroscopeWidgetRegistered(t *testing.T) {
	descriptor, ok := widgetRegistry["horoscope"]
	if !ok {
		t.Fatal("horoscope widget is not registered")
	}
	if _, ok := descriptor.constructor().(*horoscopeWidget); !ok {
		t.Fatalf("constructor returned %T", descriptor.constructor())
	}
}
