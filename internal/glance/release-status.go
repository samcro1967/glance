package glance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const (
	forkReleaseRepository    = "samcro1967/glance"
	forkReleaseCheckInterval = 6 * time.Hour
)

var forkReleaseVersionPattern = regexp.MustCompile(
	`^v([0-9]+)\.([0-9]+)\.([0-9]+)-samcro1967\.r([0-9]+)$`,
)

type releaseStatus int

const (
	releaseStatusUnknown releaseStatus = iota
	releaseStatusLatest
	releaseStatusUpdateAvailable
)

type releaseStatusResult struct {
	Status               releaseStatus
	LatestVersion        string
	ReleaseURL           string
	InstalledPublishedAt time.Time
}

func (result releaseStatusResult) InstalledReleaseTooltip() string {
	if result.InstalledPublishedAt.IsZero() {
		return ""
	}
	return "Released " + result.InstalledPublishedAt.UTC().Format("January 2, 2006") + " (UTC)"
}

func (result releaseStatusResult) IsLatest() bool {
	return result.Status == releaseStatusLatest
}

func (result releaseStatusResult) IsUpdateAvailable() bool {
	return result.Status == releaseStatusUpdateAvailable
}

type releaseStatusCache struct {
	mu     sync.RWMutex
	result releaseStatusResult
}

func (cache *releaseStatusCache) get() releaseStatusResult {
	cache.mu.RLock()
	defer cache.mu.RUnlock()

	return cache.result
}

func (cache *releaseStatusCache) set(result releaseStatusResult) {
	cache.mu.Lock()
	cache.result = result
	cache.mu.Unlock()
}

type forkReleaseVersion struct {
	Major    int
	Minor    int
	Patch    int
	Revision int
}

func parseForkReleaseVersion(version string) (forkReleaseVersion, bool) {
	matches := forkReleaseVersionPattern.FindStringSubmatch(version)
	if matches == nil {
		return forkReleaseVersion{}, false
	}

	values := make([]int, 4)
	for i := range values {
		value, err := strconv.Atoi(matches[i+1])
		if err != nil {
			return forkReleaseVersion{}, false
		}

		values[i] = value
	}

	return forkReleaseVersion{
		Major:    values[0],
		Minor:    values[1],
		Patch:    values[2],
		Revision: values[3],
	}, true
}

func compareForkReleaseVersions(a, b forkReleaseVersion) int {
	aValues := [...]int{a.Major, a.Minor, a.Patch, a.Revision}
	bValues := [...]int{b.Major, b.Minor, b.Patch, b.Revision}

	for i := range aValues {
		switch {
		case aValues[i] < bValues[i]:
			return -1
		case aValues[i] > bValues[i]:
			return 1
		}
	}

	return 0
}

func checkForkReleaseStatus(
	ctx context.Context,
	currentVersion string,
) (releaseStatusResult, error) {
	if currentVersion == "dev" {
		return releaseStatusResult{}, nil
	}

	current, ok := parseForkReleaseVersion(currentVersion)
	if !ok {
		return releaseStatusResult{}, nil
	}

	latestRelease, err := fetchLatestGithubRelease(ctx, &releaseRequest{
		Repository: forkReleaseRepository,
		source:     releaseSourceGithub,
	})
	if err != nil {
		return releaseStatusResult{}, fmt.Errorf("fetching latest fork release: %w", err)
	}

	latest, ok := parseForkReleaseVersion(latestRelease.Version)
	if !ok {
		return releaseStatusResult{
			LatestVersion: latestRelease.Version,
			ReleaseURL:    latestRelease.NotesUrl,
		}, nil
	}

	result := releaseStatusResult{
		LatestVersion: latestRelease.Version,
		ReleaseURL:    latestRelease.NotesUrl,
	}

	switch compareForkReleaseVersions(current, latest) {
	case 0:
		result.Status = releaseStatusLatest
	case -1:
		result.Status = releaseStatusUpdateAvailable
	}

	return result, nil
}

// Fetch the exact installed tag: the latest release may describe another version.
// Publication dates are optional metadata and must never fall back to time.Now.
func fetchInstalledForkReleaseDate(ctx context.Context, version string) (time.Time, error) {
	if _, ok := parseForkReleaseVersion(version); !ok {
		return time.Time{}, nil
	}
	requestURL := "https://api.github.com/repos/" + forkReleaseRepository + "/releases/tags/" + url.PathEscape(version)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("creating installed fork release request: %w", err)
	}
	response, err := decodeJsonFromRequest[githubReleaseResponseJson](defaultHTTPClient, request)
	if err != nil {
		return time.Time{}, fmt.Errorf("fetching installed fork release: %w", err)
	}
	if response.TagName != version {
		return time.Time{}, errors.New("installed fork release response has a different tag")
	}
	publishedAt, err := time.Parse(time.RFC3339, response.PublishedAt)
	if err != nil || publishedAt.IsZero() {
		return time.Time{}, errors.New("installed fork release has no valid publication date")
	}
	return publishedAt.UTC(), nil
}

// The checker is the sole writer. Retain confirmed installed-release metadata
// independently of transient latest-release failures; retry missing dates on
// the existing check schedule rather than adding a second background worker.
func updateForkReleaseStatus(ctx context.Context, version string, cache *releaseStatusCache) {
	publishedAt := cache.get().InstalledPublishedAt
	if publishedAt.IsZero() {
		var err error
		publishedAt, err = fetchInstalledForkReleaseDate(ctx, version)
		if err != nil {
			slog.Warn("Failed to look up installed fork release date", "error", err)
		}
	}
	result, err := checkForkReleaseStatus(ctx, version)
	if err != nil {
		slog.Warn("Failed to check for fork release updates", "error", err)
	}
	result.InstalledPublishedAt = publishedAt
	cache.set(result)
}

func runForkReleaseStatusChecker(
	ctx context.Context,
	currentVersion string,
	cache *releaseStatusCache,
) {
	if currentVersion == "dev" {
		return
	}

	if _, ok := parseForkReleaseVersion(currentVersion); !ok {
		return
	}

	check := func() {
		updateForkReleaseStatus(ctx, currentVersion, cache)
	}

	check()

	ticker := time.NewTicker(forkReleaseCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
