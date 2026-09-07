package glance

import (
	"testing"
	"time"
)

func TestICSSourceDefaultsPreserveConfiguredValues(t *testing.T) {
	sources := []icsEventSource{
		{
			Headers: map[string]string{
				"X-Child":    "child",
				"X-Override": "child",
			},
		},
		{
			Timeout:       durationField(21 * time.Second),
			AllowInsecure: false,
			Headers: map[string]string{
				"X-Explicit": "explicit",
			},
			configuredFields: map[string]bool{
				"timeout":        true,
				"allow-insecure": true,
				"basic-auth":     true,
			},
		},
	}

	sources[1].BasicAuth.Username = "child-user"
	sources[1].BasicAuth.Password = "child-pass"

	applyICSSourceTimeoutDefault(sources, durationField(13*time.Second))
	applyICSSourceAllowInsecureDefault(sources, true)
	applyICSSourceHeadersDefault(sources, map[string]string{
		"X-Default":  "default",
		"X-Override": "default",
	})
	applyICSSourceBasicAuthDefault(sources, basicAuthDefaults{
		Username: "default-user",
		Password: "default-pass",
	})

	if got := time.Duration(sources[0].Timeout); got != 13*time.Second {
		t.Fatalf("inherited timeout = %v, want 13s", got)
	}
	if !sources[0].AllowInsecure {
		t.Fatal("inherited allow-insecure = false, want true")
	}
	if got := sources[0].Headers["X-Default"]; got != "default" {
		t.Fatalf("inherited header = %q, want default", got)
	}
	if got := sources[0].Headers["X-Override"]; got != "child" {
		t.Fatalf("child header override = %q, want child", got)
	}
	if sources[0].BasicAuth.Username != "default-user" ||
		sources[0].BasicAuth.Password != "default-pass" {
		t.Fatalf("inherited basic auth = %#v", sources[0].BasicAuth)
	}

	if got := time.Duration(sources[1].Timeout); got != 21*time.Second {
		t.Fatalf("configured timeout = %v, want 21s", got)
	}
	if sources[1].AllowInsecure {
		t.Fatal("configured allow-insecure false was overwritten")
	}
	if got := sources[1].Headers["X-Default"]; got != "default" {
		t.Fatalf("default header on configured source = %q, want default", got)
	}
	if got := sources[1].Headers["X-Explicit"]; got != "explicit" {
		t.Fatalf("configured header = %q, want explicit", got)
	}
	if sources[1].BasicAuth.Username != "child-user" ||
		sources[1].BasicAuth.Password != "child-pass" {
		t.Fatalf("configured basic auth = %#v", sources[1].BasicAuth)
	}
}

func TestSortICSEventsOrdersByStartThenTitleStably(t *testing.T) {
	start := time.Date(2026, 9, 7, 18, 0, 0, 0, time.UTC)

	events := []icsEvent{
		{UID: "later", Title: "Later", Start: start.Add(time.Hour)},
		{UID: "zulu", Title: "Zulu", Start: start},
		{UID: "alpha-first", Title: "Alpha", Start: start},
		{UID: "alpha-second", Title: "Alpha", Start: start},
		{UID: "earlier", Title: "Earlier", Start: start.Add(-time.Hour)},
	}

	sortICSEvents(events)

	wantUIDs := []string{
		"earlier",
		"alpha-first",
		"alpha-second",
		"zulu",
		"later",
	}

	for i, want := range wantUIDs {
		if events[i].UID != want {
			t.Fatalf("event %d UID = %q, want %q", i, events[i].UID, want)
		}
	}
}
