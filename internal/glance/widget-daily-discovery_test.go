package glance

import (
	"strings"
	"testing"
	"time"
)

func TestDailyDiscoveryWidgetsRegistered(t *testing.T) {
	for _, typ := range []string{"word-of-the-day", "trivia", "on-this-day", "animal-of-the-day"} {
		if _, ok := widgetRegistry[typ]; !ok {
			t.Fatalf("widget %q is not registered", typ)
		}
		defaults, ok := builtinWidgetDefaults[typ]
		if !ok || defaults.CacheCron == nil || *defaults.CacheCron != "5 0 * * *" {
			t.Fatalf("widget %q daily cache default missing", typ)
		}
	}
	if d := builtinWidgetDefaults["on-this-day"]; d.Limit == nil || *d.Limit != 3 {
		t.Fatal("on-this-day default limit != 3")
	}
}

func TestDailyDiscoveryWidgetsAreRefreshableAfterInitialization(t *testing.T) {
	widgets := []widget{
		&wordOfTheDayWidget{},
		&triviaWidget{},
		&onThisDayWidget{Limit: 3},
		&animalOfTheDayWidget{},
	}

	now := time.Now()
	for _, candidate := range widgets {
		if err := candidate.initialize(); err != nil {
			t.Fatalf("initialize %T: %v", candidate, err)
		}
		base, ok := widgetBaseOf(candidate)
		if !ok {
			t.Fatalf("widget %T has no widget base", candidate)
		}
		if base.cacheType == cacheTypeInfinite {
			t.Fatalf("widget %T remained infinitely cached after initialization", candidate)
		}
		if !base.requiresUpdate(&now) {
			t.Fatalf("widget %T is not due for its initial refresh", candidate)
		}
	}
}

func TestParseWiktionaryWordOfTheDaySelectsTodayAndParsesSummary(t *testing.T) {
	feed := wiktionaryFeed{}
	appendEntry := func(updated, word, definition, href string) {
		e := struct {
			Title   string `xml:"title"`
			Updated string `xml:"updated"`
			Summary string `xml:"summary"`
			Links   []struct {
				Href string `xml:"href,attr"`
				Rel  string `xml:"rel,attr"`
			} `xml:"link"`
		}{
			Title:   "Word of the day for " + updated[:10],
			Updated: updated,
			Summary: `<div><ul><li>Navigation item.</li></ul><span id="WOTD-rss-title">` + word + `</span><div id="WOTD-rss-description"><ol><li>` + definition + `</li></ol></div></div>`,
		}
		e.Links = append(e.Links, struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		}{Href: href, Rel: "alternate"})
		feed.Entries = append(feed.Entries, e)
	}

	appendEntry("2026-10-01T00:00:00Z", "cadence", "A rhythm or flow.", "https://en.wiktionary.org/wiki/cadence")
	appendEntry("2026-10-02T00:00:00Z", "trundle", "To move heavily or noisily.", "https://en.wiktionary.org/wiki/trundle")

	got, err := parseWiktionaryWordOfTheDayAt(feed, time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Word != "trundle" || got.Definition != "To move heavily or noisily." || got.Date != "2026-10-02" || got.URL != "https://en.wiktionary.org/wiki/trundle" {
		t.Fatalf("unexpected entry: %#v", got)
	}
}

func TestParseWiktionaryWordOfTheDayFallsBackToNewestEntry(t *testing.T) {
	feed := wiktionaryFeed{}
	for _, entry := range []struct {
		updated, word string
	}{
		{"2026-10-01T00:00:00Z", "cadence"},
		{"2026-10-02T00:00:00Z", "trundle"},
	} {
		feed.Entries = append(feed.Entries, struct {
			Title   string `xml:"title"`
			Updated string `xml:"updated"`
			Summary string `xml:"summary"`
			Links   []struct {
				Href string `xml:"href,attr"`
				Rel  string `xml:"rel,attr"`
			} `xml:"link"`
		}{
			Updated: entry.updated,
			Summary: `<span id="WOTD-rss-title">` + entry.word + `</span><div id="WOTD-rss-description"><ol><li>Definition.</li></ol></div>`,
		})
	}

	got, err := parseWiktionaryWordOfTheDayAt(feed, time.Date(2026, 10, 3, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Word != "trundle" || got.Date != "2026-10-02" {
		t.Fatalf("unexpected fallback entry: %#v", got)
	}
}

func TestParseTriviaQuestionDecodesEntitiesAndKeepsOneCorrectAnswer(t *testing.T) {
	var r openTriviaResponse
	r.ResponseCode = 0
	r.Results = append(r.Results, struct {
		Category         string   `json:"category"`
		Difficulty       string   `json:"difficulty"`
		Question         string   `json:"question"`
		CorrectAnswer    string   `json:"correct_answer"`
		IncorrectAnswers []string `json:"incorrect_answers"`
	}{Category: "Science &amp; Nature", Difficulty: "hard", Question: "What does &quot;X&quot; mean?", CorrectAnswer: "A &amp; B", IncorrectAnswers: []string{"C", "D", "E"}})
	got, err := parseTriviaQuestion(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "Science & Nature" || got.Question != `What does "X" mean?` || got.Difficulty != "Hard" {
		t.Fatalf("unexpected question: %#v", got)
	}
	correct := 0
	for _, a := range got.Answers {
		if a.Correct {
			correct++
			if a.Text != "A & B" {
				t.Fatalf("correct answer not decoded: %q", a.Text)
			}
		}
	}
	if correct != 1 || len(got.Answers) != 4 {
		t.Fatalf("answers=%#v", got.Answers)
	}
}

func TestSelectAnimalOfTheDayRejectsUnlicensedPhoto(t *testing.T) {
	var r iNaturalistSpeciesCountsResponse
	r.Results = make([]struct {
		Count int `json:"count"`
		Taxon struct {
			ID                  int    `json:"id"`
			Rank                string `json:"rank"`
			IsActive            bool   `json:"is_active"`
			Extinct             bool   `json:"extinct"`
			Name                string `json:"name"`
			PreferredCommonName string `json:"preferred_common_name"`
			WikipediaURL        string `json:"wikipedia_url"`
			DefaultPhoto        struct {
				LicenseCode *string `json:"license_code"`
				Attribution string  `json:"attribution"`
				MediumURL   string  `json:"medium_url"`
			} `json:"default_photo"`
			ConservationStatus *struct {
				Authority  string `json:"authority"`
				StatusName string `json:"status_name"`
			} `json:"conservation_status"`
		} `json:"taxon"`
	}, 2)
	unlicensed := &r.Results[0]
	unlicensed.Count = 1000
	unlicensed.Taxon.ID = 1
	unlicensed.Taxon.Rank = "species"
	unlicensed.Taxon.IsActive = true
	unlicensed.Taxon.Name = "Bad species"
	unlicensed.Taxon.PreferredCommonName = "Bad"
	unlicensed.Taxon.WikipediaURL = "https://example.com/bad"
	unlicensed.Taxon.DefaultPhoto.MediumURL = "https://example.com/bad.jpg"
	licensed := &r.Results[1]
	licensed.Count = 900
	licensed.Taxon.ID = 2
	licensed.Taxon.Rank = "species"
	licensed.Taxon.IsActive = true
	licensed.Taxon.Name = "Good species"
	licensed.Taxon.PreferredCommonName = "Good"
	licensed.Taxon.WikipediaURL = "https://example.com/good"
	licensed.Taxon.DefaultPhoto.MediumURL = "https://example.com/good.jpg"
	code := "cc-by"
	licensed.Taxon.DefaultPhoto.LicenseCode = &code
	got, err := selectAnimalOfTheDay(r, "Mammalia", "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	if got.CommonName != "Good" || !strings.Contains(got.INaturalistURL, "/2") {
		t.Fatalf("unexpected animal: %#v", got)
	}
}

func TestStableDailyIndex(t *testing.T) {
	first := stableDailyIndex("2026-10-01", 8)
	if first != stableDailyIndex("2026-10-01", 8) {
		t.Fatal("daily index is not deterministic")
	}
	if first < 0 || first >= 8 {
		t.Fatalf("daily index out of range: %d", first)
	}
}
