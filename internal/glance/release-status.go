package glance

import (
	"context"
	"fmt"
	"log/slog"
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
	Status        releaseStatus
	LatestVersion string
	ReleaseURL    string
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
		result, err := checkForkReleaseStatus(ctx, currentVersion)
		if err != nil {
			slog.Warn(
				"Failed to check for fork release updates",
				"error", err,
			)
			cache.set(releaseStatusResult{})
			return
		}

		cache.set(result)
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
