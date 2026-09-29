package glance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDilbertWidgetInitialize(t *testing.T) {
	widget := &dilbertWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned an error: %v", err)
	}

	if widget.Title != "Dilbert" {
		t.Fatalf("unexpected default title: %q", widget.Title)
	}
	if widget.cacheDuration != 24*time.Hour {
		t.Fatalf("unexpected default cache duration: %s", widget.cacheDuration)
	}
	if widget.ArchiveURL != defaultDilbertArchiveURL {
		t.Fatalf("unexpected default archive URL: %q", widget.ArchiveURL)
	}
}

func TestDilbertWidgetInitializeDateValidation(t *testing.T) {
	validDates := []string{"1989-04-16", "2023-03-12"}
	for _, date := range validDates {
		t.Run("valid "+date, func(t *testing.T) {
			widget := &dilbertWidget{Date: date}
			if err := widget.initialize(); err != nil {
				t.Fatalf("initialize returned an error: %v", err)
			}
		})
	}

	invalidDates := []string{"1989-04-15", "2023-03-13", "not-a-date"}
	for _, date := range invalidDates {
		t.Run("invalid "+date, func(t *testing.T) {
			widget := &dilbertWidget{Date: date}
			if err := widget.initialize(); err == nil {
				t.Fatal("expected initialize to return an error")
			}
		})
	}
}

func TestDilbertWidgetInitializeArchiveURLValidation(t *testing.T) {
	for _, archiveURL := range []string{"archive.org", "://bad", "file:///tmp/archive", "https://example.com?query=1", "https://user@example.com"} {
		t.Run(archiveURL, func(t *testing.T) {
			widget := &dilbertWidget{ArchiveURL: archiveURL}
			if err := widget.initialize(); err == nil {
				t.Fatal("expected initialize to return an error")
			}
		})
	}
}

func TestRandomDilbertDateWithinArchiveRange(t *testing.T) {
	for range 10000 {
		date := randomDilbertDate()
		if date.Before(dilbertFirstDate) || date.After(dilbertLastDate) {
			t.Fatalf("random date outside archive range: %s", date)
		}
	}
}

func TestParseDilbertComicOldMarkup(t *testing.T) {
	body := []byte(`
		<html><head>
		<meta property="og:image" content="https://web.archive.org/web/20150302105510im_/http://assets.amuniversal.com/wrong-image">
		</head><body>
		<img class="img-responsive img-comic" width="900" height="284" src="https://web.archive.org/web/20150319094252im_/http://assets.amuniversal.com/correct-image">
		<div id="js-toggle-element-2005-06-15" data-id="2005-06-15" data-date="June 15, 2005" data-title=""></div>
		</body></html>`)

	comic, err := parseDilbertComic(body, "2005-06-15", defaultDilbertArchiveURL)
	if err != nil {
		t.Fatalf("parse returned an error: %v", err)
	}
	if comic.ImageURL != "https://web.archive.org/web/20150319094252im_/http://assets.amuniversal.com/correct-image" {
		t.Fatalf("parser did not select rendered comic image: %q", comic.ImageURL)
	}
	if strings.Contains(comic.ImageURL, "wrong-image") {
		t.Fatal("parser incorrectly selected og:image")
	}
	if comic.Width != 900 || comic.Height != 284 {
		t.Fatalf("unexpected dimensions: %dx%d", comic.Width, comic.Height)
	}
	if comic.Date != "2005-06-15" {
		t.Fatalf("unexpected comic date: %q", comic.Date)
	}
}

func TestParseDilbertComicNewMarkup(t *testing.T) {
	body := []byte(`
		<html><body>
		<div class="comic-item-container js-comic" data-id="2020-06-15" data-date="June 15, 2020" data-title="Real Data ">
			<img class="img-responsive img-comic" width="900" height="280" src="//web.archive.org/web/20200615230526im_/https://assets.amuniversal.com/comic">
		</div>
		</body></html>`)

	comic, err := parseDilbertComic(body, "2020-06-15", defaultDilbertArchiveURL)
	if err != nil {
		t.Fatalf("parse returned an error: %v", err)
	}
	if comic.Title != "Real Data" {
		t.Fatalf("unexpected title: %q", comic.Title)
	}
	if comic.ImageURL != "https://web.archive.org/web/20200615230526im_/https://assets.amuniversal.com/comic" {
		t.Fatalf("unexpected normalized image URL: %q", comic.ImageURL)
	}
}

func TestParseDilbertComicRejectsWrongDateAndMissingImage(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "wrong date",
			body: `<div data-id="2023-03-11"><img class="img-comic" src="https://web.archive.org/comic"></div>`,
		},
		{
			name: "missing image",
			body: `<div data-id="2023-03-12"></div>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseDilbertComic([]byte(test.body), "2023-03-12", defaultDilbertArchiveURL)
			if !errors.Is(err, errDilbertComicUnavailable) {
				t.Fatalf("expected unavailable error, got %v", err)
			}
		})
	}
}

func TestParseDilbertComicSuppressesGenericTitle(t *testing.T) {
	body := []byte(`<div data-id="2022-06-15" data-title="Dilbert Comic for 2022-06-15"><img class="img-comic" src="https://web.archive.org/comic"></div>`)
	comic, err := parseDilbertComic(body, "2022-06-15", defaultDilbertArchiveURL)
	if err != nil {
		t.Fatalf("parse returned an error: %v", err)
	}
	if comic.Title != "" {
		t.Fatalf("generic title should be suppressed, got %q", comic.Title)
	}
}

func TestFetchDilbertComicRequest(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected browser user agent")
		}
		_, _ = fmt.Fprint(w, `<div data-id="2011-05-12"><img class="img-comic" src="https://web.archive.org/comic"></div>`)
	}))
	defer server.Close()

	date := time.Date(2011, time.May, 12, 0, 0, 0, 0, time.UTC)
	comic, err := fetchDilbertComic(context.Background(), newHTTPClient(0, false), server.URL, date)
	if err != nil {
		t.Fatalf("fetch returned an error: %v", err)
	}
	if receivedPath != "/web/20110512/http://dilbert.com/strip/2011-05-12" {
		t.Fatalf("unexpected request path: %q", receivedPath)
	}
	if comic.Date != "2011-05-12" {
		t.Fatalf("unexpected comic date: %q", comic.Date)
	}
}

func TestFetchDilbertComicRetriesOnlyUnavailableContent(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := requests.Add(1)
		date := r.URL.Path[len(r.URL.Path)-10:]
		if requestNumber < 3 {
			_, _ = fmt.Fprintf(w, `<div data-id="%s"></div>`, date)
			return
		}
		_, _ = fmt.Fprintf(w, `<div data-id="%s"><img class="img-comic" src="https://web.archive.org/comic"></div>`, date)
	}))
	defer server.Close()

	comic, err := fetchDilbertComicWithRetries(
		context.Background(),
		newHTTPClient(0, false),
		server.URL,
		func() time.Time { return time.Date(2011, time.May, 12, 0, 0, 0, 0, time.UTC) },
		dilbertMaxAttempts,
	)
	if err != nil {
		t.Fatalf("fetch returned an error: %v", err)
	}
	if requests.Load() != 3 {
		t.Fatalf("unexpected request count: %d", requests.Load())
	}
	if comic.Date != "2011-05-12" {
		t.Fatalf("unexpected comic date: %q", comic.Date)
	}
}

func TestFetchDilbertComicDoesNotRetryHTTPFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := fetchDilbertComicWithRetries(
		context.Background(),
		newHTTPClient(0, false),
		server.URL,
		func() time.Time { return time.Date(2011, time.May, 12, 0, 0, 0, 0, time.UTC) },
		dilbertMaxAttempts,
	)
	if err == nil {
		t.Fatal("expected fetch error")
	}
	if requests.Load() != 1 {
		t.Fatalf("HTTP failure should not be retried, got %d requests", requests.Load())
	}

	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected preserved 429 status error, got %v", err)
	}
}

func TestFetchDilbertComicHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetchDilbertComicWithRetries(
		ctx,
		newHTTPClient(0, false),
		"http://127.0.0.1:1",
		func() time.Time { return time.Date(2011, time.May, 12, 0, 0, 0, 0, time.UTC) },
		dilbertMaxAttempts,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestDilbertWidgetRegistered(t *testing.T) {
	descriptor, ok := widgetRegistry["dilbert"]
	if !ok {
		t.Fatal("dilbert widget is not registered")
	}
	if _, ok := descriptor.constructor().(*dilbertWidget); !ok {
		t.Fatalf("dilbert constructor returned %T", descriptor.constructor())
	}
}
