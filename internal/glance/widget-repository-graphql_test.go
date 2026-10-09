package glance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRepositoryGraphQLSuccess(t *testing.T) {
	calls := 0
	useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.URL.String() != "https://api.github.com/graphql" {
			t.Fatalf("unexpected GraphQL request: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer secret" || req.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("incorrect GraphQL headers")
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Query, "defaultBranchRef") || payload.Variables["owner"] != "example" || payload.Variables["name"] != "project" {
			t.Fatalf("incorrect query or variables: %v", payload.Variables)
		}
		for key, want := range map[string]any{"prs": true, "issues": true, "commits": true, "prLimit": float64(2), "issueLimit": float64(3), "commitLimit": float64(4)} {
			if payload.Variables[key] != want {
				t.Errorf("variable %s = %v, want %v", key, payload.Variables[key], want)
			}
		}
		return repositoryTestResponse(req, 200, "200 OK", `{"data":{"repository":{"nameWithOwner":"example/project","stargazerCount":12,"forkCount":3,"pullRequests":{"totalCount":7,"nodes":[{"number":101,"title":"PR","createdAt":"2026-08-29T12:00:00Z"}]},"issues":{"totalCount":9,"nodes":[{"number":201,"title":"Issue","createdAt":"2026-08-28T12:00:00Z"}]},"defaultBranchRef":{"target":{"history":{"nodes":[{"oid":"abc","message":"Commit headline\ncontinued subject\n\nCommit body","committedDate":"2026-08-30T12:00:00Z","author":{"name":"Author","date":"2026-08-29T12:00:00Z"}}]}}}}}}`), nil
	}))
	got, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "secret", 2, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("requests = %d, want 1", calls)
	}
	if got.Name != "example/project" || got.Stars != 12 || got.Forks != 3 || got.OpenPullRequests != 7 || got.OpenIssues != 9 {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if len(got.PullRequests) != 1 || got.PullRequests[0].Number != 101 || len(got.Issues) != 1 || got.Issues[0].Number != 201 {
		t.Fatalf("unexpected tickets: %+v", got)
	}
	if len(got.Commits) != 1 || got.Commits[0].Sha != "abc" || got.Commits[0].Author != "Author" || got.Commits[0].Message != "Commit headline\ncontinued subject" || got.Commits[0].CreatedAt.Format("2006-01-02") != "2026-08-29" {
		t.Fatalf("unexpected commits: %+v", got.Commits)
	}
}

func TestRepositoryGraphQLDisabledSectionsAndLimits(t *testing.T) {
	useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"prs", "issues", "commits"} {
			if payload.Variables[key] != false {
				t.Errorf("%s should be disabled", key)
			}
		}
		for _, key := range []string{"prLimit", "issueLimit", "commitLimit"} {
			if payload.Variables[key] != float64(1) {
				t.Errorf("%s should be 1", key)
			}
		}
		return repositoryTestResponse(req, 200, "200 OK", `{"data":{"repository":{"nameWithOwner":"example/project","stargazerCount":1,"forkCount":2}}}`), nil
	}))
	got, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "secret", -1, -1, -1)
	if err != nil || got.Name != "example/project" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if githubGraphQLLimit(101) != 100 || githubGraphQLLimit(0) != 1 {
		t.Fatal("incorrect connection bounds")
	}
}

func TestRepositoryGraphQLPartialErrors(t *testing.T) {
	useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return repositoryTestResponse(req, 200, "200 OK", `{"data":{"repository":{"nameWithOwner":"example/project","stargazerCount":2,"forkCount":1,"pullRequests":null,"issues":{"totalCount":4,"nodes":[{"number":22,"title":"Good issue","createdAt":"2026-08-28T12:00:00Z"}]}}},"errors":[{"message":"SECRET_PROVIDER_MESSAGE","path":["repository","pullRequests"]}]}`), nil
	}))
	got, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "secret", 2, 2, -1)
	if !errors.Is(err, errPartialContent) || !strings.Contains(err.Error(), "failed 1 of 2 repository sections") || strings.Contains(err.Error(), "SECRET_PROVIDER_MESSAGE") {
		t.Fatalf("incorrect partial error: %v", err)
	}
	if got.Name != "example/project" || got.OpenIssues != 4 || len(got.Issues) != 1 {
		t.Fatalf("lost successful content: %+v", got)
	}
}

func TestRepositoryGraphQLFailureCases(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"repository null", `{"data":{"repository":null},"errors":[{"message":"private secret"}]}`, 200},
		{"repository error path", `{"data":{"repository":{"nameWithOwner":"example/project"}},"errors":[{"path":["repository","nameWithOwner"]}]}`, 200},
		{"invalid JSON", `{invalid`, 200},
		{"HTTP failure", `{"message":"private secret"}`, 403},
		{"missing name", `{"data":{"repository":{"stargazerCount":1}}}`, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return repositoryTestResponse(req, tc.status, http.StatusText(tc.status), tc.body), nil
			}))
			_, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "secret", -1, -1, -1)
			if !errors.Is(err, errNoContent) || strings.Contains(err.Error(), "private secret") {
				t.Fatalf("incorrect error: %v", err)
			}
		})
	}
}

func TestRepositoryGraphQLInvalidIdentifier(t *testing.T) {
	useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) { t.Fatal("unexpected HTTP request"); return nil, nil }))
	for _, name := range []string{"", "example", "example/", "/project", "example/project/extra", "example/project?token=secret"} {
		_, err := fetchRepositoryDetailsFromGithub(context.Background(), name, "secret", 1, 1, 1)
		if !errors.Is(err, errNoContent) {
			t.Errorf("%q: got %v", name, err)
		}
	}
}

func TestRepositoryGraphQLMissingDefaultBranch(t *testing.T) {
	useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return repositoryTestResponse(req, 200, "200 OK", `{"data":{"repository":{"nameWithOwner":"example/project","defaultBranchRef":null}}}`), nil
	}))
	got, err := fetchRepositoryDetailsFromGithub(context.Background(), "example/project", "secret", -1, -1, 3)
	if err != nil || got.Name != "example/project" || len(got.Commits) != 0 {
		t.Fatalf("unexpected result: %+v, %v", got, err)
	}
}

func TestRepositoryGraphQLCancellation(t *testing.T) {
	started := make(chan struct{})
	useRepositoryTestTransport(t, repositoryRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fetchRepositoryDetailsFromGithub(ctx, "example/project", "secret", 1, 1, 1)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, errNoContent) {
		t.Fatalf("incorrect cancellation: %v", err)
	}
}
