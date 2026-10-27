package glance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestHoroscopeTimeoutYAML(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			config := "signs: [cancer]\n"
			want := time.Duration(0)
			if configured {
				config += "timeout: 15s\n"
				want = 15 * time.Second
			}
			var widget horoscopeWidget
			if err := yaml.Unmarshal([]byte(config), &widget); err != nil {
				t.Fatal(err)
			}
			if err := widget.initialize(); err != nil {
				t.Fatal(err)
			}
			provider := widget.provider.(freeHoroscopeAPIProvider)
			if time.Duration(provider.timeout) != want {
				t.Fatalf("provider timeout = %v, want %v", provider.timeout, want)
			}
		})
	}
	var widget horoscopeWidget
	if err := yaml.Unmarshal([]byte("signs: [cancer]\ntimeout: invalid\n"), &widget); err == nil {
		t.Fatal("invalid timeout must be rejected")
	}
}

func TestHoroscopeTimeoutRequests(t *testing.T) {
	// Exercise real HTTP deadlines locally without external provider dependencies.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timer := time.NewTimer(80 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":{"sign":"Cancer","period":%q,"horoscope":"A test reading."}}`, r.URL.Path[1:])
	}))
	defer server.Close()
	previous := freeHoroscopeAPIBaseURL
	freeHoroscopeAPIBaseURL = server.URL
	defer func() { freeHoroscopeAPIBaseURL = previous }()

	for _, period := range []string{"daily", "weekly", "monthly"} {
		for _, name := range []string{"default", "custom-success", "timeout", "cancelled"} {
			t.Run(period+"/"+name, func(t *testing.T) {
				widget := &horoscopeWidget{Signs: []string{"cancer"}, Periods: []string{period}}
				if name != "default" {
					widget.Timeout = durationField(time.Second)
				}
				if name == "timeout" {
					widget.Timeout = durationField(15 * time.Millisecond)
				}
				if err := widget.initialize(); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if name == "cancelled" {
					cancel()
				}
				reading, err := widget.provider.fetch(ctx, period, "cancer")
				switch name {
				case "timeout":
					if !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
						t.Fatalf("expected request timeout, got %v; parent=%v", err, ctx.Err())
					}
					if classifyRefreshFailure(err) != refreshFailureTransient {
						t.Fatalf("timeout must remain transient: %v", err)
					}
				case "cancelled":
					if !errors.Is(err, context.Canceled) || classifyRefreshFailure(err) != refreshFailureCancelled {
						t.Fatalf("expected preserved cancellation, got %v", err)
					}
				default:
					if err != nil || reading.Period != period || reading.Horoscope != "A test reading." {
						t.Fatalf("unexpected response: %#v, error=%v", reading, err)
					}
				}
			})
		}
	}
}
