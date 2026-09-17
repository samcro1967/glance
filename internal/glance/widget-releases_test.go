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

func TestReleaseRequestCustomBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantSource releaseSource
		wantRepo   string
		wantBase   string
		wantErr    string
	}{
		{
			name:       "gitlab custom origin",
			yaml:       "repository: gitlab:group/project\nbase-url: https://gitlab.example.com/\n",
			wantSource: releaseSourceGitlab,
			wantRepo:   "group/project",
			wantBase:   "https://gitlab.example.com",
		},
		{
			name:       "codeberg http origin",
			yaml:       "repository: codeberg:owner/project\nbase-url: http://forge.internal:3000\n",
			wantSource: releaseSourceCodeberg,
			wantRepo:   "owner/project",
			wantBase:   "http://forge.internal:3000",
		},
		{
			name:    "github rejects custom origin",
			yaml:    "repository: github:owner/project\nbase-url: https://github.example.com\n",
			wantErr: "base-url is not supported for github repositories",
		},
		{
			name:    "docker hub rejects custom origin",
			yaml:    "repository: dockerhub:owner/project\nbase-url: https://docker.example.com\n",
			wantErr: "base-url is not supported for dockerhub repositories",
		},
		{
			name:    "rejects relative URL",
			yaml:    "repository: gitlab:owner/project\nbase-url: gitlab.example.com\n",
			wantErr: "base-url must be an absolute http or https URL",
		},
		{
			name:    "rejects credentials",
			yaml:    "repository: gitlab:owner/project\nbase-url: https://user:pass@gitlab.example.com\n",
			wantErr: "base-url must not contain credentials, a query, or a fragment",
		},
		{
			name:    "rejects path",
			yaml:    "repository: codeberg:owner/project\nbase-url: https://forge.example.com/subpath\n",
			wantErr: "base-url must not contain a path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request releaseRequest
			err := yaml.Unmarshal([]byte(test.yaml), &request)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}
			if request.source != test.wantSource {
				t.Fatalf("source = %q, want %q", request.source, test.wantSource)
			}
			if request.Repository != test.wantRepo {
				t.Fatalf("repository = %q, want %q", request.Repository, test.wantRepo)
			}
			if request.BaseURL != test.wantBase {
				t.Fatalf("base URL = %q, want %q", request.BaseURL, test.wantBase)
			}
		})
	}
}

func TestReleaseRequestStructuredRepositorySourceParsing(t *testing.T) {
	var request releaseRequest
	if err := yaml.Unmarshal([]byte("repository: gitlab:group/project\ninclude-prereleases: false\n"), &request); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if request.source != releaseSourceGitlab {
		t.Fatalf("source = %q, want %q", request.source, releaseSourceGitlab)
	}
	if request.Repository != "group/project" {
		t.Fatalf("repository = %q, want %q", request.Repository, "group/project")
	}
}

func TestReleaseProviderURLs(t *testing.T) {
	tests := []struct {
		name    string
		request *releaseRequest
		build   func(*releaseRequest) string
		want    string
	}{
		{
			name:    "gitlab default",
			request: &releaseRequest{Repository: "group/project"},
			build:   gitLabReleaseURL,
			want:    "https://gitlab.com/api/v4/projects/group%2Fproject/releases/permalink/latest",
		},
		{
			name:    "gitlab custom",
			request: &releaseRequest{Repository: "group/project", BaseURL: "https://gitlab.example.com"},
			build:   gitLabReleaseURL,
			want:    "https://gitlab.example.com/api/v4/projects/group%2Fproject/releases/permalink/latest",
		},
		{
			name:    "codeberg default",
			request: &releaseRequest{Repository: "owner/project"},
			build:   codebergReleaseURL,
			want:    "https://codeberg.org/api/v1/repos/owner/project/releases/latest",
		},
		{
			name:    "codeberg custom",
			request: &releaseRequest{Repository: "owner/project", BaseURL: "http://forge.internal:3000"},
			build:   codebergReleaseURL,
			want:    "http://forge.internal:3000/api/v1/repos/owner/project/releases/latest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.build(test.request); got != test.want {
				t.Fatalf("URL = %q, want %q", got, test.want)
			}
		})
	}
}
