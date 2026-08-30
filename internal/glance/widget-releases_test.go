package glance

import (
	"context"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var request releaseRequest

			if err := yaml.Unmarshal([]byte(test.value), &request); err != nil {
				t.Fatalf("unmarshalling release request: %v", err)
			}

			if request.source != test.wantSource {
				t.Fatalf(
					"source = %q, want %q",
					request.source,
					test.wantSource,
				)
			}

			if request.Repository != test.wantRepo {
				t.Fatalf(
					"repository = %q, want %q",
					request.Repository,
					test.wantRepo,
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
