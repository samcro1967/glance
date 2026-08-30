package glance

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"
)

var changeDetectionWidgetTemplate = mustParseTemplate("change-detection.html", "widget-base.html")

type changeDetectionWidget struct {
	widgetBase       `yaml:",inline"`
	ChangeDetections changeDetectionWatchList `yaml:"-"`
	WatchUUIDs       []string                 `yaml:"watches"`
	InstanceURL      string                   `yaml:"instance-url"`
	Token            string                   `yaml:"token"`
	Limit            int                      `yaml:"limit"`
	CollapseAfter    int                      `yaml:"collapse-after"`
}

func (widget *changeDetectionWidget) initialize() error {
	widget.withTitle("Change Detection").withCacheDuration(1 * time.Hour)

	if widget.Limit <= 0 {
		widget.Limit = 10
	}

	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}

	if widget.InstanceURL == "" {
		widget.InstanceURL = "https://www.changedetection.io"
	}

	return nil
}

func (widget *changeDetectionWidget) update(ctx context.Context) {
	if len(widget.WatchUUIDs) == 0 {
		uuids, err := fetchWatchUUIDsFromChangeDetection(ctx, widget.InstanceURL, string(widget.Token))

		if !widget.canContinueUpdateAfterHandlingErr(err) {
			return
		}

		widget.WatchUUIDs = uuids
	}

	watches, err := fetchWatchesFromChangeDetection(ctx, widget.InstanceURL, widget.WatchUUIDs, string(widget.Token))

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if len(watches) > widget.Limit {
		watches = watches[:widget.Limit]
	}

	widget.ChangeDetections = watches
}

func (widget *changeDetectionWidget) Render() template.HTML {
	return widget.renderTemplate(widget, changeDetectionWidgetTemplate)
}

type changeDetectionWatch struct {
	Title        string
	URL          string
	LastChanged  time.Time
	DiffURL      string
	PreviousHash string
}

type changeDetectionWatchList []changeDetectionWatch

func (r changeDetectionWatchList) sortByNewest() changeDetectionWatchList {
	sort.Slice(r, func(i, j int) bool {
		return r[i].LastChanged.After(r[j].LastChanged)
	})

	return r
}

type changeDetectionResponseJson struct {
	Title        string `json:"title"`
	URL          string `json:"url"`
	LastChanged  int64  `json:"last_changed"`
	DateCreated  int64  `json:"date_created"`
	PreviousHash string `json:"previous_md5"`
}

func fetchWatchUUIDsFromChangeDetection(ctx context.Context, instanceURL string, token string) ([]string, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/watch", instanceURL),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating change detection watch list request: %w", err)
	}

	if token != "" {
		request.Header.Add("x-api-key", token)
	}

	uuidsMap, err := decodeJsonFromRequest[map[string]struct{}](defaultHTTPClient, request)
	if err != nil {
		return nil, fmt.Errorf("fetching change detection watch list: %w", err)
	}

	uuids := make([]string, 0, len(uuidsMap))

	for uuid := range uuidsMap {
		uuids = append(uuids, uuid)
	}

	return uuids, nil
}

func fetchWatchesFromChangeDetection(ctx context.Context, instanceURL string, requestedWatchIDs []string, token string) (changeDetectionWatchList, error) {
	watches := make(changeDetectionWatchList, 0, len(requestedWatchIDs))

	if len(requestedWatchIDs) == 0 {
		return watches, nil
	}

	requests := make([]*http.Request, 0, len(requestedWatchIDs))

	for _, watchID := range requestedWatchIDs {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/api/v1/watch/%s", instanceURL, watchID),
			nil,
		)
		if err != nil {
			return nil, contentFetchError(
				errNoContent,
				1,
				len(requestedWatchIDs),
				"change detection watches",
				fmt.Errorf("creating change detection watch request: %w", err),
			)
		}

		if token != "" {
			request.Header.Add("x-api-key", token)
		}

		requests = append(requests, request)
	}

	task := decodeJsonFromRequestTask[changeDetectionResponseJson](defaultHTTPClient)
	job := newJob(task, requests).withWorkers(15).withContext(ctx)
	responses, errs, err := workerPoolDo(job)
	if err != nil {
		return nil, fmt.Errorf("%w: fetching change detection watches: %w", errNoContent, err)
	}

	failed := 0
	var firstFailure error

	for i := range responses {
		if errs[i] != nil {
			failed++

			if firstFailure == nil {
				firstFailure = errs[i]
			}

			continue
		}

		watchJson := responses[i]

		watch := changeDetectionWatch{
			URL:     watchJson.URL,
			DiffURL: fmt.Sprintf("%s/diff/%s?from_version=%d", instanceURL, requestedWatchIDs[i], watchJson.LastChanged-1),
		}

		if watchJson.LastChanged == 0 {
			watch.LastChanged = time.Unix(watchJson.DateCreated, 0)
		} else {
			watch.LastChanged = time.Unix(watchJson.LastChanged, 0)
		}

		if watchJson.Title != "" {
			watch.Title = watchJson.Title
		} else {
			watch.Title = strings.TrimPrefix(strings.Trim(stripURLScheme(watchJson.URL), "/"), "www.")
		}

		if watchJson.PreviousHash != "" {
			var hashLength = 8

			if len(watchJson.PreviousHash) < hashLength {
				hashLength = len(watchJson.PreviousHash)
			}

			watch.PreviousHash = watchJson.PreviousHash[0:hashLength]
		}

		watches = append(watches, watch)
	}

	if len(watches) == 0 {
		return nil, contentFetchError(
			errNoContent,
			failed,
			len(requestedWatchIDs),
			"change detection watches",
			firstFailure,
		)
	}

	watches.sortByNewest()

	if failed > 0 {
		return watches, contentFetchError(
			errPartialContent,
			failed,
			len(requestedWatchIDs),
			"change detection watches",
			firstFailure,
		)
	}

	return watches, nil
}
