// GitHub GraphQL implementation for authenticated repository widgets.
package glance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const githubRepositoryGraphQLQuery = `query RepositoryWidget($owner: String!, $name: String!, $prs: Boolean!, $issues: Boolean!, $commits: Boolean!, $prLimit: Int!, $issueLimit: Int!, $commitLimit: Int!) {
  repository(owner: $owner, name: $name) {
    nameWithOwner
    stargazerCount
    forkCount
    pullRequests(states: OPEN, first: $prLimit, orderBy: {field: CREATED_AT, direction: DESC}) @include(if: $prs) {
      totalCount
      nodes { number title createdAt }
    }
    issues(states: OPEN, first: $issueLimit, orderBy: {field: CREATED_AT, direction: DESC}) @include(if: $issues) {
      totalCount
      nodes { number title createdAt }
    }
    defaultBranchRef @include(if: $commits) {
      target {
        ... on Commit {
          history(first: $commitLimit) {
            nodes { oid message committedDate author { name date } }
          }
        }
      }
    }
  }
}`

type githubGraphQLTicketConnection struct {
	TotalCount int `json:"totalCount"`
	Nodes      []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		CreatedAt string `json:"createdAt"`
	} `json:"nodes"`
}

type githubGraphQLRepositoryResponse struct {
	Data struct {
		Repository *struct {
			Name             string                         `json:"nameWithOwner"`
			Stars            int                            `json:"stargazerCount"`
			Forks            int                            `json:"forkCount"`
			PullRequests     *githubGraphQLTicketConnection `json:"pullRequests"`
			Issues           *githubGraphQLTicketConnection `json:"issues"`
			DefaultBranchRef *struct {
				Target struct {
					History *struct {
						Nodes []struct {
							OID           string `json:"oid"`
							Message       string `json:"message"`
							CommittedDate string `json:"committedDate"`
							Author        *struct {
								Name string `json:"name"`
								Date string `json:"date"`
							} `json:"author"`
						} `json:"nodes"`
					} `json:"history"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Path []json.RawMessage `json:"path"`
	} `json:"errors"`
}

func githubGraphQLLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func fetchRepositoryDetailsFromGithubGraphQL(ctx context.Context, repo, token string, maxPRs, maxIssues, maxCommits int) (repository, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.TrimSpace(repo) != repo || strings.ContainsAny(repo, " \t\r\n?#") {
		return repository{}, fmt.Errorf("%w: invalid GitHub repository identifier", errNoContent)
	}

	variables := map[string]any{
		"owner": parts[0], "name": parts[1],
		"prs": maxPRs > 0, "issues": maxIssues > 0, "commits": maxCommits > 0,
		"prLimit":     githubGraphQLLimit(maxPRs),
		"issueLimit":  githubGraphQLLimit(maxIssues),
		"commitLimit": githubGraphQLLimit(maxCommits),
	}
	payload, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{githubRepositoryGraphQLQuery, variables})
	if err != nil {
		return repository{}, fmt.Errorf("%w: encoding repository request: %w", errNoContent, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(payload))
	if err != nil {
		return repository{}, fmt.Errorf("%w: creating repository request: %w", errNoContent, err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	response, err := decodeJsonFromRequest[githubGraphQLRepositoryResponse](defaultHTTPClient, request)
	if err != nil {
		return repository{}, fmt.Errorf("%w: fetching repository details: %w", errNoContent, err)
	}
	if response.Data.Repository == nil || response.Data.Repository.Name == "" {
		return repository{}, fmt.Errorf("%w: GitHub GraphQL repository data unavailable", errNoContent)
	}

	details := repository{
		Name:  response.Data.Repository.Name,
		Stars: response.Data.Repository.Stars,
		Forks: response.Data.Repository.Forks,
	}
	sectionErrors := map[string]bool{}
	for _, gqlErr := range response.Errors {
		if len(gqlErr.Path) < 2 {
			return repository{}, fmt.Errorf("%w: GitHub GraphQL repository query failed", errNoContent)
		}
		var root, field string
		if json.Unmarshal(gqlErr.Path[0], &root) != nil || root != "repository" || json.Unmarshal(gqlErr.Path[1], &field) != nil {
			return repository{}, fmt.Errorf("%w: GitHub GraphQL repository query failed", errNoContent)
		}
		switch field {
		case "pullRequests", "issues", "defaultBranchRef":
			sectionErrors[field] = true
		default:
			return repository{}, fmt.Errorf("%w: GitHub GraphQL repository query failed", errNoContent)
		}
	}

	failed, total := 0, 0
	var firstFailure error
	fail := func(section string) {
		failed++
		if firstFailure == nil {
			firstFailure = fmt.Errorf("fetching %s: GitHub GraphQL section unavailable", section)
		}
	}
	if maxPRs > 0 {
		total++
		if sectionErrors["pullRequests"] || response.Data.Repository.PullRequests == nil {
			fail("pull requests")
		} else {
			connection := response.Data.Repository.PullRequests
			details.OpenPullRequests = connection.TotalCount
			for _, item := range connection.Nodes {
				details.PullRequests = append(details.PullRequests, githubTicket{Number: item.Number, Title: item.Title, CreatedAt: parseRFC3339Time(item.CreatedAt)})
			}
		}
	}
	if maxIssues > 0 {
		total++
		if sectionErrors["issues"] || response.Data.Repository.Issues == nil {
			fail("issues")
		} else {
			connection := response.Data.Repository.Issues
			details.OpenIssues = connection.TotalCount
			for _, item := range connection.Nodes {
				details.Issues = append(details.Issues, githubTicket{Number: item.Number, Title: item.Title, CreatedAt: parseRFC3339Time(item.CreatedAt)})
			}
		}
	}
	if maxCommits > 0 {
		total++
		branch := response.Data.Repository.DefaultBranchRef
		if sectionErrors["defaultBranchRef"] {
			fail("commits")
		} else if branch != nil && branch.Target.History == nil {
			fail("commits")
		} else if branch != nil {
			for _, item := range branch.Target.History.Nodes {
				author, date := "", item.CommittedDate
				if item.Author != nil {
					author = item.Author.Name
					if item.Author.Date != "" {
						date = item.Author.Date
					}
				}
				details.Commits = append(details.Commits, githubCommitDetails{
					Sha: item.OID, Author: author, CreatedAt: parseRFC3339Time(date), Message: strings.SplitN(item.Message, "\n\n", 2)[0],
				})
			}
		}
	}
	if failed > 0 {
		return details, contentFetchError(errPartialContent, failed, total, "repository sections", firstFailure)
	}
	return details, nil
}
