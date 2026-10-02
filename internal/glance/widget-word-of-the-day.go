package glance

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

var wordOfTheDayWidgetTemplate = mustParseTemplate("word-of-the-day.html", "widget-base.html")

const wiktionaryWordOfTheDayURL = "https://en.wiktionary.org/w/api.php?action=featuredfeed&feed=wotd&feedformat=atom&format=xml"

type wordOfTheDayWidget struct {
	widgetBase `yaml:",inline"`
	Entry      wordOfTheDayEntry `yaml:"-"`
}

type wordOfTheDayEntry struct{ Word, Definition, URL, Date string }

type wiktionaryFeed struct {
	Entries []struct {
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Summary string `xml:"summary"`
		Links   []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func (w *wordOfTheDayWidget) initialize() error {
	w.withTitle("Word of the Day").withCacheDuration(24 * time.Hour)
	return nil
}
func (w *wordOfTheDayWidget) update(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dailyDiscoveryProviderURL("/daily-discovery/wiktionary", wiktionaryWordOfTheDayURL), nil)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	req.Header.Set("User-Agent", glanceUserAgentString)
	feed, err := decodeXmlFromRequest[wiktionaryFeed](defaultHTTPClient, req)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching Wiktionary word of the day: %w", err))
		return
	}
	entry, err := parseWiktionaryWordOfTheDayAt(feed, time.Now())
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	if !w.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	w.Entry = entry
}
func (w *wordOfTheDayWidget) Render() template.HTML {
	return w.renderTemplate(w, wordOfTheDayWidgetTemplate)
}

func parseWiktionaryWordOfTheDayAt(feed wiktionaryFeed, now time.Time) (wordOfTheDayEntry, error) {
	if len(feed.Entries) == 0 {
		return wordOfTheDayEntry{}, fmt.Errorf("Wiktionary word of the day feed contained no entries")
	}

	today := now.Format("2006-01-02")
	selected := len(feed.Entries) - 1
	for i := range feed.Entries {
		if strings.HasPrefix(feed.Entries[i].Updated, today) {
			selected = i
			break
		}
	}

	e := feed.Entries[selected]
	word := elementTextByID(e.Summary, "WOTD-rss-title")
	if word == "" {
		return wordOfTheDayEntry{}, fmt.Errorf("Wiktionary word of the day entry has no word")
	}

	definition := firstListItemTextByID(e.Summary, "WOTD-rss-description")
	if definition == "" {
		return wordOfTheDayEntry{}, fmt.Errorf("Wiktionary word of the day entry has no definition")
	}

	date := e.Updated
	if len(date) >= 10 {
		date = date[:10]
	}
	entryURL := ""
	for _, link := range e.Links {
		if strings.TrimSpace(link.Rel) == "" || strings.EqualFold(strings.TrimSpace(link.Rel), "alternate") {
			entryURL = strings.TrimSpace(link.Href)
			if strings.EqualFold(strings.TrimSpace(link.Rel), "alternate") {
				break
			}
		}
	}

	return wordOfTheDayEntry{Word: word, Definition: definition, URL: entryURL, Date: date}, nil
}

func elementTextByID(fragment, id string) string {
	n := elementByID(fragment, id)
	if n == nil {
		return ""
	}
	return strings.Join(strings.Fields(nodeText(n)), " ")
}

func firstListItemTextByID(fragment, id string) string {
	root := elementByID(fragment, id)
	if root == nil {
		return ""
	}

	var walk func(*xhtml.Node) string
	walk = func(n *xhtml.Node) string {
		if n.Type == xhtml.ElementNode && n.Data == "li" {
			return strings.Join(strings.Fields(nodeText(n)), " ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if v := walk(c); v != "" {
				return v
			}
		}
		return ""
	}

	return walk(root)
}

func elementByID(fragment, id string) *xhtml.Node {
	nodes, err := xhtml.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return nil
	}

	var walk func(*xhtml.Node) *xhtml.Node
	walk = func(n *xhtml.Node) *xhtml.Node {
		if n.Type == xhtml.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == id {
					return n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}

	for _, n := range nodes {
		if found := walk(n); found != nil {
			return found
		}
	}
	return nil
}

func nodeText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
