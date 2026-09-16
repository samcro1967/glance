package glance

import (
	"strings"
	"testing"
)

func TestNormalizeAnalyticsEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "https", input: "https://analytics.example.com", want: "https://analytics.example.com"},
		{name: "http", input: "http://analytics.example.com", want: "http://analytics.example.com"},
		{name: "port", input: "https://analytics.example.com:8443", want: "https://analytics.example.com:8443"},
		{name: "trailing slash", input: "https://analytics.example.com/", want: "https://analytics.example.com"},
		{name: "relative", input: "analytics.example.com", wantErr: "must use http or https"},
		{name: "scheme", input: "ftp://analytics.example.com", wantErr: "must use http or https"},
		{name: "userinfo", input: "https://user:pass@analytics.example.com", wantErr: "user information"},
		{name: "path", input: "https://analytics.example.com/stats", wantErr: "path"},
		{name: "query", input: "https://analytics.example.com/?token=secret", wantErr: "query"},
		{name: "fragment", input: "https://analytics.example.com/#stats", wantErr: "fragment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAnalyticsEndpoint(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("normalizeAnalyticsEndpoint(%q) error = %v, want containing %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeAnalyticsEndpoint(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeAnalyticsEndpoint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnalyticsConfigURLs(t *testing.T) {
	config := analyticsConfig{Provider: analyticsProviderGoatCounter, Endpoint: "https://analytics.example.com"}
	if got := config.CountURL(); got != "https://analytics.example.com/count" {
		t.Fatalf("CountURL() = %q", got)
	}
	if got := config.ScriptURL(); got != "https://analytics.example.com/count.js" {
		t.Fatalf("ScriptURL() = %q", got)
	}
}

func TestAnalyticsConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		analyticsYAML string
		wantProvider  string
		wantEndpoint  string
		wantErr       string
	}{
		{name: "disabled"},
		{name: "goatcounter", analyticsYAML: "analytics:\n  provider: GOATCOUNTER\n  endpoint: https://analytics.example.com/\n", wantProvider: "goatcounter", wantEndpoint: "https://analytics.example.com"},
		{name: "missing provider", analyticsYAML: "analytics:\n  endpoint: https://analytics.example.com\n", wantErr: "analytics provider must be set"},
		{name: "missing endpoint", analyticsYAML: "analytics:\n  provider: goatcounter\n", wantErr: "analytics endpoint must be set"},
		{name: "unsupported provider", analyticsYAML: "analytics:\n  provider: other\n  endpoint: https://analytics.example.com\n", wantErr: "unsupported analytics provider"},
		{name: "invalid endpoint", analyticsYAML: "analytics:\n  provider: goatcounter\n  endpoint: https://analytics.example.com/private\n", wantErr: "analytics endpoint must not contain a path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := tt.analyticsYAML + "pages:\n  - name: Home\n    columns:\n      - size: full\n"
			config, err := newConfigFromYAML([]byte(yaml))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("newConfigFromYAML() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newConfigFromYAML() error = %v", err)
			}
			if config.Analytics.Provider != tt.wantProvider {
				t.Fatalf("provider = %q, want %q", config.Analytics.Provider, tt.wantProvider)
			}
			if config.Analytics.Endpoint != tt.wantEndpoint {
				t.Fatalf("endpoint = %q, want %q", config.Analytics.Endpoint, tt.wantEndpoint)
			}
		})
	}
}
