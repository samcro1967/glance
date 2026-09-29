package glance

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var dilbertWidgetTemplate = mustParseTemplate("dilbert.html", "widget-base.html")

var errDilbertComicUnavailable = errors.New("dilbert comic unavailable")

const (
	defaultDilbertArchiveURL = "https://web.archive.org"
	dilbertMaxAttempts       = 3
	dilbertHTTPTimeout       = 15 * time.Second
)

var (
	dilbertFirstDate = time.Date(1989, time.April, 16, 0, 0, 0, 0, time.UTC)
	dilbertLastDate  = time.Date(2023, time.March, 12, 0, 0, 0, 0, time.UTC)
)

type dilbertWidget struct {
	widgetBase `yaml:",inline"`
	Comic      dilbertComic `yaml:"-"`
	Date       string       `yaml:"date"`
	ArchiveURL string       `yaml:"archive-url"`
}

type dilbertComic struct {
	Date     string
	Title    string
	ImageURL string
	Width    int
	Height   int
}

func (widget *dilbertWidget) initialize() error {
	widget.withTitle("Dilbert").withCacheDuration(24 * time.Hour)

	if widget.ArchiveURL == "" {
		widget.ArchiveURL = defaultDilbertArchiveURL
	}
	widget.ArchiveURL = strings.TrimRight(strings.TrimSpace(widget.ArchiveURL), "/")

	archiveURL, err := url.Parse(widget.ArchiveURL)
	if err != nil || archiveURL.User != nil || archiveURL.Host == "" || archiveURL.RawQuery != "" || archiveURL.Fragment != "" || (archiveURL.Scheme != "http" && archiveURL.Scheme != "https") {
		return fmt.Errorf("archive-url must be an absolute HTTP or HTTPS URL")
	}

	if widget.Date != "" {
		parsedDate, err := time.Parse("2006-01-02", widget.Date)
		if err != nil {
			return fmt.Errorf("date must use YYYY-MM-DD format: %w", err)
		}
		parsedDate = parsedDate.UTC()
		if parsedDate.Before(dilbertFirstDate) || parsedDate.After(dilbertLastDate) {
			return fmt.Errorf("date must be between %s and %s", formatDilbertDate(dilbertFirstDate), formatDilbertDate(dilbertLastDate))
		}
	}

	return nil
}

func (widget *dilbertWidget) update(ctx context.Context) {
	client := newHTTPClient(durationField(dilbertHTTPTimeout), false)

	maxAttempts := dilbertMaxAttempts
	if widget.Date != "" {
		maxAttempts = 1
	}

	comic, err := fetchDilbertComicWithRetries(
		ctx,
		client,
		widget.ArchiveURL,
		widget.selectDate,
		maxAttempts,
	)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	comic.ImageURL = widget.resolveResourceProxyImageURL(comic.ImageURL)
	if comic.ImageURL == "" {
		widget.canContinueUpdateAfterHandlingErr(fmt.Errorf("%w: comic image URL could not be rendered safely", errDilbertComicUnavailable))
		return
	}

	if !widget.canContinueUpdateAfterHandlingErr(nil) {
		return
	}

	widget.Comic = comic
}

func (widget *dilbertWidget) selectDate() time.Time {
	if widget.Date != "" {
		date, _ := time.Parse("2006-01-02", widget.Date)
		return date.UTC()
	}

	return randomDilbertDate()
}

func (widget *dilbertWidget) Render() template.HTML {
	return widget.renderTemplate(widget, dilbertWidgetTemplate)
}

func randomDilbertDate() time.Time {
	days := int(dilbertLastDate.Sub(dilbertFirstDate).Hours()/24) + 1
	return dilbertFirstDate.AddDate(0, 0, rand.IntN(days))
}

func fetchDilbertComicWithRetries(
	ctx context.Context,
	client requestDoer,
	archiveURL string,
	selectDate func() time.Time,
	maxAttempts int,
) (dilbertComic, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return dilbertComic{}, err
		}

		date := selectDate()
		comic, err := fetchDilbertComic(ctx, client, archiveURL, date)
		if err == nil {
			return comic, nil
		}

		if !errors.Is(err, errDilbertComicUnavailable) {
			return dilbertComic{}, err
		}

		lastErr = err
	}

	return dilbertComic{}, fmt.Errorf("%w after %d attempts: %v", errDilbertComicUnavailable, maxAttempts, lastErr)
}

func fetchDilbertComic(
	ctx context.Context,
	client requestDoer,
	archiveURL string,
	date time.Time,
) (dilbertComic, error) {
	date = date.UTC()
	requestedDate := formatDilbertDate(date)
	requestURL := fmt.Sprintf(
		"%s/web/%s/http://dilbert.com/strip/%s",
		strings.TrimRight(archiveURL, "/"),
		date.Format("20060102"),
		requestedDate,
	)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return dilbertComic{}, fmt.Errorf("creating Dilbert archive request: %w", err)
	}
	setBrowserUserAgentHeader(request)

	body, err := fetchHTTPResponseBody(client, request)
	if err != nil {
		return dilbertComic{}, fmt.Errorf("fetching Dilbert comic for %s: %w", requestedDate, err)
	}

	comic, err := parseDilbertComic(body, requestedDate, archiveURL)
	if err != nil {
		return dilbertComic{}, err
	}

	return comic, nil
}

func parseDilbertComic(body []byte, requestedDate string, archiveURL string) (dilbertComic, error) {
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return dilbertComic{}, fmt.Errorf("%w: parsing archived Dilbert page: %v", errDilbertComicUnavailable, err)
	}

	var comic dilbertComic
	var dateFound bool

	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if value, ok := htmlAttribute(node, "data-id"); ok && value == requestedDate {
				dateFound = true
				comic.Date = requestedDate
				if title, ok := htmlAttribute(node, "data-title"); ok {
					comic.Title = normalizeDilbertTitle(title, requestedDate)
				}
			}

			if node.Data == "img" && hasHTMLClass(node, "img-comic") && comic.ImageURL == "" {
				if src, ok := htmlAttribute(node, "src"); ok {
					comic.ImageURL = normalizeDilbertImageURL(src, archiveURL)
				}
				if width, ok := htmlAttribute(node, "width"); ok {
					comic.Width, _ = strconv.Atoi(width)
				}
				if height, ok := htmlAttribute(node, "height"); ok {
					comic.Height, _ = strconv.Atoi(height)
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)

	if !dateFound {
		return dilbertComic{}, fmt.Errorf("%w: archived page does not identify comic date %s", errDilbertComicUnavailable, requestedDate)
	}
	if comic.ImageURL == "" {
		return dilbertComic{}, fmt.Errorf("%w: archived page for %s has no rendered comic image", errDilbertComicUnavailable, requestedDate)
	}

	return comic, nil
}

func htmlAttribute(node *html.Node, name string) (string, bool) {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return strings.TrimSpace(attribute.Val), true
		}
	}
	return "", false
}

func hasHTMLClass(node *html.Node, className string) bool {
	classes, ok := htmlAttribute(node, "class")
	if !ok {
		return false
	}
	for _, class := range strings.Fields(classes) {
		if class == className {
			return true
		}
	}
	return false
}

func normalizeDilbertImageURL(rawURL string, archiveURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	if strings.HasPrefix(rawURL, "/") {
		return strings.TrimRight(archiveURL, "/") + rawURL
	}
	return rawURL
}

func normalizeDilbertTitle(title string, date string) string {
	title = strings.TrimSpace(title)
	if title == "" || strings.EqualFold(title, "Dilbert Comic for "+date) {
		return ""
	}
	return title
}

func formatDilbertDate(date time.Time) string {
	return date.UTC().Format("2006-01-02")
}
