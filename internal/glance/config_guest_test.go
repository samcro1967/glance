package glance

import (
	"strings"
	"testing"
)

func TestGuestConfigurationGrants(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		grants  map[string][]string
		want    string
	}{
		{name: "disabled by default"},
		{name: "disabled grants rejected", grants: map[string][]string{"Default": nil}, want: "requires enabled"},
		{name: "enabled requires grant", enabled: true, want: "at least one"},
		{name: "all pages", enabled: true, grants: map[string][]string{"Default": nil}},
		{name: "page subset", enabled: true, grants: map[string][]string{"Default": {"home"}}},
		{name: "unknown dashboard", enabled: true, grants: map[string][]string{"Missing": nil}, want: "unknown dashboard"},
		{name: "unknown page", enabled: true, grants: map[string][]string{"Default": {"private"}}, want: "outside that dashboard"},
		{name: "duplicate page", enabled: true, grants: map[string][]string{"Default": {"home", "home"}}, want: "duplicate page"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config{}
			cfg.Dashboards.keys = []string{"Default"}
			cfg.Dashboards.data = map[string][]string{"Default": {"home", "status"}}
			cfg.Auth.Guest = authGuestConfig{Enabled: tc.enabled, Dashboards: tc.grants}
			err := validateGuestConfig(cfg)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}
