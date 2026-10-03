package glance

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNASAAPODInitialize(t *testing.T) {
	widget := &nasaAPODWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize returned error: %v", err)
	}

	if widget.Title != "NASA" {
		t.Fatalf("expected default title NASA, got %q", widget.Title)
	}
	if widget.cacheDuration != 24*time.Hour {
		t.Fatalf("expected 24h cache duration, got %s", widget.cacheDuration)
	}
}

func TestNASAAPODSelectsToday(t *testing.T) {
	now := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	apods := []nasaAPOD{
		{Date: "2026-10-02", Title: "Yesterday"},
		{Date: "2026-10-03", Title: "Today"},
	}

	got, err := selectNASAAPOD(apods, now)
	if err != nil {
		t.Fatalf("selectNASAAPOD returned error: %v", err)
	}
	if got.Title != "Today" {
		t.Fatalf("expected todays APOD, got %q", got.Title)
	}
}

func TestNASAAPODFallsBackToFirstEntry(t *testing.T) {
	now := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	apods := []nasaAPOD{
		{Date: "2026-10-02", Title: "Newest"},
		{Date: "2026-10-01", Title: "Older"},
	}

	got, err := selectNASAAPOD(apods, now)
	if err != nil {
		t.Fatalf("selectNASAAPOD returned error: %v", err)
	}
	if got.Title != "Newest" {
		t.Fatalf("expected first APOD fallback, got %q", got.Title)
	}
}

func TestNASAAPODRejectsEmptyResponse(t *testing.T) {
	if _, err := selectNASAAPOD(nil, time.Now()); err == nil {
		t.Fatal("expected empty APOD response to return an error")
	}
}

func TestNASAAPODNormalizesProviderHTML(t *testing.T) {
	input := `<strong>Explanation:</strong> A <a href="https://example.com">galaxy</a> &amp; stars.<br><br>More text.`
	want := "Explanation: A galaxy & stars. More text."

	if got := normalizeNASAAPODText(input); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNASAAPODImageRenderUsesImageExpander(t *testing.T) {
	widget := &nasaAPODWidget{
		APOD: nasaAPOD{
			Title:       "Test APOD",
			Permalink:   "https://science.nasa.gov/test/",
			MediaType:   "image",
			Explanation: "Test explanation.",
			Alt:         "Test image",
			HDURL:       "https://example.com/test.jpg",
		},
	}
	widget.ContentAvailable = true

	rendered := string(widget.Render())
	for _, expected := range []string{
		`data-image-expand`,
		`data-image-expand-src="https://example.com/test.jpg"`,
		`Show Explanation`,
		`Test explanation.`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected rendered widget to contain %q; got %s", expected, rendered)
		}
	}
}

func TestNASAAPODNonImageRenderPreservesExistingBehavior(t *testing.T) {
	widget := &nasaAPODWidget{
		APOD: nasaAPOD{MediaType: "video", Title: "Video APOD"},
	}
	widget.ContentAvailable = true

	rendered := string(widget.Render())
	if !strings.Contains(rendered, "No image available today.") {
		t.Fatalf("expected non-image fallback; got %s", rendered)
	}
	if strings.Contains(rendered, "data-image-expand") {
		t.Fatalf("non-image APOD unexpectedly rendered image expansion; got %s", rendered)
	}
}

func TestNASAAPODRegistered(t *testing.T) {
	descriptor, ok := widgetRegistry["nasa-apod"]
	if !ok {
		t.Fatal("nasa-apod is not registered")
	}

	if _, ok := descriptor.constructor().(*nasaAPODWidget); !ok {
		t.Fatalf("nasa-apod constructor returned %T", descriptor.constructor())
	}
}

type nasaAPODTestRoundTripper func(*http.Request) (*http.Response, error)

func (f nasaAPODTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func useNASAAPODTestTransport(t *testing.T, fn nasaAPODTestRoundTripper) {
	t.Helper()
	originalTransport := defaultHTTPClient.Transport
	defaultHTTPClient.Transport = fn
	t.Cleanup(func() {
		defaultHTTPClient.Transport = originalTransport
	})
}

func nasaAPODTestResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: r,
	}
}

func TestNASAAPODUpdateAcquiresAndNormalizesProviderResponse(t *testing.T) {
	useNASAAPODTestTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != nasaAPODURL {
			t.Fatalf("request URL = %q, want %q", r.URL.String(), nasaAPODURL)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("Accept header = %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != glanceUserAgentString {
			t.Fatalf("User-Agent header = %q, want %q", r.Header.Get("User-Agent"), glanceUserAgentString)
		}

		return nasaAPODTestResponse(r, http.StatusOK, `[{"date":"2026-10-03","title":"Test APOD","permalink":"https://science.nasa.gov/test/","media_type":"image","explanation":"A <strong>galaxy</strong> &amp; stars.","credit":"NASA &amp; ESA","copyright":"Test Author","alt":"Test image","hdurl":"https://example.com/test.jpg"}]`), nil
	})

	widget := &nasaAPODWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("update error = %v", widget.Error)
	}
	if widget.APOD.Title != "Test APOD" {
		t.Fatalf("title = %q", widget.APOD.Title)
	}
	if widget.APOD.Explanation != "A galaxy & stars." {
		t.Fatalf("explanation = %q", widget.APOD.Explanation)
	}
	if widget.APOD.Credit != "NASA & ESA" {
		t.Fatalf("credit = %q", widget.APOD.Credit)
	}
	if widget.APOD.HDURL != "https://example.com/test.jpg" {
		t.Fatalf("image URL = %q", widget.APOD.HDURL)
	}
}

func TestNASAAPODUpdateDeduplicatesNormalizedAttribution(t *testing.T) {
	useNASAAPODTestTransport(t, func(r *http.Request) (*http.Response, error) {
		return nasaAPODTestResponse(r, http.StatusOK, `[{"date":"2026-10-03","title":"Test APOD","permalink":"https://science.nasa.gov/test/","media_type":"image","explanation":"Test explanation.","credit":"NASA &amp; ESA","copyright":"<strong>NASA &amp; ESA</strong>","alt":"Test image","hdurl":"https://example.com/test.jpg"}]`), nil
	})

	widget := &nasaAPODWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	widget.update(context.Background())

	if widget.Error != nil {
		t.Fatalf("update error = %v", widget.Error)
	}
	if widget.APOD.Credit != "NASA & ESA" {
		t.Fatalf("credit = %q", widget.APOD.Credit)
	}
	if widget.APOD.Copyright != "" {
		t.Fatalf("expected duplicate copyright attribution to be removed, got %q", widget.APOD.Copyright)
	}
}

func TestNASAAPODUpdateSurfacesProviderFailure(t *testing.T) {
	useNASAAPODTestTransport(t, func(r *http.Request) (*http.Response, error) {
		return nasaAPODTestResponse(r, http.StatusBadGateway, `{"error":"upstream failure"}`), nil
	})

	widget := &nasaAPODWidget{}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	widget.update(context.Background())

	if widget.Error == nil {
		t.Fatal("expected provider failure to surface as widget error")
	}
}
