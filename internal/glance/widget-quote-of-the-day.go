package glance

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var quoteOfTheDayWidgetTemplate = mustParseTemplate("quote-of-the-day.html", "widget-base.html")

const wikiquoteAPIURL = "https://en.wikiquote.org/w/api.php"

var (
	wikiquoteCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)
	wikiquoteLinkPattern    = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	wikiquoteBreakPattern   = regexp.MustCompile(`(?i)<br\s*/?>`)
	wikiquoteTagPattern     = regexp.MustCompile(`<[^>]+>`)
)

type quoteOfTheDayWidget struct {
	widgetBase `yaml:",inline"`
	Entry      quoteOfTheDayEntry `yaml:"-"`
}
type quoteOfTheDayEntry struct{ Quote, Author, AuthorURL, SourceURL, Date string }
type wikiquoteParseResponse struct {
	Parse struct {
		Title    string `json:"title"`
		Wikitext string `json:"wikitext"`
	} `json:"parse"`
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error"`
}

func (w *quoteOfTheDayWidget) initialize() error {
	w.withTitle("Quote of the Day").withCacheDuration(24 * time.Hour)
	return nil
}
func (w *quoteOfTheDayWidget) update(ctx context.Context) {
	now := time.Now()
	page := wikiquoteDailyPageName(now)
	endpoint, err := url.Parse(dailyDiscoveryProviderURL("/daily-discovery/wikiquote", wikiquoteAPIURL))
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	q := endpoint.Query()
	q.Set("action", "parse")
	q.Set("page", page)
	q.Set("prop", "wikitext")
	q.Set("format", "json")
	q.Set("formatversion", "2")
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	req.Header.Set("User-Agent", glanceUserAgentString)
	response, err := decodeJsonFromRequest[wikiquoteParseResponse](defaultHTTPClient, req)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching Wikiquote quote of the day: %w", err))
		return
	}
	if response.Error != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching Wikiquote quote of the day: %s: %s", response.Error.Code, response.Error.Info))
		return
	}
	entry, err := parseWikiquoteQuoteOfTheDay(response.Parse.Wikitext, now)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	if !w.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	w.Entry = entry
}
func (w *quoteOfTheDayWidget) Render() template.HTML {
	return w.renderTemplate(w, quoteOfTheDayWidgetTemplate)
}
func wikiquoteDailyPageName(now time.Time) string {
	return "Wikiquote:Quote of the day/" + now.Format("January 2, 2006")
}
func wikiquotePageURL(page string) string {
	return "https://en.wikiquote.org/wiki/" + url.PathEscape(strings.ReplaceAll(page, " ", "_"))
}

func parseWikiquoteQuoteOfTheDay(wikitext string, now time.Time) (quoteOfTheDayEntry, error) {
	body, err := wikiquoteQOTDTemplateBody(wikitext)
	if err != nil {
		return quoteOfTheDayEntry{}, err
	}
	params := wikiquoteTemplateParams(body)
	quote := normalizeWikiquoteText(params["quote"], true)
	author := normalizeWikiquoteAuthor(params["author"])
	if quote == "" {
		return quoteOfTheDayEntry{}, fmt.Errorf("Wikiquote quote of the day entry has no quote")
	}
	if author == "" {
		return quoteOfTheDayEntry{}, fmt.Errorf("Wikiquote quote of the day entry has no author")
	}
	page := wikiquoteDailyPageName(now)
	return quoteOfTheDayEntry{Quote: quote, Author: author, AuthorURL: wikiquotePageURL(author), SourceURL: wikiquotePageURL(page), Date: now.Format("2006-01-02")}, nil
}

func wikiquoteQOTDTemplateBody(wikitext string) (string, error) {
	lower := strings.ToLower(wikitext)
	names := []string{"{{wikiquote:quote of the day/template", "{{quote of the day"}
	start := -1
	for _, name := range names {
		if i := strings.Index(lower, name); i >= 0 && (start < 0 || i < start) {
			start = i
		}
	}
	if start < 0 {
		return "", fmt.Errorf("Wikiquote quote of the day template was not found")
	}
	depth := 0
	for i := start; i+1 < len(wikitext); i++ {
		switch wikitext[i : i+2] {
		case "{{":
			depth++
			i++
		case "}}":
			depth--
			if depth == 0 {
				return wikitext[start+2 : i], nil
			}
			i++
		}
	}
	return "", fmt.Errorf("Wikiquote quote of the day template was not closed")
}
func wikiquoteTemplateParams(body string) map[string]string {
	parts := splitWikiquoteTemplateParts(body)
	params := make(map[string]string)
	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, "=")
		if ok {
			params[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	return params
}
func splitWikiquoteTemplateParts(body string) []string {
	var parts []string
	start, templateDepth, linkDepth := 0, 0, 0
	inComment := false
	for i := 0; i < len(body); i++ {
		if !inComment && i+3 < len(body) && body[i:i+4] == "<!--" {
			inComment = true
			i += 3
			continue
		}
		if inComment {
			if i+2 < len(body) && body[i:i+3] == "-->" {
				inComment = false
				i += 2
			}
			continue
		}
		if i+1 < len(body) {
			pair := body[i : i+2]
			switch pair {
			case "{{":
				templateDepth++
				i++
				continue
			case "}}":
				if templateDepth > 0 {
					templateDepth--
				}
				i++
				continue
			case "[[":
				linkDepth++
				i++
				continue
			case "]]":
				if linkDepth > 0 {
					linkDepth--
				}
				i++
				continue
			}
		}
		if body[i] == '|' && templateDepth == 0 && linkDepth == 0 {
			parts = append(parts, body[start:i])
			start = i + 1
		}
	}
	return append(parts, body[start:])
}
func normalizeWikiquoteAuthor(value string) string {
	value = wikiquoteCommentPattern.ReplaceAllString(value, "")
	if i := wikiquoteBreakPattern.FindStringIndex(value); i != nil {
		value = value[:i[0]]
	}
	return strings.Trim(normalizeWikiquoteText(value, false), " ~")
}

func normalizeWikiquoteText(value string, preserveBreaks bool) string {
	value = wikiquoteCommentPattern.ReplaceAllString(value, "")
	value = wikiquoteLinkPattern.ReplaceAllStringFunc(value, func(match string) string {
		g := wikiquoteLinkPattern.FindStringSubmatch(match)
		if len(g) > 2 && strings.TrimSpace(g[2]) != "" {
			return g[2]
		}
		return g[1]
	})
	if preserveBreaks {
		value = wikiquoteBreakPattern.ReplaceAllString(value, "\n")
	} else {
		value = wikiquoteBreakPattern.ReplaceAllString(value, " ")
	}
	value = wikiquoteTagPattern.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, string([]byte{39, 39, 39}), "")
	value = strings.ReplaceAll(value, string([]byte{39, 39}), "")
	value = html.UnescapeString(value)
	if !preserveBreaks {
		return strings.Join(strings.Fields(value), " ")
	}
	lines := strings.Split(value, "\n")
	normalized := lines[:0]
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			normalized = append(normalized, line)
		}
	}
	return strings.Join(normalized, "\n")
}
