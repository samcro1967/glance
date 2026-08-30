package glance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestReleasesWidgetInitializeDefaults(t *testing.T) {
	widget := &releasesWidget{}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Title != "Releases" {
		t.Fatalf("title = %q, want %q", widget.Title, "Releases")
	}

	if widget.cacheDuration != 2*time.Hour {
		t.Fatalf(
			"cache duration = %s, want %s",
			widget.cacheDuration,
			2*time.Hour,
		)
	}

	if widget.Limit != 10 {
		t.Fatalf("limit = %d, want 10", widget.Limit)
	}

	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse after = %d, want 5", widget.CollapseAfter)
	}
}

func TestReleasesWidgetInitializePreservesConfiguredValues(t *testing.T) {
	widget := &releasesWidget{
		Limit:         25,
		CollapseAfter: -1,
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 25 {
		t.Fatalf("limit = %d, want 25", widget.Limit)
	}

	if widget.CollapseAfter != -1 {
		t.Fatalf("collapse after = %d, want -1", widget.CollapseAfter)
	}
}

func TestReleasesWidgetInitializeNormalizesInvalidValues(t *testing.T) {
	widget := &releasesWidget{
		Limit:         -5,
		CollapseAfter: -2,
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Limit != 10 {
		t.Fatalf("limit = %d, want 10", widget.Limit)
	}

	if widget.CollapseAfter != 5 {
		t.Fatalf("collapse after = %d, want 5", widget.CollapseAfter)
	}
}

func TestReleasesWidgetInitializeAssignsProviderTokens(t *testing.T) {
	widget := &releasesWidget{
		Token:       "github-token",
		GitLabToken: "gitlab-token",
		Repositories: []*releaseRequest{
			{
				Repository: "example/github",
				source:     releaseSourceGithub,
			},
			{
				Repository: "example/gitlab",
				source:     releaseSourceGitlab,
			},
			{
				Repository: "example/codeberg",
				source:     releaseSourceCodeberg,
			},
			{
				Repository: "example/docker",
				source:     releaseSourceDockerHub,
			},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	if widget.Repositories[0].token == nil {
		t.Fatal("GitHub repository token was not assigned")
	}
	if *widget.Repositories[0].token != "github-token" {
		t.Fatalf(
			"GitHub repository token = %q, want %q",
			*widget.Repositories[0].token,
			"github-token",
		)
	}

	if widget.Repositories[1].token == nil {
		t.Fatal("GitLab repository token was not assigned")
	}
	if *widget.Repositories[1].token != "gitlab-token" {
		t.Fatalf(
			"GitLab repository token = %q, want %q",
			*widget.Repositories[1].token,
			"gitlab-token",
		)
	}

	if widget.Repositories[2].token != nil {
		t.Fatal("Codeberg repository unexpectedly received a token")
	}

	if widget.Repositories[3].token != nil {
		t.Fatal("Docker Hub repository unexpectedly received a token")
	}
}

func TestReleasesWidgetInitializeLeavesTokensUnsetWhenNotConfigured(t *testing.T) {
	widget := &releasesWidget{
		Repositories: []*releaseRequest{
			{
				Repository: "example/github",
				source:     releaseSourceGithub,
			},
			{
				Repository: "example/gitlab",
				source:     releaseSourceGitlab,
			},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}

	for i, request := range widget.Repositories {
		if request.token != nil {
			t.Fatalf("repository %d unexpectedly received a token", i)
		}
	}
}

func TestFetchLatestReleasesCancellationPreservesClassificationAndCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	requests := []*releaseRequest{
		{
			Repository: "example/repository",
			source:     releaseSourceGithub,
		},
	}

	releases, err := fetchLatestReleases(ctx, requests)
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error does not preserve context cancellation: %v", err)
	}

	if releases != nil {
		t.Fatalf("releases = %#v, want nil", releases)
	}

	const expected = "failed to retrieve any content: fetching releases: context canceled"
	if err.Error() != expected {
		t.Fatalf(
			"unexpected cancellation error:\n got: %q\nwant: %q",
			err.Error(),
			expected,
		)
	}
}

func TestFetchLatestReleasesEmptyRepositoriesReturnsNoContent(t *testing.T) {
	releases, err := fetchLatestReleases(context.Background(), nil)
	if err == nil {
		t.Fatal("expected no-content error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if releases != nil {
		t.Fatalf("releases = %#v, want nil", releases)
	}

	const expected = "failed to retrieve any content: failed 0 of 0 releases"
	if err.Error() != expected {
		t.Fatalf(
			"unexpected empty-repository error:\n got: %q\nwant: %q",
			err.Error(),
			expected,
		)
	}
}

func TestReleaseRequestUnmarshalPreservesSupportedSources(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantSource releaseSource
		wantRepo   string
	}{
		{
			name:       "default GitHub",
			value:      "example/project",
			wantSource: releaseSourceGithub,
			wantRepo:   "example/project",
		},
		{
			name:       "explicit GitHub",
			value:      "github:example/project",
			wantSource: releaseSourceGithub,
			wantRepo:   "example/project",
		},
		{
			name:       "GitLab",
			value:      "gitlab:example/project",
			wantSource: releaseSourceGitlab,
			wantRepo:   "example/project",
		},
		{
			name:       "Docker Hub",
			value:      "dockerhub:example/project",
			wantSource: releaseSourceDockerHub,
			wantRepo:   "example/project",
		},
		{
			name:       "Codeberg",
			value:      "codeberg:example/project",
			wantSource: releaseSourceCodeberg,
			wantRepo:   "example/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request releaseRequest

			if err := yaml.Unmarshal([]byte(tt.value), &request); err != nil {
				t.Fatalf("unmarshalling release request: %v", err)
			}

			if request.source != tt.wantSource {
				t.Fatalf(
					"source = %q, want %q",
					request.source,
					tt.wantSource,
				)
			}

			if request.Repository != tt.wantRepo {
				t.Fatalf(
					"repository = %q, want %q",
					request.Repository,
					tt.wantRepo,
				)
			}
		})
	}
}

func TestReleaseRequestUnmarshalPreservesStructuredConfiguration(t *testing.T) {
	var request releaseRequest

	config := `
repository: example/project
include-prereleases: true
`

	if err := yaml.Unmarshal([]byte(config), &request); err != nil {
		t.Fatalf("unmarshalling structured release request: %v", err)
	}

	if request.source != releaseSourceGithub {
		t.Fatalf(
			"source = %q, want %q",
			request.source,
			releaseSourceGithub,
		)
	}

	if request.Repository != "example/project" {
		t.Fatalf(
			"repository = %q, want %q",
			request.Repository,
			"example/project",
		)
	}

	if !request.IncludePreleases {
		t.Fatal("include-prereleases = false, want true")
	}
}

func TestReleaseRequestUnmarshalRejectsMissingRepository(t *testing.T) {
	var request releaseRequest

	err := yaml.Unmarshal([]byte("{}"), &request)
	if err == nil {
		t.Fatal("expected missing repository error")
	}

	if !strings.Contains(err.Error(), "repository is required") {
		t.Fatalf(
			"error = %q, want repository-required diagnostic",
			err,
		)
	}
}

func TestReleaseRequestUnmarshalRejectsInvalidSource(t *testing.T) {
	var request releaseRequest

	err := yaml.Unmarshal([]byte("unsupported:example/project"), &request)
	if err == nil {
		t.Fatal("expected invalid source error")
	}

	if !strings.Contains(err.Error(), "invalid source") {
		t.Fatalf("error = %q, want invalid-source diagnostic", err)
	}
}

func TestAppReleaseListSortByNewest(t *testing.T) {
	oldest := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	middle := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	releases := appReleaseList{
		{
			Name:         "middle",
			TimeReleased: middle,
		},
		{
			Name:         "oldest",
			TimeReleased: oldest,
		},
		{
			Name:         "newest",
			TimeReleased: newest,
		},
	}

	got := releases.sortByNewest()

	wantNames := []string{"newest", "middle", "oldest"}

	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf(
				"release %d name = %q, want %q",
				i,
				got[i].Name,
				want,
			)
		}
	}
}

func TestFetchLatestReleaseTaskRejectsUnsupportedSource(t *testing.T) {
	release, err := fetchLatestReleaseTask(
		context.Background(),
		&releaseRequest{
			Repository: "example/project",
			source:     releaseSource("unsupported"),
		},
	)

	if err == nil {
		t.Fatal("expected unsupported source error")
	}

	if err.Error() != "unsupported source" {
		t.Fatalf("error = %q, want %q", err, "unsupported source")
	}

	if release != nil {
		t.Fatalf("release = %#v, want nil", release)
	}
}

func TestFetchLatestDockerHubReleaseRejectsInvalidRepository(t *testing.T) {
	release, err := fetchLatestDockerHubRelease(
		context.Background(),
		&releaseRequest{
			Repository: "one/two/three",
			source:     releaseSourceDockerHub,
		},
	)

	if err == nil {
		t.Fatal("expected invalid repository error")
	}

	const want = "invalid repository name: one/two/three"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}

	if release != nil {
		t.Fatalf("release = %#v, want nil", release)
	}
}
