package glance

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gofeedext "github.com/mmcdole/gofeed/extensions"
)

type priorityRoundTripper func(*http.Request) (*http.Response, error)

func (f priorityRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func priorityJSONResponse(r *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func usePriorityTransport(t *testing.T, fn priorityRoundTripper) {
	t.Helper()
	old := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = fn
	t.Cleanup(func() { defaultHTTPClient.Transport = old })
}

func TestPriorityReleaseProviderResponsesAndAuthentication(t *testing.T) {
	tests := []struct {
		name        string
		req         *releaseRequest
		path        string
		auth        func(*http.Request) bool
		body        string
		wantSource  releaseSource
		wantName    string
		wantVersion string
	}{
		{"github", &releaseRequest{Repository: "example/project", source: releaseSourceGithub}, "/repos/example/project/releases/latest", func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer secret" }, `{"tag_name":"v1.2.3","published_at":"2026-08-30T12:00:00Z","html_url":"https://example.invalid/release"}`, releaseSourceGithub, "example/project", "v1.2.3"},
		{"gitlab", &releaseRequest{Repository: "example/project", source: releaseSourceGitlab}, "/api/v4/projects/example%2Fproject/releases/permalink/latest", func(r *http.Request) bool { return r.Header.Get("PRIVATE-TOKEN") == "secret" }, `{"tag_name":"v2.0.0","released_at":"2026-08-30T12:00:00Z","_links":{"self":"https://example.invalid/release"}}`, releaseSourceGitlab, "example/project", "v2.0.0"},
		{"codeberg", &releaseRequest{Repository: "example/project", source: releaseSourceCodeberg}, "/api/v1/repos/example/project/releases/latest", func(r *http.Request) bool { return r.Header.Get("Authorization") == "" }, `{"tag_name":"v3.0.0","published_at":"2026-08-30T12:00:00Z","html_url":"https://example.invalid/release"}`, releaseSourceCodeberg, "example/project", "v3.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := "secret"
			tt.req.token = &token
			usePriorityTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.EscapedPath() != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.EscapedPath(), tt.path)
				}
				if !tt.auth(r) {
					t.Fatalf("unexpected authentication headers: %#v", r.Header)
				}
				return priorityJSONResponse(r, tt.body), nil
			})
			got, err := fetchLatestReleaseTask(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("fetch release: %v", err)
			}
			if got.Source != tt.wantSource || got.Name != tt.wantName || got.Version != tt.wantVersion {
				t.Fatalf("release = %#v", got)
			}
			if got.TimeReleased.IsZero() {
				t.Fatal("release time was not parsed")
			}
		})
	}
}

func TestPriorityGithubPrereleaseEmptyResponseIsRejected(t *testing.T) {
	usePriorityTransport(t, func(r *http.Request) (*http.Response, error) { return priorityJSONResponse(r, `[]`), nil })
	got, err := fetchLatestGithubRelease(context.Background(), &releaseRequest{Repository: "example/project", IncludePreleases: true})
	if err == nil || !strings.Contains(err.Error(), "no releases found") {
		t.Fatalf("error = %v", err)
	}
	if got != nil {
		t.Fatalf("release = %#v, want nil", got)
	}
}

func TestPriorityDockerHubOfficialAndSpecificTagResponses(t *testing.T) {
	tests := []struct{ repo, path, body, name, version string }{
		{"alpine", "/v2/namespaces/library/repositories/alpine/tags", `{"results":[{"name":"3.22","tag_last_pushed":"2026-08-30T12:00:00Z"}]}`, "alpine", "3.22"},
		{"example/project:stable", "/v2/namespaces/example/repositories/project/tags/stable", `{"name":"stable","tag_last_pushed":"2026-08-30T12:00:00Z"}`, "example/project", "stable"},
	}
	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			usePriorityTransport(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %q, want %q", r.URL.Path, tt.path)
				}
				return priorityJSONResponse(r, tt.body), nil
			})
			got, err := fetchLatestDockerHubRelease(context.Background(), &releaseRequest{Repository: tt.repo})
			if err != nil {
				t.Fatalf("fetch release: %v", err)
			}
			if got.Name != tt.name || got.Version != tt.version {
				t.Fatalf("release = %#v", got)
			}
		})
	}
}

func TestPriorityDockerHubEmptyTagsAreRejected(t *testing.T) {
	usePriorityTransport(t, func(r *http.Request) (*http.Response, error) { return priorityJSONResponse(r, `{"results":[]}`), nil })
	got, err := fetchLatestDockerHubRelease(context.Background(), &releaseRequest{Repository: "alpine"})
	if err == nil || !strings.Contains(err.Error(), "no tags found") {
		t.Fatalf("error = %v", err)
	}
	if got != nil {
		t.Fatalf("release = %#v, want nil", got)
	}
}

func TestPriorityRecursiveRSSMediaThumbnailLookup(t *testing.T) {
	extensions := map[string][]gofeedext.Extension{
		"group": {{Name: "group", Children: map[string][]gofeedext.Extension{
			"content": {{Name: "content", Children: map[string][]gofeedext.Extension{
				"thumbnail": {{Name: "thumbnail", Attrs: map[string]string{"url": "https://example.invalid/thumb.jpg"}}},
			}}},
		}}},
	}
	if got := recursiveFindThumbnailInExtensions(extensions); got != "https://example.invalid/thumb.jpg" {
		t.Fatalf("thumbnail = %q", got)
	}
	if got := recursiveFindThumbnailInExtensions(map[string][]gofeedext.Extension{"x": {{Name: "thumbnail"}}}); got != "" {
		t.Fatalf("thumbnail without URL = %q", got)
	}
}

func TestPriorityCustomAPITimeAliasesAndInvalidInputs(t *testing.T) {
	value := time.Date(2026, 8, 30, 14, 15, 16, 123456789, time.UTC)
	if got := customAPIFuncFormatTime("unix", value); got != "1788099316" {
		t.Fatalf("unix = %q", got)
	}
	if got := customAPIFuncFormatTime("RFC3339", value); got != "2026-08-30T14:15:16Z" {
		t.Fatalf("rfc3339 = %q", got)
	}
	if got := customAPIFuncFormatTime("DateOnly", value); got != "2026-08-30" {
		t.Fatalf("dateonly = %q", got)
	}
	loc := time.FixedZone("test", -5*60*60)
	if got := customAPIFuncParseTimeInLocation("DateTime", "2026-08-30 14:15:16", loc); got.Location() != loc || got.Hour() != 14 {
		t.Fatalf("parsed = %v", got)
	}
	if got := customAPIFuncParseTimeInLocation("unix", "invalid", time.UTC); !got.Equal(time.Unix(0, 0)) {
		t.Fatalf("invalid unix = %v", got)
	}
	if got := customAPIFuncParseTimeInLocation("DateOnly", "not-a-date", time.UTC); !got.Equal(time.Unix(0, 0)) {
		t.Fatalf("invalid date = %v", got)
	}
}
