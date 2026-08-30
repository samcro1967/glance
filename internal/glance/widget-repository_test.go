package glance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type repositoryRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f repositoryRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func repositoryTestResponse(
	request *http.Request,
	statusCode int,
	status string,
	body string,
) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func useRepositoryTestTransport(
	t *testing.T,
	transport http.RoundTripper,
) {
	t.Helper()

	originalTransport := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = transport

	t.Cleanup(func() {
		defaultHTTPClient.Transport = originalTransport
	})
}

func TestFetchRepositoryDetailsFromGithubSuccess(t *testing.T) {
	var mutex sync.Mutex
	requestedPaths := make(map[string]int)

	useRepositoryTestTransport(
		t,
		repositoryRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			mutex.Lock()
			requestedPaths[request.URL.Path]++
			mutex.Unlock()

			switch request.URL.Path {
			case "/repos/example/project":
				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`{
						"full_name":"example/project",
						"stargazers_count":123,
						"forks_count":45
					}`,
				), nil

			case "/search/issues":
				query := request.URL.Query().Get("q")

				switch {
				case strings.Contains(query, "is:pr"):
					return repositoryTestResponse(
						request,
						http.StatusOK,
						"200 OK",
						`{
							"total_count":7,
							"items":[
								{
									"title":"Pull request one",
									"number":101,
									"created_at":"2026-08-29T12:00:00Z"
								},
								{
									"title":"Pull request two",
									"number":102,
									"created_at":"2026-08-30T12:00:00Z"
								}
							]
						}`,
					), nil

				case strings.Contains(query, "is:issue"):
					return repositoryTestResponse(
						request,
						http.StatusOK,
						"200 OK",
						`{
							"total_count":9,
							"items":[
								{
									"title":"Issue one",
									"number":201,
									"created_at":"2026-08-28T12:00:00Z"
								}
							]
						}`,
					), nil
				}

			case "/repos/example/project/commits":
				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`[
						{
							"sha":"abc123",
							"commit":{
								"message":"First commit",
								"author":{
									"name":"Example Author",
									"date":"2026-08-30T10:00:00Z"
								}
							}
						}
					]`,
				), nil
			}

			return repositoryTestResponse(
				request,
				http.StatusNotFound,
				"404 Not Found",
				`{}`,
			), nil
		}),
	)

	details, err := fetchRepositoryDetailsFromGithub(
		context.Background(),
		"example/project",
		"",
		2,
		1,
		1,
	)
	if err != nil {
		t.Fatalf("fetchRepositoryDetailsFromGithub returned unexpected error: %v", err)
	}

	if details.Name != "example/project" {
		t.Fatalf("repository name = %q, want %q", details.Name, "example/project")
	}

	if details.Stars != 123 {
		t.Fatalf("stars = %d, want 123", details.Stars)
	}

	if details.Forks != 45 {
		t.Fatalf("forks = %d, want 45", details.Forks)
	}

	if details.OpenPullRequests != 7 {
		t.Fatalf("open pull requests = %d, want 7", details.OpenPullRequests)
	}

	if len(details.PullRequests) != 2 {
		t.Fatalf("pull request count = %d, want 2", len(details.PullRequests))
	}

	if details.PullRequests[0].Title != "Pull request one" {
		t.Fatalf(
			"first pull request title = %q, want %q",
			details.PullRequests[0].Title,
			"Pull request one",
		)
	}

	if details.OpenIssues != 9 {
		t.Fatalf("open issues = %d, want 9", details.OpenIssues)
	}

	if len(details.Issues) != 1 {
		t.Fatalf("issue count = %d, want 1", len(details.Issues))
	}

	if details.Issues[0].Title != "Issue one" {
		t.Fatalf(
			"first issue title = %q, want %q",
			details.Issues[0].Title,
			"Issue one",
		)
	}

	if len(details.Commits) != 1 {
		t.Fatalf("commit count = %d, want 1", len(details.Commits))
	}

	if details.Commits[0].Sha != "abc123" {
		t.Fatalf("commit SHA = %q, want %q", details.Commits[0].Sha, "abc123")
	}

	mutex.Lock()
	defer mutex.Unlock()

	if requestedPaths["/repos/example/project"] != 1 {
		t.Fatalf(
			"repository details requests = %d, want 1",
			requestedPaths["/repos/example/project"],
		)
	}

	if requestedPaths["/search/issues"] != 2 {
		t.Fatalf(
			"search requests = %d, want 2",
			requestedPaths["/search/issues"],
		)
	}

	if requestedPaths["/repos/example/project/commits"] != 1 {
		t.Fatalf(
			"commit requests = %d, want 1",
			requestedPaths["/repos/example/project/commits"],
		)
	}
}

func TestFetchRepositoryDetailsFromGithubDisabledSectionsAreNotRequested(t *testing.T) {
	var mutex sync.Mutex
	requestedPaths := make([]string, 0, 1)

	useRepositoryTestTransport(
		t,
		repositoryRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			mutex.Lock()
			requestedPaths = append(requestedPaths, request.URL.Path)
			mutex.Unlock()

			if request.URL.Path != "/repos/example/project" {
				return repositoryTestResponse(
					request,
					http.StatusInternalServerError,
					"500 Internal Server Error",
					`{}`,
				), nil
			}

			return repositoryTestResponse(
				request,
				http.StatusOK,
				"200 OK",
				`{
					"full_name":"example/project",
					"stargazers_count":10,
					"forks_count":2
				}`,
			), nil
		}),
	)

	details, err := fetchRepositoryDetailsFromGithub(
		context.Background(),
		"example/project",
		"",
		-1,
		-1,
		-1,
	)
	if err != nil {
		t.Fatalf("fetchRepositoryDetailsFromGithub returned unexpected error: %v", err)
	}

	if details.Name != "example/project" {
		t.Fatalf("repository name = %q, want %q", details.Name, "example/project")
	}

	mutex.Lock()
	defer mutex.Unlock()

	if len(requestedPaths) != 1 {
		t.Fatalf("request count = %d, want 1: %#v", len(requestedPaths), requestedPaths)
	}

	if requestedPaths[0] != "/repos/example/project" {
		t.Fatalf(
			"requested path = %q, want %q",
			requestedPaths[0],
			"/repos/example/project",
		)
	}
}

func TestFetchRepositoryDetailsFromGithubPartialFailurePreservesSuccessfulContent(t *testing.T) {
	useRepositoryTestTransport(
		t,
		repositoryRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			switch request.URL.Path {
			case "/repos/example/project":
				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`{
						"full_name":"example/project",
						"stargazers_count":100,
						"forks_count":20
					}`,
				), nil

			case "/search/issues":
				query := request.URL.Query().Get("q")

				if strings.Contains(query, "is:pr") {
					return repositoryTestResponse(
						request,
						http.StatusServiceUnavailable,
						"503 Service Unavailable",
						`{"message":"private provider detail"}`,
					), nil
				}

				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`{
						"total_count":4,
						"items":[
							{
								"title":"Working issue",
								"number":201,
								"created_at":"2026-08-30T12:00:00Z"
							}
						]
					}`,
				), nil
			}

			return repositoryTestResponse(
				request,
				http.StatusNotFound,
				"404 Not Found",
				`{}`,
			), nil
		}),
	)

	details, err := fetchRepositoryDetailsFromGithub(
		context.Background(),
		"example/project",
		"",
		3,
		3,
		-1,
	)
	if err == nil {
		t.Fatal("expected partial-content error")
	}

	if !errors.Is(err, errPartialContent) {
		t.Fatalf("error does not preserve partial-content classification: %v", err)
	}

	if errors.Is(err, errNoContent) {
		t.Fatalf("partial-content error unexpectedly classified as no-content: %v", err)
	}

	if !strings.Contains(err.Error(), "failed 1 of 2 repository sections") {
		t.Fatalf("error missing repository section failure count: %v", err)
	}

	if !strings.Contains(err.Error(), "unexpected HTTP status 503 Service Unavailable") {
		t.Fatalf("error missing safe representative cause: %v", err)
	}

	if strings.Contains(err.Error(), "private provider detail") {
		t.Fatalf("error exposed provider response body: %v", err)
	}

	if details.Name != "example/project" {
		t.Fatalf("repository name = %q, want %q", details.Name, "example/project")
	}

	if details.Stars != 100 || details.Forks != 20 {
		t.Fatalf(
			"repository metadata = stars %d forks %d, want stars 100 forks 20",
			details.Stars,
			details.Forks,
		)
	}

	if len(details.PullRequests) != 0 {
		t.Fatalf("pull requests = %#v, want none after failed PR section", details.PullRequests)
	}

	if details.OpenIssues != 4 {
		t.Fatalf("open issues = %d, want 4", details.OpenIssues)
	}

	if len(details.Issues) != 1 || details.Issues[0].Title != "Working issue" {
		t.Fatalf("issues = %#v, want successful issue content", details.Issues)
	}
}

func TestFetchRepositoryDetailsFromGithubDetailsFailureReturnsNoContent(t *testing.T) {
	useRepositoryTestTransport(
		t,
		repositoryRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return repositoryTestResponse(
				request,
				http.StatusForbidden,
				"403 Forbidden",
				`{"message":"private rate limit detail"}`,
			), nil
		}),
	)

	details, err := fetchRepositoryDetailsFromGithub(
		context.Background(),
		"example/project",
		"",
		-1,
		-1,
		-1,
	)
	if err == nil {
		t.Fatal("expected no-content error")
	}

	if !errors.Is(err, errNoContent) {
		t.Fatalf("error does not preserve no-content classification: %v", err)
	}

	if errors.Is(err, errPartialContent) {
		t.Fatalf("no-content error unexpectedly classified as partial-content: %v", err)
	}

	if !strings.Contains(err.Error(), "fetching repository details") {
		t.Fatalf("error missing repository-details context: %v", err)
	}

	if !strings.Contains(err.Error(), "unexpected HTTP status 403 Forbidden") {
		t.Fatalf("error missing safe HTTP status: %v", err)
	}

	if strings.Contains(err.Error(), "private rate limit detail") {
		t.Fatalf("error exposed provider response body: %v", err)
	}

	if details.Name != "" ||
		details.Stars != 0 ||
		details.Forks != 0 ||
		len(details.PullRequests) != 0 ||
		len(details.Issues) != 0 ||
		len(details.Commits) != 0 {
		t.Fatalf("details = %#v, want zero repository after mandatory request failure", details)
	}
}

func TestFetchRepositoryDetailsFromGithubAppliesBearerTokenToAllRequests(t *testing.T) {
	const token = "fake-test-token"

	var mutex sync.Mutex
	authorizationHeaders := make([]string, 0, 4)

	useRepositoryTestTransport(
		t,
		repositoryRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			mutex.Lock()
			authorizationHeaders = append(
				authorizationHeaders,
				request.Header.Get("Authorization"),
			)
			mutex.Unlock()

			switch request.URL.Path {
			case "/repos/example/project":
				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`{
						"full_name":"example/project",
						"stargazers_count":1,
						"forks_count":1
					}`,
				), nil

			case "/search/issues":
				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`{"total_count":0,"items":[]}`,
				), nil

			case "/repos/example/project/commits":
				return repositoryTestResponse(
					request,
					http.StatusOK,
					"200 OK",
					`[]`,
				), nil
			}

			return repositoryTestResponse(
				request,
				http.StatusNotFound,
				"404 Not Found",
				`{}`,
			), nil
		}),
	)

	_, err := fetchRepositoryDetailsFromGithub(
		context.Background(),
		"example/project",
		token,
		1,
		1,
		1,
	)
	if err != nil {
		t.Fatalf("fetchRepositoryDetailsFromGithub returned unexpected error: %v", err)
	}

	mutex.Lock()
	defer mutex.Unlock()

	if len(authorizationHeaders) != 4 {
		t.Fatalf(
			"authorization header count = %d, want 4",
			len(authorizationHeaders),
		)
	}

	for i, header := range authorizationHeaders {
		if header != "Bearer "+token {
			t.Fatalf(
				"authorization header %d = %q, want %q",
				i,
				header,
				"Bearer "+token,
			)
		}
	}
}

func TestFetchRepositoryDetailsFromGithubCancellationStopsInFlightRequests(t *testing.T) {
	const requestCount = 4

	requestStarted := make(chan struct{}, requestCount)
	requestCanceled := make(chan struct{}, requestCount)

	useRepositoryTestTransport(
		t,
		repositoryRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			requestStarted <- struct{}{}

			<-request.Context().Done()

			requestCanceled <- struct{}{}

			return nil, request.Context().Err()
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, err := fetchRepositoryDetailsFromGithub(
			ctx,
			"example/project",
			"",
			1,
			1,
			1,
		)
		result <- err
	}()

	for i := 0; i < requestCount; i++ {
		select {
		case <-requestStarted:
		case <-time.After(time.Second):
			cancel()
			t.Fatalf("request %d of %d did not start", i+1, requestCount)
		}
	}

	cancel()

	for i := 0; i < requestCount; i++ {
		select {
		case <-requestCanceled:
		case <-time.After(time.Second):
			t.Fatalf("request %d of %d did not observe cancellation", i+1, requestCount)
		}
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected cancellation error")
		}

		if !errors.Is(err, errNoContent) {
			t.Fatalf("error does not preserve no-content classification: %v", err)
		}

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error does not preserve context cancellation: %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("repository fetch did not return after cancellation")
	}
}
