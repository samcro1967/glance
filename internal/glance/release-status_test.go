package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	testCurrentReleaseVersion = "v1.2.3-samcro1967.r004"
	testNewerReleaseVersion   = "v1.2.3-samcro1967.r005"
	testCurrentReleaseURL     = "https://example.invalid/releases/r004"
	testNewerReleaseURL       = "https://example.invalid/releases/r005"
	testPublishedAt           = "2026-01-01T12:00:00Z"
)

type releaseStatusRoundTripper func(*http.Request) (*http.Response, error)

func (f releaseStatusRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func useReleaseStatusTransport(t *testing.T, fn releaseStatusRoundTripper) {
	t.Helper()

	originalTransport := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = fn

	t.Cleanup(func() {
		defaultHTTPClient.Transport = originalTransport
	})
}

func releaseStatusJSONResponse(r *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: r,
	}
}

func TestParseForkReleaseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    forkReleaseVersion
		valid   bool
	}{
		{
			name:    "formal release",
			version: "v1.2.3-samcro1967.r004",
			want: forkReleaseVersion{
				Major:    1,
				Minor:    2,
				Patch:    3,
				Revision: 4,
			},
			valid: true,
		},
		{
			name:    "larger revision",
			version: "v1.2.3-samcro1967.r010",
			want: forkReleaseVersion{
				Major:    1,
				Minor:    2,
				Patch:    3,
				Revision: 10,
			},
			valid: true,
		},
		{
			name:    "development",
			version: "dev",
			valid:   false,
		},
		{
			name:    "upstream release",
			version: "v1.2.3",
			valid:   false,
		},
		{
			name:    "different fork",
			version: "v1.2.3-example.r004",
			valid:   false,
		},
		{
			name:    "missing revision",
			version: "v1.2.3-samcro1967",
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := parseForkReleaseVersion(tt.version)

			if valid != tt.valid {
				t.Fatalf("valid = %v, want %v", valid, tt.valid)
			}

			if valid && got != tt.want {
				t.Fatalf("version = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCompareForkReleaseVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{
			name: "same",
			a:    "v1.2.3-samcro1967.r004",
			b:    "v1.2.3-samcro1967.r004",
			want: 0,
		},
		{
			name: "newer revision",
			a:    "v1.2.3-samcro1967.r010",
			b:    "v1.2.3-samcro1967.r009",
			want: 1,
		},
		{
			name: "older revision",
			a:    "v1.2.3-samcro1967.r009",
			b:    "v1.2.3-samcro1967.r010",
			want: -1,
		},
		{
			name: "newer fork release series",
			a:    "v1.2.4-samcro1967.r001",
			b:    "v1.2.3-samcro1967.r999",
			want: 1,
		},
		{
			name: "older fork release series",
			a:    "v1.2.3-samcro1967.r999",
			b:    "v1.2.4-samcro1967.r001",
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, ok := parseForkReleaseVersion(tt.a)
			if !ok {
				t.Fatalf("could not parse %q", tt.a)
			}

			b, ok := parseForkReleaseVersion(tt.b)
			if !ok {
				t.Fatalf("could not parse %q", tt.b)
			}

			if got := compareForkReleaseVersions(a, b); got != tt.want {
				t.Fatalf("compare = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCheckForkReleaseStatusLatest(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/repos/samcro1967/glance/releases/latest" {
			t.Fatalf("path = %q", r.URL.EscapedPath())
		}

		return releaseStatusJSONResponse(
			r,
			`{"tag_name":"`+testCurrentReleaseVersion+
				`","published_at":"`+testPublishedAt+
				`","html_url":"`+testCurrentReleaseURL+`"}`,
		), nil
	})

	result, err := checkForkReleaseStatus(
		context.Background(),
		testCurrentReleaseVersion,
	)
	if err != nil {
		t.Fatalf("check release status: %v", err)
	}

	if result.Status != releaseStatusLatest {
		t.Fatalf("status = %v, want latest", result.Status)
	}

	if result.LatestVersion != testCurrentReleaseVersion {
		t.Fatalf("latest version = %q", result.LatestVersion)
	}

	if result.ReleaseURL != testCurrentReleaseURL {
		t.Fatalf("release URL = %q", result.ReleaseURL)
	}
}

func TestCheckForkReleaseStatusUpdateAvailable(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		return releaseStatusJSONResponse(
			r,
			`{"tag_name":"`+testNewerReleaseVersion+
				`","published_at":"`+testPublishedAt+
				`","html_url":"`+testNewerReleaseURL+`"}`,
		), nil
	})

	result, err := checkForkReleaseStatus(
		context.Background(),
		testCurrentReleaseVersion,
	)
	if err != nil {
		t.Fatalf("check release status: %v", err)
	}

	if result.Status != releaseStatusUpdateAvailable {
		t.Fatalf("status = %v, want update available", result.Status)
	}

	if result.LatestVersion != testNewerReleaseVersion {
		t.Fatalf("latest version = %q", result.LatestVersion)
	}

	if result.ReleaseURL != testNewerReleaseURL {
		t.Fatalf("release URL = %q", result.ReleaseURL)
	}
}

func TestCheckForkReleaseStatusRunningVersionNewer(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		return releaseStatusJSONResponse(
			r,
			`{"tag_name":"`+testCurrentReleaseVersion+
				`","published_at":"`+testPublishedAt+
				`","html_url":"`+testCurrentReleaseURL+`"}`,
		), nil
	})

	result, err := checkForkReleaseStatus(
		context.Background(),
		testNewerReleaseVersion,
	)
	if err != nil {
		t.Fatalf("check release status: %v", err)
	}

	if result.Status != releaseStatusUnknown {
		t.Fatalf("status = %v, want unknown", result.Status)
	}
}

func TestCheckForkReleaseStatusDevSkipsRequest(t *testing.T) {
	requested := false

	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		requested = true
		return nil, errors.New("unexpected request")
	})

	result, err := checkForkReleaseStatus(context.Background(), "dev")
	if err != nil {
		t.Fatalf("check release status: %v", err)
	}

	if requested {
		t.Fatal("development version performed release request")
	}

	if result.Status != releaseStatusUnknown {
		t.Fatalf("status = %v, want unknown", result.Status)
	}
}

func TestCheckForkReleaseStatusUnrecognizedVersionSkipsRequest(t *testing.T) {
	requested := false

	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		requested = true
		return nil, errors.New("unexpected request")
	})

	result, err := checkForkReleaseStatus(context.Background(), "v1.2.3")
	if err != nil {
		t.Fatalf("check release status: %v", err)
	}

	if requested {
		t.Fatal("unrecognized version performed release request")
	}

	if result.Status != releaseStatusUnknown {
		t.Fatalf("status = %v, want unknown", result.Status)
	}
}

func TestCheckForkReleaseStatusRequestFailure(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})

	result, err := checkForkReleaseStatus(
		context.Background(),
		testCurrentReleaseVersion,
	)

	if err == nil {
		t.Fatal("expected release request error")
	}

	if result.Status != releaseStatusUnknown {
		t.Fatalf("status = %v, want unknown", result.Status)
	}
}

func TestCheckForkReleaseStatusUnrecognizedLatestRelease(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		return releaseStatusJSONResponse(
			r,
			`{"tag_name":"unexpected","published_at":"`+testPublishedAt+
				`","html_url":"https://example.invalid/releases/unexpected"}`,
		), nil
	})

	result, err := checkForkReleaseStatus(
		context.Background(),
		testCurrentReleaseVersion,
	)
	if err != nil {
		t.Fatalf("check release status: %v", err)
	}

	if result.Status != releaseStatusUnknown {
		t.Fatalf("status = %v, want unknown", result.Status)
	}
}

func TestReleaseStatusCache(t *testing.T) {
	var cache releaseStatusCache

	if got := cache.get(); got.Status != releaseStatusUnknown {
		t.Fatalf("initial status = %v, want unknown", got.Status)
	}

	want := releaseStatusResult{
		Status:        releaseStatusUpdateAvailable,
		LatestVersion: testNewerReleaseVersion,
		ReleaseURL:    testNewerReleaseURL,
	}

	cache.set(want)

	if got := cache.get(); got != want {
		t.Fatalf("cached result = %#v, want %#v", got, want)
	}
}

func TestShouldCheckForkReleaseStatus(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*application)
		want  bool
	}{
		{
			name: "formal release with default footer",
			want: true,
		},
		{
			name: "development build",
			setup: func(app *application) {
				app.Version = "dev"
			},
			want: false,
		},
		{
			name: "hidden footer",
			setup: func(app *application) {
				app.Config.Branding.HideFooter = true
			},
			want: false,
		},
		{
			name: "custom footer",
			setup: func(app *application) {
				app.Config.Branding.CustomFooter = "Custom footer"
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{
				Version: testCurrentReleaseVersion,
			}

			if tt.setup != nil {
				tt.setup(app)
			}

			if got := app.shouldCheckForkReleaseStatus(); got != tt.want {
				t.Fatalf(
					"shouldCheckForkReleaseStatus() = %v, want %v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestRunForkReleaseStatusCheckerDevSkipsRequest(t *testing.T) {
	requested := false

	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		requested = true
		return nil, errors.New("unexpected request")
	})

	var cache releaseStatusCache

	runForkReleaseStatusChecker(
		context.Background(),
		"dev",
		&cache,
	)

	if requested {
		t.Fatal("development checker performed release request")
	}

	if got := cache.get(); got.Status != releaseStatusUnknown {
		t.Fatalf("status = %v, want unknown", got.Status)
	}
}

func TestRunForkReleaseStatusCheckerStopsAfterCancelledInitialCheck(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		return releaseStatusJSONResponse(
			r,
			`{"tag_name":"`+testCurrentReleaseVersion+
				`","published_at":"`+testPublishedAt+
				`","html_url":"`+testCurrentReleaseURL+`"}`,
		), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var cache releaseStatusCache

	done := make(chan struct{})
	go func() {
		runForkReleaseStatusChecker(
			ctx,
			testCurrentReleaseVersion,
			&cache,
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("release status checker did not stop after cancellation")
	}
}

func TestFetchInstalledForkReleaseDate(t *testing.T) {
	for _, tt := range []struct {
		name      string
		tag       string
		published string
		want      string
	}{
		{"published", testCurrentReleaseVersion, testPublishedAt, testPublishedAt},
		{"UTC conversion", testCurrentReleaseVersion, "2026-01-02T00:30:00+02:00", "2026-01-01T22:30:00Z"},
		{"missing", testCurrentReleaseVersion, "", ""},
		{"invalid", testCurrentReleaseVersion, "invalid-date", ""},
		{"zero", testCurrentReleaseVersion, "0001-01-01T00:00:00Z", ""},
		{"different tag", testNewerReleaseVersion, testPublishedAt, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.RequestURI() != "/repos/samcro1967/glance/releases/tags/"+testCurrentReleaseVersion {
					t.Fatalf("unexpected installed-release request: %s %s", r.Method, r.URL.RequestURI())
				}
				return releaseStatusJSONResponse(r, `{"tag_name":"`+tt.tag+`","published_at":"`+tt.published+`"}`), nil
			})
			got, err := fetchInstalledForkReleaseDate(context.Background(), testCurrentReleaseVersion)
			if tt.want == "" {
				if err == nil || !got.IsZero() {
					t.Fatalf("unverified date accepted: date=%v err=%v", got, err)
				}
				return
			}
			if err != nil || got.Format(time.RFC3339) != tt.want || got.Location() != time.UTC {
				t.Fatalf("date=%v err=%v, want %s in UTC", got, err, tt.want)
			}
		})
	}
}

func TestFetchInstalledForkReleaseDateSkipsNonreleaseBuilds(t *testing.T) {
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		t.Fatal("nonrelease build performed installed-release request")
		return nil, errors.New("unexpected request")
	})
	for _, version := range []string{"dev", "", "v1.2.3"} {
		date, err := fetchInstalledForkReleaseDate(context.Background(), version)
		if err != nil || !date.IsZero() {
			t.Fatalf("version %q: date=%v err=%v", version, date, err)
		}
	}
}

func TestFetchInstalledForkReleaseDateFailures(t *testing.T) {
	for _, failure := range []string{"network", "not found", "malformed JSON", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "cancellation" {
				cancel()
			}
			useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
				switch failure {
				case "network":
					return nil, errors.New("network unavailable")
				case "cancellation":
					return nil, r.Context().Err()
				case "not found":
					response := releaseStatusJSONResponse(r, `{}`)
					response.StatusCode = http.StatusNotFound
					response.Status = "404 Not Found"
					return response, nil
				default:
					return releaseStatusJSONResponse(r, `{`), nil
				}
			})
			date, err := fetchInstalledForkReleaseDate(ctx, testCurrentReleaseVersion)
			if err == nil || !date.IsZero() {
				t.Fatalf("failure returned a date: date=%v err=%v", date, err)
			}
			if failure == "cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation cause lost: %v", err)
			}
		})
	}
}

func TestUpdateForkReleaseStatusCachesInstalledDateIndependently(t *testing.T) {
	var cache releaseStatusCache
	installedRequests, latestRequests := 0, 0
	failLatest := false
	useReleaseStatusTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/repos/samcro1967/glance/releases/tags/" + testCurrentReleaseVersion:
			installedRequests++
			// A failed lookup must not hide update status and is retried later.
			if installedRequests == 1 {
				return nil, errors.New("installed release temporarily unavailable")
			}
			return releaseStatusJSONResponse(r, `{"tag_name":"`+testCurrentReleaseVersion+`","published_at":"`+testPublishedAt+`"}`), nil
		case "/repos/samcro1967/glance/releases/latest":
			latestRequests++
			if failLatest {
				return nil, errors.New("latest release temporarily unavailable")
			}
			return releaseStatusJSONResponse(r, `{"tag_name":"`+testNewerReleaseVersion+`","published_at":"2026-02-01T12:00:00Z"}`), nil
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
			return nil, errors.New("unexpected request")
		}
	})
	updateForkReleaseStatus(context.Background(), testCurrentReleaseVersion, &cache)
	if got := cache.get(); got.Status != releaseStatusUpdateAvailable || !got.InstalledPublishedAt.IsZero() {
		t.Fatalf("date failure affected update status: %+v", got)
	}
	updateForkReleaseStatus(context.Background(), testCurrentReleaseVersion, &cache)
	if got := cache.get(); got.InstalledReleaseTooltip() != "Released January 1, 2026 (UTC)" || got.Status != releaseStatusUpdateAvailable {
		t.Fatalf("installed date was not acquired independently: %+v", got)
	}
	failLatest = true
	updateForkReleaseStatus(context.Background(), testCurrentReleaseVersion, &cache)
	if got := cache.get(); got.Status != releaseStatusUnknown || got.InstalledReleaseTooltip() != "Released January 1, 2026 (UTC)" {
		t.Fatalf("latest failure discarded confirmed installed date: %+v", got)
	}
	if installedRequests != 2 || latestRequests != 3 {
		t.Fatalf("request counts: installed=%d latest=%d, want 2 and 3", installedRequests, latestRequests)
	}
}
