package glance

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var nasaAPODWidgetTemplate = mustParseTemplate("nasa-apod.html", "widget-base.html")

const nasaAPODURL = "https://science.nasa.gov/wp-json/wp/v2/apod-basic"

type nasaAPODWidget struct {
	widgetBase `yaml:",inline"`
	APOD       nasaAPOD `yaml:"-"`
}

type nasaAPOD struct {
	Date        string `json:"date"`
	Title       string `json:"title"`
	Permalink   string `json:"permalink"`
	MediaType   string `json:"media_type"`
	Explanation string `json:"explanation"`
	Credit      string `json:"credit"`
	Copyright   string `json:"copyright"`
	Alt         string `json:"alt"`
	HDURL       string `json:"hdurl"`
}

var nasaAPODHTMLTagPattern = regexp.MustCompile(`<[^>]*>`)

func (w *nasaAPODWidget) initialize() error {
	w.withTitle("NASA").withCacheDuration(24 * time.Hour)
	return nil
}

func (w *nasaAPODWidget) update(ctx context.Context) {
	providerURL := dailyDiscoveryProviderURL("/daily-discovery/nasa-apod", nasaAPODURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, providerURL, nil)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", glanceUserAgentString)

	response, err := decodeJsonFromRequest[[]nasaAPOD](defaultHTTPClient, req)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching NASA APOD: %w", err))
		return
	}

	apod, err := selectNASAAPOD(response, time.Now())
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}

	apod.Explanation = normalizeNASAAPODText(apod.Explanation)
	apod.Credit = normalizeNASAAPODText(apod.Credit)
	apod.Copyright = normalizeNASAAPODText(apod.Copyright)
	if apod.Credit != "" && apod.Credit == apod.Copyright {
		apod.Copyright = ""
	}

	if apod.MediaType == "image" {
		apod.HDURL = w.resolveResourceProxyImageURL(apod.HDURL)
		if apod.HDURL == "" {
			w.canContinueUpdateAfterHandlingErr(fmt.Errorf("NASA APOD image could not be rendered safely"))
			return
		}
	}

	if !w.canContinueUpdateAfterHandlingErr(nil) {
		return
	}

	w.APOD = apod
}

func (w *nasaAPODWidget) Render() template.HTML {
	return w.renderTemplate(w, nasaAPODWidgetTemplate)
}

func selectNASAAPOD(apods []nasaAPOD, now time.Time) (nasaAPOD, error) {
	if len(apods) == 0 {
		return nasaAPOD{}, fmt.Errorf("NASA APOD returned no entries")
	}

	today := now.Format("2006-01-02")
	for _, apod := range apods {
		if apod.Date == today {
			return apod, nil
		}
	}

	return apods[0], nil
}

func normalizeNASAAPODText(value string) string {
	value = strings.ReplaceAll(value, "<br>", " ")
	value = strings.ReplaceAll(value, "<br/>", " ")
	value = strings.ReplaceAll(value, "<br />", " ")
	value = nasaAPODHTMLTagPattern.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}
